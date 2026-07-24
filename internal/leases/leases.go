package leases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"reveille/internal/config"
	"reveille/internal/hosts"
	"reveille/internal/logging"
)

type StopFunc func(context.Context, hosts.Host) error

// EventFunc receives lease lifecycle events: "lease_expired_stop" after a
// successful timer-driven stop and "stop_failed" when that stop errors.
type EventFunc func(event string, host hosts.Host, err error)

type Lease struct {
	Host        string        `json:"host"`
	Label       string        `json:"label,omitempty"`
	Never       bool          `json:"never"`
	Idle        bool          `json:"idle,omitempty"`
	IdleWindow  time.Duration `json:"idleWindowNanos,omitempty"`
	Provisional bool          `json:"provisional,omitempty"`
	ExpiresAt   time.Time     `json:"expiresAt,omitzero"`
}

type entry struct {
	lease        Lease
	host         hosts.Host
	lastActivity time.Time
	timer        *time.Timer
}

type Manager struct {
	mu        sync.Mutex
	leases    map[string]*entry
	stop      StopFunc
	stopTTL   time.Duration
	logger    *logging.Logger
	statePath string
	onEvent   EventFunc
	closed    bool
}

func NewManager(stop StopFunc, logger ...*logging.Logger) *Manager {
	return &Manager{
		leases:  map[string]*entry{},
		stop:    stop,
		stopTTL: 30 * time.Second,
		logger:  firstLogger(logger),
	}
}

// SetStatePath enables lease persistence to the given file. Call before the
// manager is shared between goroutines.
func (m *Manager) SetStatePath(path string) {
	m.statePath = path
}

// SetOnEvent registers a callback for lease lifecycle events. Call before the
// manager is shared between goroutines.
func (m *Manager) SetOnEvent(fn EventFunc) {
	m.onEvent = fn
}

// Set replaces the lease for the host with a user-selected one.
func (m *Manager) Set(host hosts.Host, lease config.LeaseDuration, now time.Time) Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setLocked(host, lease, now, false)
}

// EnsureProvisional arms a lease only when the host has none, marking it
// provisional so a later user selection can replace it. It reports whether a
// new lease was created.
func (m *Manager) EnsureProvisional(host hosts.Host, lease config.LeaseDuration, now time.Time) (Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.leases[host.Host]; ok {
		return m.effectiveLocked(existing), false
	}
	return m.setLocked(host, lease, now, true), true
}

func (m *Manager) setLocked(host hosts.Host, lease config.LeaseDuration, now time.Time, provisional bool) Lease {
	key := host.Host
	if existing := m.leases[key]; existing != nil && existing.timer != nil {
		existing.timer.Stop()
	}
	active := Lease{
		Host:        key,
		Label:       lease.Label,
		Never:       lease.Never,
		Idle:        lease.Idle,
		Provisional: provisional,
	}
	if lease.Idle {
		active.IdleWindow = lease.Duration
	}
	if !lease.Never {
		active.ExpiresAt = now.Add(lease.Duration)
	}
	e := &entry{lease: active, host: host, lastActivity: now}
	if !lease.Never {
		// Delay by the lease duration rather than time.Until(ExpiresAt) so
		// an injected clock cannot skew the timer.
		e.timer = time.AfterFunc(lease.Duration, func() { m.expire(key) })
	}
	m.leases[key] = e
	m.persistLocked()
	return active
}

// Touch records request activity for the host so idle leases slide forward.
func (m *Manager) Touch(host string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.leases[host]; ok {
		e.lastActivity = now
	}
}

func (m *Manager) expire(key string) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	e, ok := m.leases[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	now := time.Now()
	if e.lease.Idle {
		deadline := e.lastActivity.Add(e.lease.IdleWindow)
		if deadline.After(now) {
			e.lease.ExpiresAt = deadline
			e.timer = time.AfterFunc(time.Until(deadline), func() { m.expire(key) })
			m.persistLocked()
			m.mu.Unlock()
			return
		}
	}
	host := e.host
	delete(m.leases, key)
	m.persistLocked()
	m.mu.Unlock()

	m.logger.Infof("lease expired for %s; requesting stop", key)
	ctx, cancel := context.WithTimeout(context.Background(), m.stopTTL)
	defer cancel()
	if err := m.stop(ctx, host); err != nil {
		m.logger.Errorf("lease stop failed for %s: %v", key, err)
		m.fireEvent("stop_failed", host, err)
		return
	}
	m.logger.Infof("lease stop succeeded for %s", key)
	m.fireEvent("lease_expired_stop", host, nil)
}

