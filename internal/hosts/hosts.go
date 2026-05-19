package hosts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

type Target struct {
	Type          string
	ID            string
	Name          string
	HealthURL     string
	HealthyStatus []int
}

type Lease struct {
	Default config.LeaseDuration
	Options []config.LeaseDuration
}

type Routing struct {
	ReturnToHeader string
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
		host, err := LoadFile(path, defaults)
		if err != nil {
			return err
		}
		byHost[strings.ToLower(host.Host)] = host
		return nil
	})
	return byHost, err
}

func LoadFile(path string, defaults config.Defaults) (Host, error) {
	values, err := config.ParseYAMLFile(path)
	if err != nil {
		return Host{}, err
	}
	h := Host{
		Host: config.String(values, "host"),
		Target: Target{
			Type:          config.String(values, "target.type"),
			ID:            config.String(values, "target.id"),
			Name:          config.String(values, "target.name"),
			HealthURL:     config.String(values, "target.healthUrl"),
			HealthyStatus: []int{200},
		},
		Lease:   Lease{Default: defaults.Lease, Options: defaults.LeaseOptions},
		Routing: Routing{ReturnToHeader: "X-Forwarded-Uri"},
	}
	if h.Target.Type == "" {
		h.Target.Type = "container"
	}
	if statuses := config.List(values, "target.healthyStatus"); len(statuses) > 0 {
		h.Target.HealthyStatus = nil
		for _, value := range statuses {
			code, err := strconv.Atoi(value)
			if err != nil {
				return h, fmt.Errorf("%s target.healthyStatus: %w", path, err)
			}
			h.Target.HealthyStatus = append(h.Target.HealthyStatus, code)
		}
	}
	if v := config.String(values, "lease.default"); v != "" {
		lease, err := config.ParseLeaseDuration(v)
		if err != nil {
			return h, fmt.Errorf("%s lease.default: %w", path, err)
		}
		h.Lease.Default = lease
	}
	if vs := config.List(values, "lease.options"); len(vs) > 0 {
		options, err := config.ParseLeaseDurations(vs)
		if err != nil {
			return h, fmt.Errorf("%s lease.options: %w", path, err)
		}
		h.Lease.Options = options
	}
	if v := config.String(values, "routing.returnToHeader"); v != "" {
		h.Routing.ReturnToHeader = v
	}
	return validateHost(path, h)
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

func validateHost(path string, h Host) (Host, error) {
	if h.Host == "" {
		return h, fmt.Errorf("%s host is required", path)
	}
	if h.Target.Type != "container" && h.Target.Type != "stack" {
		return h, fmt.Errorf("%s target.type must be container or stack", path)
	}
	if h.Target.Type == "container" && h.Target.ID == "" {
		return h, fmt.Errorf("%s target.id is required for container targets", path)
	}
	if h.Target.Type == "stack" && h.Target.Name == "" {
		return h, fmt.Errorf("%s target.name is required for stack targets", path)
	}
	if h.Target.HealthURL == "" {
		return h, fmt.Errorf("%s target.healthUrl is required", path)
	}
	return h, nil
}
