package hosts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"reveille/internal/config"
	"reveille/internal/logging"
	"gopkg.in/yaml.v3"
)

type Host struct {
	Host    string
	Target  Target
	Lease   Lease
	Routing Routing
}

type rawFile struct {
	TargetNode yaml.Node `yaml:"target"`
	Lease      rawLease  `yaml:"lease"`
	Routing    Routing   `yaml:"routing"`
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
	state    string
	logger   *logging.Logger
}

func LoadDir(dir string, defaults config.Defaults, logger ...*logging.Logger) (*Store, error) {
	store := &Store{
		dir:      dir,
		defaults: defaults,
		byHost:   map[string]Host{},
		logger:   firstLogger(logger),
	}
	loaded, err := loadDir(dir, defaults)
	if err != nil {
		return nil, err
	}
	store.byHost = loaded
	store.state = snapshotState(loaded)
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
	targets, err := parseTargetNode(path, raw.TargetNode)
	if err != nil {
		return nil, err
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
			Host:    target.Hostname,
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

func parseTargetNode(path string, node yaml.Node) ([]Target, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("%s target is required", path)
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s target must be a mapping", path)
	}

	targets := make([]Target, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s target.%s must be a mapping", path, key)
		}
		var target Target
		if err := value.Decode(&target); err != nil {
			return nil, fmt.Errorf("%s target.%s: %w", path, key, err)
		}
		if target.Name == "" {
			target.Name = key
		}
		targets = append(targets, target)
	}
	return targets, nil
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

func (s *Store) Reload() (bool, int, error) {
	loaded, err := loadDir(s.dir, s.defaults)
	if err != nil {
		return false, 0, err
	}
	state := snapshotState(loaded)
	s.mu.Lock()
	changed := state != s.state
	s.byHost = loaded
	s.state = state
	s.mu.Unlock()
	return changed, len(loaded), nil
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
			changed, count, err := s.Reload()
			if err != nil {
				if onError != nil {
					onError(err)
				}
				continue
			}
			if changed {
				s.logger.Infof("reloaded %d host entries from %s", count, s.dir)
			}
		}
	}
}

func firstLogger(loggers []*logging.Logger) *logging.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}
	return logging.Must("info")
}

func snapshotState(byHost map[string]Host) string {
	keys := make([]string, 0, len(byHost))
	for key := range byHost {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		host := byHost[key]
		parts = append(parts, strings.Join([]string{
			host.Host,
			host.Target.Type,
			host.Target.ID,
			host.Target.Name,
			host.Target.Environment,
			host.Target.Hostname,
			host.Target.HealthURL,
			strconv.Itoa(len(host.Lease.Options)),
			host.Lease.Default.Label,
			host.Routing.ReturnToHeader,
		}, "|"))
	}
	return strings.Join(parts, "\n")
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
