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
	Provider string
	Dockhand DockhandConfig
	Docker   DockerConfig
	Admin    AdminConfig
	Notify   NotifyConfig
	Defaults Defaults
}

type ServerConfig struct {
	Listen                 string
	PublicPath             string
	FailClosedUnknownHosts bool
	TokenSecret            string
	ForwardAuthSecret      string
	ExposeHealthDetail     bool
	StateFile              string
	HostsReloadInterval    time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}

type DockhandConfig struct {
	BaseURL  string
	APIToken string
	Timeout  time.Duration
}

type DockerConfig struct {
	Socket  string
	Timeout time.Duration
}

type AdminConfig struct {
	Listen string
	Token  string
}

type NotifyConfig struct {
	Events   []string
	Gotify   GotifyConfig
	Telegram TelegramConfig
	Ntfy     NtfyConfig
	Webhook  WebhookConfig
}

type GotifyConfig struct {
	URL      string
	Token    string
	Priority int
}

type TelegramConfig struct {
	Token  string
	ChatID string
}

type NtfyConfig struct {
	URL   string
	Topic string
	Token string
}

type WebhookConfig struct {
	URL string
}

type Defaults struct {
	Lease          LeaseDuration
	LeaseOptions   []LeaseDuration
	StartTimeout   time.Duration
	StopGrace      time.Duration
	PollInterval   time.Duration
	HealthCacheTTL time.Duration
	OrphanGrace    time.Duration
}

type LeaseDuration struct {
	Label    string
	Duration time.Duration
	Never    bool
	Idle     bool
}