func (m *Manager) fireEvent(event string, host hosts.Host, err error) {
	if m.onEvent != nil {
		m.onEvent(event, host, err)
	}
}

func (m *Manager) StopNow(ctx context.Context, host hosts.Host) error {
	m.mu.Lock()
	if e, ok := m.leases[host.Host]; ok && e.timer != nil {
		e.timer.Stop()
	}
	delete(m.leases, host.Host)
	m.persistLocked()
	m.mu.Unlock()
	return m.stop(ctx, host)
}

func (m *Manager) Get(host string) (Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.leases[host]
	if !ok {
		return Lease{}, false
	}
	return m.effectiveLocked(e), true
}

// All returns every active lease sorted by host.
func (m *Manager) All() []Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Lease, 0, len(m.leases))
	for _, e := range m.leases {
		out = append(out, m.effectiveLocked(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

// effectiveLocked reports the lease with the sliding deadline idle leases
// will actually expire at, so countdowns reflect recent activity.
func (m *Manager) effectiveLocked(e *entry) Lease {
	lease := e.lease
	if lease.Idle {
		if deadline := e.lastActivity.Add(lease.IdleWindow); deadline.After(lease.ExpiresAt) {
			lease.ExpiresAt = deadline
		}
	}
	return lease
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, e := range m.leases {
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	m.persistLocked()
}

// UpdateHosts refreshes the stored host snapshot for active leases after a
// host reload, so timer-driven stops use current target settings. Leases for
// removed hosts keep their old snapshot so the stop still runs at expiry.
func (m *Manager) UpdateHosts(lookup func(string) (hosts.Host, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, e := range m.leases {
		if host, ok := lookup(key); ok {
			e.host = host
		}
	}
}

type persistedLease struct {
	Lease        Lease      `json:"lease"`
	Host         hosts.Host `json:"hostConfig"`
	LastActivity time.Time  `json:"lastActivity"`
}

type stateFile struct {
	Leases []persistedLease `json:"leases"`
}

func (m *Manager) persistLocked() {
	if m.statePath == "" {
		return
	}
	state := stateFile{Leases: make([]persistedLease, 0, len(m.leases))}
	for _, e := range m.leases {
		state.Leases = append(state.Leases, persistedLease{
			Lease:        e.lease,
			Host:         e.host,
			LastActivity: e.lastActivity,
		})
	}
	sort.Slice(state.Leases, func(i, j int) bool { return state.Leases[i].Lease.Host < state.Leases[j].Lease.Host })
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		m.logger.Errorf("persist leases: %v", err)
		return
	}
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		m.logger.Errorf("persist leases: %v", err)
		return
	}
	if err := os.Rename(tmp, m.statePath); err != nil {
		m.logger.Errorf("persist leases: %v", err)
	}
}

// Load restores persisted leases and re-arms their timers. Leases already
// expired at load time trigger an immediate asynchronous stop.
func (m *Manager) Load(now time.Time) error {
	if m.statePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode %s: %w", m.statePath, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	restored := 0
	for _, saved := range state.Leases {
		key := saved.Lease.Host
		if key == "" || saved.Host.Host == "" {
			continue
		}
		if _, exists := m.leases[key]; exists {
			continue
		}
		e := &entry{lease: saved.Lease, host: saved.Host, lastActivity: saved.LastActivity}
		if !saved.Lease.Never {
			delay := time.Until(saved.Lease.ExpiresAt)
			if delay < 0 {
				delay = 0
			}
			key := key
			e.timer = time.AfterFunc(delay, func() { m.expire(key) })
		}
		m.leases[key] = e
		restored++
	}
	if restored > 0 {
		m.logger.Infof("restored %d lease(s) from %s", restored, m.statePath)
	}
	return nil
}

// EnsureStateDir verifies the state file's directory exists and is writable,
// creating it when missing. It returns an error when persistence cannot work
// so the caller can disable it explicitly.
func EnsureStateDir(path string) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	probe, err := os.OpenFile(filepath.Join(dir, ".reveille-write-check"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	name := probe.Name()
	_ = probe.Close()
	return os.Remove(name)
}

func firstLogger(loggers []*logging.Logger) *logging.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}
	return logging.Must("info")
}
