package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadParsesDurationsNeverAndEnv(t *testing.T) {
	t.Setenv("DOCKHAND_API_TOKEN", "dh_test")
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`
server:
  listen: ":9090"
  failClosedUnknownHosts: true
log:
  level: "warning"
dockhand:
  baseUrl: "http://dockhand.local"
  apiToken: "${DOCKHAND_API_TOKEN}"
  timeout: "10s"
defaults:
  lease: "1h"
  leaseOptions:
    - "30m"
    - "never"
  startTimeout: "45s"
  stopGrace: "5s"
  pollInterval: "2s"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != ":9090" || cfg.Dockhand.APIToken != "dh_test" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.Server.FailClosedUnknownHosts {
		t.Fatal("failClosedUnknownHosts = false, want true")
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("log level = %q", cfg.Log.Level)
	}
	if cfg.Defaults.Lease.Duration != time.Hour {
		t.Fatalf("lease = %v", cfg.Defaults.Lease.Duration)
	}
	if len(cfg.Defaults.LeaseOptions) != 2 || !cfg.Defaults.LeaseOptions[1].Never {
		t.Fatalf("lease options = %+v", cfg.Defaults.LeaseOptions)
	}
	if cfg.Defaults.PollInterval != 2*time.Second {
		t.Fatalf("poll interval = %v", cfg.Defaults.PollInterval)
	}
}

func TestLoadUsesDockhandTokenFromEnvironmentWhenConfigOmitsIt(t *testing.T) {
	t.Setenv("DOCKHAND_API_TOKEN", "dh_from_env")
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`
dockhand:
  baseUrl: "http://dockhand.local"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dockhand.APIToken != "dh_from_env" {
		t.Fatalf("api token = %q", cfg.Dockhand.APIToken)
	}
}

func TestParseLeaseDurationNever(t *testing.T) {
	lease, err := ParseLeaseDuration("never")
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Never || lease.Label != "Never" {
		t.Fatalf("lease = %+v", lease)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`
log:
  level: "verbose"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid log level to fail")
	}
}

func TestLoadParsesNewServerAndNotifyFields(t *testing.T) {
	t.Setenv("REVEILLE_TOKEN_SECRET", "tok-secret")
	t.Setenv("GOTIFY_TOKEN", "go-tok")
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`
server:
  forwardAuthSecret: "fa-secret"
  exposeHealthDetail: true
  stateFile: "/tmp/reveille-state.json"
  hostsReloadInterval: "10s"
provider: "docker"
docker:
  socket: "/run/docker.sock"
admin:
  listen: ":8081"
  token: "admin-tok"
notify:
  events: ["stop_failed", "wake"]
  gotify:
    url: "https://gotify.local/"
    token: "${GOTIFY_TOKEN}"
  telegram:
    token: "tg-tok"
    chatId: "-100"
defaults:
  leaseOptions:
    - "30m"
    - "idle:20m"
    - "never"
  healthCacheTTL: "2s"
  orphanGrace: "5m"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.TokenSecret != "tok-secret" {
		t.Fatalf("tokenSecret = %q", cfg.Server.TokenSecret)
	}
	if cfg.Server.ForwardAuthSecret != "fa-secret" || !cfg.Server.ExposeHealthDetail {
		t.Fatalf("server = %+v", cfg.Server)
	}
	if cfg.Server.StateFile != "/tmp/reveille-state.json" || cfg.Server.HostsReloadInterval != 10*time.Second {
		t.Fatalf("server = %+v", cfg.Server)
	}
	if cfg.Provider != "docker" || cfg.Docker.Socket != "/run/docker.sock" {
		t.Fatalf("provider = %q docker = %+v", cfg.Provider, cfg.Docker)
	}
	if cfg.Admin.Listen != ":8081" || cfg.Admin.Token != "admin-tok" {
		t.Fatalf("admin = %+v", cfg.Admin)
	}
	if len(cfg.Notify.Events) != 2 || cfg.Notify.Gotify.Token != "go-tok" || cfg.Notify.Telegram.ChatID != "-100" {
		t.Fatalf("notify = %+v", cfg.Notify)
	}
	if cfg.Defaults.HealthCacheTTL != 2*time.Second || cfg.Defaults.OrphanGrace != 5*time.Minute {
		t.Fatalf("defaults = %+v", cfg.Defaults)
	}
	idle := cfg.Defaults.LeaseOptions[1]
	if !idle.Idle || idle.Duration != 20*time.Minute || idle.Label != "idle:20m" {
		t.Fatalf("idle option = %+v", idle)
	}
}

func TestLoadDisablesStatePersistenceWithEmptyStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`
server:
  stateFile: ""
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.StateFile != "" {
		t.Fatalf("stateFile = %q, want empty (disabled)", cfg.Server.StateFile)
	}
}

func TestLoadRejectsUnknownProviderAndEvents(t *testing.T) {
	dir := t.TempDir()
	badProvider := filepath.Join(dir, "provider.yml")
	if err := os.WriteFile(badProvider, []byte("provider: \"podman\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badProvider); err == nil {
		t.Fatal("expected unknown provider to fail")
	}

	badEvent := filepath.Join(dir, "event.yml")
	if err := os.WriteFile(badEvent, []byte("notify:\n  events: [\"explode\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badEvent); err == nil {
		t.Fatal("expected unknown notify event to fail")
	}
}

func TestParseIdleLeaseDuration(t *testing.T) {
	lease, err := ParseLeaseDuration("idle:45m")
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Idle || lease.Duration != 45*time.Minute || lease.Never {
		t.Fatalf("lease = %+v", lease)
	}
	if _, err := ParseLeaseDuration("idle:banana"); err == nil {
		t.Fatal("expected invalid idle duration to fail")
	}
}
