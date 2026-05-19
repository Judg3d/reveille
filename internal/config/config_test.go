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
dockhand:
  baseUrl: "http://dockhand.local"
  apiToken: "${DOCKHAND_API_TOKEN}"
  environmentId: 7
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
	if cfg.Server.Listen != ":9090" || cfg.Dockhand.APIToken != "dh_test" || cfg.Dockhand.EnvironmentID != 7 {
		t.Fatalf("unexpected config: %+v", cfg)
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
