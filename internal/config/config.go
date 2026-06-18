package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"reveille/internal/logging"
)

type Config struct {
	Server   ServerConfig
	Log      LogConfig
	Dockhand DockhandConfig
	Defaults Defaults
}

type ServerConfig struct {
	Listen                 string
	PublicPath             string
	FailClosedUnknownHosts bool
}

type LogConfig struct {
	Level string
}

type DockhandConfig struct {
	BaseURL  string
	APIToken string
	Timeout  time.Duration
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

type rawConfig struct {
	Server struct {
		Listen                 string `yaml:"listen"`
		PublicPath             string `yaml:"publicPath"`
		FailClosedUnknownHosts bool   `yaml:"failClosedUnknownHosts"`
	} `yaml:"server"`
	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
	Dockhand struct {
		BaseURL  string `yaml:"baseUrl"`
		APIToken string `yaml:"apiToken"`
		Timeout  string `yaml:"timeout"`
	} `yaml:"dockhand"`
	Defaults struct {
		Lease        string   `yaml:"lease"`
		LeaseOptions []string `yaml:"leaseOptions"`
		StartTimeout string   `yaml:"startTimeout"`
		StopGrace    string   `yaml:"stopGrace"`
		PollInterval string   `yaml:"pollInterval"`
	} `yaml:"defaults"`
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var raw rawConfig
	if err := LoadYAML(path, &raw); err != nil {
		return cfg, err
	}
	if raw.Server.Listen != "" {
		cfg.Server.Listen = raw.Server.Listen
	}
	if raw.Server.PublicPath != "" {
		cfg.Server.PublicPath = raw.Server.PublicPath
	}
	cfg.Server.FailClosedUnknownHosts = raw.Server.FailClosedUnknownHosts
	if raw.Log.Level != "" {
		level, err := logging.NormalizeLevel(raw.Log.Level)
		if err != nil {
			return cfg, fmt.Errorf("log.level: %w", err)
		}
		cfg.Log.Level = level
	}
	if raw.Dockhand.BaseURL != "" {
		cfg.Dockhand.BaseURL = strings.TrimRight(raw.Dockhand.BaseURL, "/")
	}
	if raw.Dockhand.APIToken != "" {
		cfg.Dockhand.APIToken = expandEnv(raw.Dockhand.APIToken)
	}
	if cfg.Dockhand.APIToken == "" {
		cfg.Dockhand.APIToken = os.Getenv("DOCKHAND_API_TOKEN")
	}
	if raw.Dockhand.Timeout != "" {
		d, err := time.ParseDuration(raw.Dockhand.Timeout)
		if err != nil {
			return cfg, fmt.Errorf("dockhand.timeout: %w", err)
		}
		cfg.Dockhand.Timeout = d
	}
	if raw.Defaults.Lease != "" {
		lease, err := ParseLeaseDuration(raw.Defaults.Lease)
		if err != nil {
			return cfg, fmt.Errorf("defaults.lease: %w", err)
		}
		cfg.Defaults.Lease = lease
	}
	if len(raw.Defaults.LeaseOptions) > 0 {
		options, err := ParseLeaseDurations(raw.Defaults.LeaseOptions)
		if err != nil {
			return cfg, fmt.Errorf("defaults.leaseOptions: %w", err)
		}
		cfg.Defaults.LeaseOptions = options
	}
	if err := setDuration("defaults.startTimeout", raw.Defaults.StartTimeout, &cfg.Defaults.StartTimeout); err != nil {
		return cfg, err
	}
	if err := setDuration("defaults.stopGrace", raw.Defaults.StopGrace, &cfg.Defaults.StopGrace); err != nil {
		return cfg, err
	}
	if err := setDuration("defaults.pollInterval", raw.Defaults.PollInterval, &cfg.Defaults.PollInterval); err != nil {
		return cfg, err
	}
	return cfg, validate(cfg)
}

func LoadYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

func setDuration(name, value string, dst *time.Duration) error {
	if value == "" {
		return nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = d
	return nil
}

func DefaultConfig() Config {
	options, _ := ParseLeaseDurations([]string{"30m", "1h", "2h", "4h", "never"})
	lease, _ := ParseLeaseDuration("2h")
	cfg := Config{
		Server: ServerConfig{Listen: ":8080", PublicPath: "/_reveille"},
		Log:    LogConfig{Level: "info"},
		Dockhand: DockhandConfig{
			BaseURL:  "http://dockhand:3000",
			APIToken: os.Getenv("DOCKHAND_API_TOKEN"),
			Timeout:  30 * time.Second,
		},
		Defaults: Defaults{
			Lease:        lease,
			LeaseOptions: options,
			StartTimeout: 3 * time.Minute,
			StopGrace:    30 * time.Second,
			PollInterval: 5 * time.Second,
		},
	}
	return cfg
}

func validate(cfg Config) error {
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if _, err := logging.ParseLevel(cfg.Log.Level); err != nil {
		return fmt.Errorf("log.level: %w", err)
	}
	if cfg.Server.PublicPath == "" || !strings.HasPrefix(cfg.Server.PublicPath, "/") {
		return fmt.Errorf("server.publicPath must start with /")
	}
	if cfg.Dockhand.BaseURL == "" {
		return fmt.Errorf("dockhand.baseUrl is required")
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
