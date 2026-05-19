package leases

import (
	"context"
	"sync"
	"time"

	"reveille/internal/config"
	"reveille/internal/hosts"
)

type StopFunc func(context.Context, hosts.Host) error

type Lease struct {
	Host      string
	Never     bool
	ExpiresAt time.Time
}

type Manager struct {
	mu      sync.Mutex
	leases  map[string]Lease
	timers  map[string]*time.Timer
	stop    StopFunc
	closed  bool
	stopTTL time.Duration
}

func NewManager(stop StopFunc) *Manager {
	return &Manager{
		leases:  map[string]Lease{},
		timers:  map[string]*time.Timer{},
		stop:    stop,
		stopTTL: 30 * time.Second,
	}
}

func (m *Manager) Set(host hosts.Host, lease config.LeaseDuration, now time.Time) Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := host.Host
	if timer := m.timers[key]; timer != nil {
		timer.Stop()
		delete(m.timers, key)
	}
	active := Lease{Host: key, Never: lease.Never}
	if !lease.Never {
		active.ExpiresAt = now.Add(lease.Duration)
		m.timers[key] = time.AfterFunc(lease.Duration, func() {
			ctx, cancel := context.WithTimeout(context.Background(), m.stopTTL)
			defer cancel()
			_ = m.stop(ctx, host)
			m.mu.Lock()
			delete(m.leases, key)
			delete(m.timers, key)
			m.mu.Unlock()
		})
	}
	m.leases[key] = active
	return active
}

func (m *Manager) StopNow(ctx context.Context, host hosts.Host) error {
	m.mu.Lock()
	if timer := m.timers[host.Host]; timer != nil {
		timer.Stop()
	}
	delete(m.timers, host.Host)
	delete(m.leases, host.Host)
	m.mu.Unlock()
	return m.stop(ctx, host)
}

func (m *Manager) Get(host string) (Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease, ok := m.leases[host]
	return lease, ok
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, timer := range m.timers {
		timer.Stop()
	}
	m.closed = true
}
