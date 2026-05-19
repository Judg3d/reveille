package hosts

import (
	"os"
	"path/filepath"
	"testing"

	"reveille/internal/config"
)

func TestLookupByForwardedHostIgnoresPort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jellyfin.yml"), []byte(`
host: jellyfin.example.com
target:
  type: container
  id: jellyfin
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
host: one.example.com
target:
  type: stack
  name: one
  healthUrl: http://one/
`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadDir(dir, config.DefaultConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`
host: two.example.com
target:
  type: stack
  name: two
  healthUrl: http://two/
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup("one.example.com"); ok {
		t.Fatal("old host still present")
	}
	if _, ok := store.Lookup("two.example.com"); !ok {
		t.Fatal("new host missing")
	}
}
