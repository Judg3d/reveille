package hosts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reveille/internal/config"
)

type Host struct {
	Host    string
	Target  Target
	Lease   Lease
	Routing Routing
}

type rawFile struct {
	Host    string   `yaml:"host"`
	Target  Target   `yaml:"target"`
	Targets []Target `yaml:"targets"`
	Lease   rawLease `yaml:"lease"`
	Routing Routing  `yaml:"routing"`
}

type rawLease struct {
	Default string   `yaml:"default"`
	Options []string `yaml:"options"`
}

type Target struct {
	Type          string `yaml:"type"`
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Environment   string `yaml:"environment"`
	Hostname      string `yaml:"hostname"`
	HealthURL     string `yaml:"healthUrl"`
	HealthyStatus []int  `yaml:"healthyStatus"`
}

type Lease struct {
	Default config.LeaseDuration
	Options []config.LeaseDuration
}

type Routing struct {
	ReturnToHeader string `yaml:"returnToHeader"`
}

type Store struct {
	mu       sync.RWMutex
	dir      string
	defaults config.Defaults
	byHost   map[string]Host
}

func LoadDir(dir string, defaults config.Defaults) (*Store, error) {
	store := &Store{dir: dir, defaults: defaults, byHost: map[string]Host{}}
	loaded, err := loadDir(dir, defaults)
	if err != nil {
		return nil, err
	}
	store.byHost = loaded
	return store, nil
}

func loadDir(dir string, defaults config.Defaults) (map[string]Host, error) {
	byHost := map[string]Host{}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return byHost, nil
		}
		return nil, err
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		hosts, err := LoadFile(path, defaults)
		if err != nil {
			return err
		}
		for _, host := range hosts {
			byHost[strings.ToLower(host.Host)] = host
		}
		return nil
	})
	return byHost, err
}

func LoadFile(path string, defaults config.Defaults) ([]Host, error) {
	var raw rawFile
	if err := config.LoadYAML(path, &raw); err != nil {
		return nil, err
	}
	targets := raw.Targets
	if len(targets) == 0 {
		targets = []Target{raw.Target}
	}
	lease, err := parseLease(path, defaults, raw.Lease)
	if err != nil {
		return nil, err
	}
	routing := raw.Routing
	if routing.ReturnToHeader == "" {
		routing.ReturnToHeader = "X-Forwarded-Uri"
	}

	hosts := make([]Host, 0, len(targets))
	for i, target := range targets {
		h := Host{
			Host:    firstNonEmpty(target.Hostname, raw.Host),
			Target:  target,
			Lease:   lease,
			Routing: routing,
		}
		if h.Target.Type == "" {
			h.Target.Type = "container"
		}
		if len(h.Target.HealthyStatus) == 0 {
			h.Target.HealthyStatus = []int{200}
		}
		h, err := validateHost(path, i, h)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func parseLease(path string, defaults config.Defaults, raw rawLease) (Lease, error) {
	lease := Lease{Default: defaults.Lease, Options: defaults.LeaseOptions}
	if raw.Default != "" {
		parsed, err := config.ParseLeaseDuration(raw.Default)
		if err != nil {
			return lease, fmt.Errorf("%s lease.default: %w", path, err)
		}
		lease.Default = parsed
	}
	if len(raw.Options) > 0 {
		options, err := config.ParseLeaseDurations(raw.Options)
		if err != nil {
			return lease, fmt.Errorf("%s lease.options: %w", path, err)
		}
		lease.Options = options
	}
	return lease, nil
}

func NewHost(hostname string, target Target, defaults config.Defaults) (Host, error) {
	h := Host{
		Host:    firstNonEmpty(target.Hostname, hostname),
		Target:  target,
		Lease:   Lease{Default: defaults.Lease, Options: defaults.LeaseOptions},
		Routing: Routing{ReturnToHeader: "X-Forwarded-Uri"},
	}
	if h.Target.Type == "" {
		h.Target.Type = "container"
	}
	if len(h.Target.HealthyStatus) == 0 {
		h.Target.HealthyStatus = []int{200}
	}
	return validateHost("target", 0, h)
}

func (s *Store) Lookup(host string) (Host, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.byHost[host]
	return h, ok
}

func (s *Store) Reload() error {
	loaded, err := loadDir(s.dir, s.defaults)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.byHost = loaded
	s.mu.Unlock()
	return nil
}

func (s *Store) Watch(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reload(); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

func validateHost(path string, index int, h Host) (Host, error) {
	label := path
	if index > 0 {
		label = fmt.Sprintf("%s targets[%d]", path, index)
	}
	if h.Host == "" {
		return h, fmt.Errorf("%s hostname is required", label)
	}
	if h.Target.Type != "container" && h.Target.Type != "stack" {
		return h, fmt.Errorf("%s target.type must be container or stack", label)
	}
	if h.Target.Type == "container" && h.Target.ID == "" {
		return h, fmt.Errorf("%s target.id is required for container targets", label)
	}
	if h.Target.Type == "stack" && h.Target.Name == "" {
		return h, fmt.Errorf("%s target.name is required for stack targets", label)
	}
	if h.Target.Type == "stack" && h.Target.HealthURL == "" {
		return h, fmt.Errorf("%s target.healthUrl is required for stack targets", label)
	}
	return h, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
