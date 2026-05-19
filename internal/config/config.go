package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Dockhand DockhandConfig
	Defaults Defaults
}

type ServerConfig struct {
	Listen     string
	PublicPath string
}

type DockhandConfig struct {
	BaseURL       string
	APIToken      string
	EnvironmentID int
	Timeout       time.Duration
}

type Defaults struct {
	Lease        LeaseDuration
	LeaseOptions []LeaseDuration
	StartTimeout time.Duration
	StopGrace    time.Duration
	PollInterval time.Duration
}

type LeaseDuration struct {
	Label    string
	Duration time.Duration
	Never    bool
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	values, err := parseYAMLFile(path)
	if err != nil {
		return cfg, err
	}
	if v := str(values, "server.listen"); v != "" {
		cfg.Server.Listen = v
	}
	if v := str(values, "server.publicPath"); v != "" {
		cfg.Server.PublicPath = v
	}
	if v := str(values, "dockhand.baseUrl"); v != "" {
		cfg.Dockhand.BaseURL = strings.TrimRight(v, "/")
	}
	if v := str(values, "dockhand.apiToken"); v != "" {
		cfg.Dockhand.APIToken = expandEnv(v)
	}
	if v := str(values, "dockhand.environmentId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("dockhand.environmentId: %w", err)
		}
		cfg.Dockhand.EnvironmentID = id
	}
	if v := str(values, "dockhand.timeout"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("dockhand.timeout: %w", err)
		}
		cfg.Dockhand.Timeout = d
	}
	if v := str(values, "defaults.lease"); v != "" {
		lease, err := ParseLeaseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("defaults.lease: %w", err)
		}
		cfg.Defaults.Lease = lease
	}
	if vs := list(values, "defaults.leaseOptions"); len(vs) > 0 {
		options, err := ParseLeaseDurations(vs)
		if err != nil {
			return cfg, fmt.Errorf("defaults.leaseOptions: %w", err)
		}
		cfg.Defaults.LeaseOptions = options
	}
	for key, dst := range map[string]*time.Duration{
		"defaults.startTimeout": &cfg.Defaults.StartTimeout,
		"defaults.stopGrace":    &cfg.Defaults.StopGrace,
		"defaults.pollInterval": &cfg.Defaults.PollInterval,
	} {
		if v := str(values, key); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return cfg, fmt.Errorf("%s: %w", key, err)
			}
			*dst = d
		}
	}
	return cfg, validate(cfg)
}

func DefaultConfig() Config {
	options, _ := ParseLeaseDurations([]string{"30m", "1h", "2h", "4h", "never"})
	lease, _ := ParseLeaseDuration("2h")
	return Config{
		Server: ServerConfig{Listen: ":8080", PublicPath: "/_reveille"},
		Dockhand: DockhandConfig{
			BaseURL:       "http://dockhand:3000",
			EnvironmentID: 1,
			Timeout:       30 * time.Second,
		},
		Defaults: Defaults{
			Lease:        lease,
			LeaseOptions: options,
			StartTimeout: 3 * time.Minute,
			StopGrace:    30 * time.Second,
			PollInterval: 5 * time.Second,
		},
	}
}

func validate(cfg Config) error {
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if cfg.Server.PublicPath == "" || !strings.HasPrefix(cfg.Server.PublicPath, "/") {
		return fmt.Errorf("server.publicPath must start with /")
	}
	if cfg.Dockhand.BaseURL == "" {
		return fmt.Errorf("dockhand.baseUrl is required")
	}
	if cfg.Dockhand.EnvironmentID <= 0 {
		return fmt.Errorf("dockhand.environmentId must be positive")
	}
	if cfg.Dockhand.Timeout <= 0 || cfg.Defaults.StartTimeout <= 0 || cfg.Defaults.PollInterval <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	return nil
}

func ParseLeaseDurations(values []string) ([]LeaseDuration, error) {
	out := make([]LeaseDuration, 0, len(values))
	for _, value := range values {
		lease, err := ParseLeaseDuration(value)
		if err != nil {
			return nil, err
		}
		out = append(out, lease)
	}
	return out, nil
}

func ParseLeaseDuration(value string) (LeaseDuration, error) {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	if strings.EqualFold(value, "never") {
		return LeaseDuration{Label: "Never", Never: true}, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return LeaseDuration{}, err
	}
	return LeaseDuration{Label: value, Duration: d}, nil
}

func expandEnv(value string) string {
	return os.Expand(value, func(key string) string { return os.Getenv(key) })
}
