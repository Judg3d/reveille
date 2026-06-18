package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reveille/internal/config"
)

func TestLookupByForwardedHostIgnoresPort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jellyfin.yml"), []byte(`
target:
  jellyfin:
    type: container
    id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com
    healthUrl: http://jellyfin:8096/health
    healthyStatus:
      - 200
      - 302
`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadDir(dir, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	host, ok := store.Lookup("JELLYFIN.EXAMPLE.COM:443")
	if !ok {
		t.Fatal("host not found")
	}
	if host.Target.HealthyStatus[1] != 302 {
		t.Fatalf("statuses = %+v", host.Target.HealthyStatus)
	}
}

func TestReloadReplacesHostMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  one:
    type: stack
    environment: homelab
    hostname: one.example.com
    healthUrl: http://one/
`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadDir(dir, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`
target:
  two:
    type: stack
    environment: homelab
    hostname: two.example.com
    healthUrl: http://two/
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup("one.example.com"); ok {
		t.Fatal("old host still present")
	}
	if _, ok := store.Lookup("two.example.com"); !ok {
		t.Fatal("new host missing")
	}
}

func TestContainerHostCanUseTargetHostnameAndDockhandHealth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yml"), []byte(`
target:
  app:
    type: container
    id: app
    environment: prod
    hostname: app.example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadDir(dir, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	host, ok := store.Lookup("app.example.com")
	if !ok {
		t.Fatal("host not found")
	}
	if host.Target.Environment != "prod" || host.Target.HealthURL != "" {
		t.Fatalf("target = %+v", host.Target)
	}
}

func TestLoadFileRejectsTargetsList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	if err := os.WriteFile(path, []byte(`
targets:
  - id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com
  - id: sonarr
    environment: homelab
    hostname: sonarr.example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil {
		t.Fatal("expected targets list format to be rejected")
	}
}

func TestLoadFileSupportsNamedTargetsMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	if err := os.WriteFile(path, []byte(`
target:
  convertx:
    type: stack
    environment: homelab
    hostname: convert.example.com
    healthUrl: http://10.0.0.50:3003/healthcheck
  app2:
    type: stack
    environment: homelab
    hostname: convert2.example.com
    healthUrl: http://10.0.0.50:3002/healthcheck
`), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := LoadFile(path, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts = %+v", hosts)
	}
	if hosts[0].Target.Name != "convertx" || hosts[0].Host != "convert.example.com" {
		t.Fatalf("first host = %+v", hosts[0])
	}
	if hosts[1].Target.Name != "app2" || hosts[1].Host != "convert2.example.com" {
		t.Fatalf("second host = %+v", hosts[1])
	}
}

func TestLoadFileNamedTargetsPreservesExplicitName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	if err := os.WriteFile(path, []byte(`
target:
  convertx:
    type: stack
    name: convertx-prod
    environment: homelab
    hostname: convert.example.com
    healthUrl: http://10.0.0.50:3003/healthcheck
`), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := LoadFile(path, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts = %+v", hosts)
	}
	if hosts[0].Target.Name != "convertx-prod" {
		t.Fatalf("target = %+v", hosts[0].Target)
	}
}

func TestLoadFileRejectsUnsupportedHealthURLScheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  app:
    type: stack
    environment: homelab
    hostname: app.example.com
    healthUrl: file:///etc/passwd
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil || !strings.Contains(err.Error(), "scheme must be http or https") {
		t.Fatalf("LoadFile() err = %v, want scheme validation error", err)
	}
}

func TestLoadFileRejectsHealthURLWithoutHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  app:
    type: stack
    environment: homelab
    hostname: app.example.com
    healthUrl: http:///health
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("LoadFile() err = %v, want host validation error", err)
	}
}

func TestLoadFileRejectsHealthURLCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  app:
    type: stack
    environment: homelab
    hostname: app.example.com
    healthUrl: https://user:pass@app.example.com/health
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil || !strings.Contains(err.Error(), "credentials are not allowed") {
		t.Fatalf("LoadFile() err = %v, want credentials validation error", err)
	}
}

func TestLoadFileTrimsHealthURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  app:
    type: stack
    environment: homelab
    hostname: app.example.com
    healthUrl: " https://app.example.com/health#local "
`), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := LoadFile(path, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if got := hosts[0].Target.HealthURL; got != "https://app.example.com/health" {
		t.Fatalf("healthUrl = %q, want normalized URL", got)
	}
}

func TestLoadFileRejectsMissingEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
target:
  app:
    type: container
    id: app
    hostname: app.example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil || !strings.Contains(err.Error(), "target.environment is required") {
		t.Fatalf("LoadFile() err = %v, want environment validation error", err)
	}
}

func TestLoadFileRejectsSingleTargetObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(path, []byte(`
host: jellyfin.example.com
target:
  id: jellyfin
  environment: homelab
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, config.DefaultConfig().Defaults); err == nil {
		t.Fatal("expected single target object format to be rejected")
	}
}
