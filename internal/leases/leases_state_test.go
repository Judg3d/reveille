package leases

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reveille/internal/config"
	"reveille/internal/hosts"
)

func TestPersistedLeaseSurvivesRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	host := hosts.Host{Host: "app.example.com", Target: hosts.Target{Type: "container", ID: "app", Environment: "1"}}

	m1 := NewManager(func(context.Context, hosts.Host) error { return nil })
	m1.SetStatePath(statePath)
	now := time.Now()
	set := m1.Set(host, config.LeaseDuration{Label: "1h", Duration: time.Hour}, now)
	m1.Close()

	m2 := NewManager(func(context.Context, hosts.Host) error { return nil })
	m2.SetStatePath(statePath)
	if err := m2.Load(time.Now()); err != nil {
		t.Fatalf("load state: %v", err)
	}
	restored, ok := m2.Get(host.Host)
	if !ok {
		t.Fatal("lease not restored")
	}
	if !restored.ExpiresAt.Equal(set.ExpiresAt) {
		t.Fatalf("expiresAt = %v, want %v", restored.ExpiresAt, set.ExpiresAt)
	}
	if restored.Label != "1h" {
		t.Fatalf("label = %q", restored.Label)
	}
	m2.Close()
}

func TestExpiredPersistedLeaseStopsOnLoad(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	host := hosts.Host{Host: "app.example.com", Target: hosts.Target{Type: "container", ID: "app", Environment: "1"}}

	m1 := NewManager(func(context.Context, hosts.Host) error { return nil })
	m1.SetStatePath(statePath)
	m1.Set(host, config.LeaseDuration{Label: "10ms", Duration: 10 * time.Millisecond}, time.Now())
	// Close before expiry so the lease persists as still-active.
	m1.Close()

	stopped := make(chan string, 1)
	m2 := NewManager(func(_ context.Context, h hosts.Host) error {
		stopped <- h.Host
		return nil
	})
	m2.SetStatePath(statePath)
	if err := m2.Load(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("load state: %v", err)
	}
	select {
	case got := <-stopped:
		if got != host.Host {
			t.Fatalf("stopped %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expired lease was not stopped after load")
	}
}

func TestProvisionalLeaseIsReplacedNotDuplicated(t *testing.T) {
	m := NewManager(func(context.Context, hosts.Host) error { return nil })
	host := hosts.Host{Host: "app.example.com"}
	now := time.Now()

	first, created := m.EnsureProvisional(host, config.LeaseDuration{Label: "grace", Duration: time.Minute}, now)
	if !created || !first.Provisional {
		t.Fatalf("first = %+v created=%t", first, created)
	}
	second, created := m.EnsureProvisional(host, config.LeaseDuration{Label: "grace", Duration: time.Minute}, now.Add(time.Second))
	if created {
		t.Fatalf("second EnsureProvisional created a new lease: %+v", second)
	}

	chosen := m.Set(host, config.LeaseDuration{Label: "2h", Duration: 2 * time.Hour}, now)
	if chosen.Provisional {
		t.Fatal("user lease still marked provisional")
	}
	active, _ := m.Get(host.Host)
	if active.Label != "2h" {
		t.Fatalf("active = %+v", active)
	}
	m.Close()
}

func TestIdleLeaseSlidesWithActivity(t *testing.T) {
	stopped := make(chan struct{}, 1)
	m := NewManager(func(context.Context, hosts.Host) error {
		stopped <- struct{}{}
		return nil
	})
	host := hosts.Host{Host: "app.example.com"}
	m.Set(host, config.LeaseDuration{Label: "idle:60ms", Duration: 60 * time.Millisecond, Idle: true}, time.Now())

	// Keep touching within the idle window; the lease must not expire.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		m.Touch(host.Host, time.Now())
		select {
		case <-stopped:
			t.Fatal("idle lease stopped despite activity")
		case <-time.After(20 * time.Millisecond):
		}
	}

	// Once activity ceases the lease must expire and stop the target.
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("idle lease did not stop after inactivity")
	}
	if _, ok := m.Get(host.Host); ok {
		t.Fatal("idle lease still active after stop")
	}
}

func TestUpdateHostsRefreshesSnapshot(t *testing.T) {
	m := NewManager(func(context.Context, hosts.Host) error { return nil })
	host := hosts.Host{Host: "app.example.com", Target: hosts.Target{ID: "old"}}
	m.Set(host, config.LeaseDuration{Label: "1h", Duration: time.Hour}, time.Now())

	m.UpdateHosts(func(name string) (hosts.Host, bool) {
		return hosts.Host{Host: name, Target: hosts.Target{ID: "new"}}, true
	})

	m.mu.Lock()
	got := m.leases["app.example.com"].host.Target.ID
	m.mu.Unlock()
	if got != "new" {
		t.Fatalf("target id = %q, want %q", got, "new")
	}
	m.Close()
}