type rawConfig struct {
	Server struct {
		Listen                 string  `yaml:"listen"`
		PublicPath             string  `yaml:"publicPath"`
		FailClosedUnknownHosts bool    `yaml:"failClosedUnknownHosts"`
		TokenSecret            string  `yaml:"tokenSecret"`
		ForwardAuthSecret      string  `yaml:"forwardAuthSecret"`
		ExposeHealthDetail     bool    `yaml:"exposeHealthDetail"`
		StateFile              *string `yaml:"stateFile"`
		HostsReloadInterval    string  `yaml:"hostsReloadInterval"`
	} `yaml:"server"`
	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"log"`
	Provider string `yaml:"provider"`
	Dockhand struct {
		BaseURL  string `yaml:"baseUrl"`
		APIToken string `yaml:"apiToken"`
		Timeout  string `yaml:"timeout"`
	} `yaml:"dockhand"`
	Docker struct {
		Socket  string `yaml:"socket"`
		Timeout string `yaml:"timeout"`
	} `yaml:"docker"`
	Admin struct {
		Listen string `yaml:"listen"`
		Token  string `yaml:"token"`
	} `yaml:"admin"`
	Notify struct {
		Events []string `yaml:"events"`
		Gotify struct {
			URL      string `yaml:"url"`
			Token    string `yaml:"token"`
			Priority int    `yaml:"priority"`
		} `yaml:"gotify"`
		Telegram struct {
			Token  string `yaml:"token"`
			ChatID string `yaml:"chatId"`
		} `yaml:"telegram"`
		Ntfy struct {
			URL   string `yaml:"url"`
			Topic string `yaml:"topic"`
			Token string `yaml:"token"`
		} `yaml:"ntfy"`
		Webhook struct {
			URL string `yaml:"url"`
		} `yaml:"webhook"`
	} `yaml:"notify"`
	Defaults struct {
		Lease          string   `yaml:"lease"`
		LeaseOptions   []string `yaml:"leaseOptions"`
		StartTimeout   string   `yaml:"startTimeout"`
		StopGrace      string   `yaml:"stopGrace"`
		PollInterval   string   `yaml:"pollInterval"`
		HealthCacheTTL string   `yaml:"healthCacheTTL"`
		OrphanGrace    string   `yaml:"orphanGrace"`
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
	cfg.Server.ExposeHealthDetail = raw.Server.ExposeHealthDetail
	if raw.Server.TokenSecret != "" {
		cfg.Server.TokenSecret = expandEnv(raw.Server.TokenSecret)
	}
	if cfg.Server.TokenSecret == "" {
		cfg.Server.TokenSecret = os.Getenv("REVEILLE_TOKEN_SECRET")
	}
	if raw.Server.ForwardAuthSecret != "" {
		cfg.Server.ForwardAuthSecret = expandEnv(raw.Server.ForwardAuthSecret)
	}
	if cfg.Server.ForwardAuthSecret == "" {
		cfg.Server.ForwardAuthSecret = os.Getenv("REVEILLE_FORWARD_AUTH_SECRET")
	}
	if raw.Server.StateFile != nil {
		cfg.Server.StateFile = strings.TrimSpace(*raw.Server.StateFile)
	}
	if err := setDuration("server.hostsReloadInterval", raw.Server.HostsReloadInterval, &cfg.Server.HostsReloadInterval); err != nil {
		return cfg, err
	}
	if raw.Log.Level != "" {
		level, err := logging.NormalizeLevel(raw.Log.Level)
		if err != nil {
			return cfg, fmt.Errorf("log.level: %w", err)
		}
		cfg.Log.Level = level
	}
	if raw.Log.Format != "" {
		cfg.Log.Format = strings.ToLower(strings.TrimSpace(raw.Log.Format))
	}
	if raw.Provider != "" {
		cfg.Provider = strings.ToLower(strings.TrimSpace(raw.Provider))
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
	if raw.Docker.Socket != "" {
		cfg.Docker.Socket = raw.Docker.Socket
	}
	if err := setDuration("docker.timeout", raw.Docker.Timeout, &cfg.Docker.Timeout); err != nil {
		return cfg, err
	}
	if raw.Admin.Listen != "" {
		cfg.Admin.Listen = raw.Admin.Listen
	}
	if raw.Admin.Token != "" {
		cfg.Admin.Token = expandEnv(raw.Admin.Token)
	}
	if cfg.Admin.Token == "" {
		cfg.Admin.Token = os.Getenv("REVEILLE_ADMIN_TOKEN")
	}
	cfg.Notify.Events = normalizeEvents(raw.Notify.Events)
	cfg.Notify.Gotify = GotifyConfig{
		URL:      strings.TrimRight(raw.Notify.Gotify.URL, "/"),
		Token:    expandEnv(raw.Notify.Gotify.Token),
		Priority: raw.Notify.Gotify.Priority,
	}
	cfg.Notify.Telegram = TelegramConfig{
		Token:  expandEnv(raw.Notify.Telegram.Token),
		ChatID: expandEnv(raw.Notify.Telegram.ChatID),
	}
	cfg.Notify.Ntfy = NtfyConfig{
		URL:   strings.TrimRight(raw.Notify.Ntfy.URL, "/"),
		Topic: raw.Notify.Ntfy.Topic,
		Token: expandEnv(raw.Notify.Ntfy.Token),
	}
	cfg.Notify.Webhook = WebhookConfig{URL: raw.Notify.Webhook.URL}
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
	if err := setDuration("defaults.healthCacheTTL", raw.Defaults.HealthCacheTTL, &cfg.Defaults.HealthCacheTTL); err != nil {
		return cfg, err
	}
	if err := setDuration("defaults.orphanGrace", raw.Defaults.OrphanGrace, &cfg.Defaults.OrphanGrace); err != nil {
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
		Server: ServerConfig{
			Listen:              ":8080",
			PublicPath:          "/_reveille",
			TokenSecret:         os.Getenv("REVEILLE_TOKEN_SECRET"),
			ForwardAuthSecret:   os.Getenv("REVEILLE_FORWARD_AUTH_SECRET"),
			StateFile:           "/var/lib/reveille/state.json",
			HostsReloadInterval: 5 * time.Second,
		},
		Log:      LogConfig{Level: "info", Format: "text"},
		Provider: "dockhand",
		Dockhand: DockhandConfig{
			BaseURL:  "http://dockhand:3000",
			APIToken: os.Getenv("DOCKHAND_API_TOKEN"),
			Timeout:  30 * time.Second,
		},
		Docker: DockerConfig{
			Socket:  "/var/run/docker.sock",
			Timeout: 30 * time.Second,
		},
		Admin: AdminConfig{Token: os.Getenv("REVEILLE_ADMIN_TOKEN")},
		Defaults: Defaults{
			Lease:          lease,
			LeaseOptions:   options,
			StartTimeout:   3 * time.Minute,
			StopGrace:      30 * time.Second,
			PollInterval:   5 * time.Second,
			HealthCacheTTL: 3 * time.Second,
			OrphanGrace:    10 * time.Minute,
		},
	}
	return cfg
}

// NotifyEvents lists every event type a notifier can subscribe to.
var NotifyEvents = []string{"wake", "lease_expired_stop", "manual_stop", "stop_failed"}

func normalizeEvents(events []string) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.ToLower(strings.TrimSpace(event))
		if event != "" {
			out = append(out, event)
		}
	}
	return out
}

func validate(cfg Config) error {
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if _, err := logging.ParseLevel(cfg.Log.Level); err != nil {
		return fmt.Errorf("log.level: %w", err)
	}
	if cfg.Log.Format != "text" && cfg.Log.Format != "json" {
		return fmt.Errorf("log.format must be text or json")
	}
	if cfg.Server.PublicPath == "" || !strings.HasPrefix(cfg.Server.PublicPath, "/") {
		return fmt.Errorf("server.publicPath must start with /")
	}
	if cfg.Server.HostsReloadInterval <= 0 {
		return fmt.Errorf("server.hostsReloadInterval must be positive")
	}
	switch cfg.Provider {
	case "dockhand":
		if cfg.Dockhand.BaseURL == "" {
			return fmt.Errorf("dockhand.baseUrl is required")
		}
		if cfg.Dockhand.Timeout <= 0 {
			return fmt.Errorf("dockhand.timeout must be positive")
		}
	case "docker":
		if cfg.Docker.Socket == "" {
			return fmt.Errorf("docker.socket is required")
		}
		if cfg.Docker.Timeout <= 0 {
			return fmt.Errorf("docker.timeout must be positive")
		}
	default:
		return fmt.Errorf("provider must be dockhand or docker")
	}
	if cfg.Defaults.StartTimeout <= 0 || cfg.Defaults.PollInterval <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	if cfg.Defaults.HealthCacheTTL < 0 {
		return fmt.Errorf("defaults.healthCacheTTL must not be negative")
	}
	if cfg.Defaults.OrphanGrace <= 0 {
		return fmt.Errorf("defaults.orphanGrace must be positive")
	}
	for _, event := range cfg.Notify.Events {
		if !validNotifyEvent(event) {
			return fmt.Errorf("notify.events: unknown event %q", event)
		}
	}
	if cfg.Notify.Telegram.Token != "" && cfg.Notify.Telegram.ChatID == "" {
		return fmt.Errorf("notify.telegram.chatId is required when a telegram token is set")
	}
	if cfg.Notify.Gotify.URL != "" && cfg.Notify.Gotify.Token == "" {
		return fmt.Errorf("notify.gotify.token is required when a gotify url is set")
	}
	if cfg.Notify.Ntfy.URL != "" && cfg.Notify.Ntfy.Topic == "" {
		return fmt.Errorf("notify.ntfy.topic is required when an ntfy url is set")
	}
	return nil
}

func validNotifyEvent(event string) bool {
	for _, known := range NotifyEvents {
		if event == known {
			return true
		}
	}
	return false
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
	if rest, ok := cutFold(value, "idle:"); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return LeaseDuration{}, err
		}
		if d <= 0 {
			return LeaseDuration{}, fmt.Errorf("idle window must be positive")
		}
		return LeaseDuration{Label: "idle:" + strings.TrimSpace(rest), Duration: d, Idle: true}, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return LeaseDuration{}, err
	}
	return LeaseDuration{Label: value, Duration: d}, nil
}

// Equal reports whether two lease options describe the same lease.
func (l LeaseDuration) Equal(other LeaseDuration) bool {
	return l.Never == other.Never && l.Idle == other.Idle && l.Duration == other.Duration
}

func cutFold(value, prefix string) (string, bool) {
	if len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
		return value[len(prefix):], true
	}
	return "", false
}

func expandEnv(value string) string {
	return os.Expand(value, func(key string) string { return os.Getenv(key) })
}
