package leases

import (
	"context"
	"testing"
	"time"

	"reveille/internal/config"
	"reveille/internal/hosts"
)

func TestFiniteLeaseExpiresAndStops(t *testing.T) {
	done := make(chan string, 1)
	m := NewManager(func(_ context.Context, host hosts.Host) error {
		done <- host.Host
		return nil
	})
	host := hosts.Host{Host: "app.example.com"}
	m.Set(host, config.LeaseDuration{Label: "10ms", Duration: 10 * time.Millisecond}, time.Now())

	select {
	case got := <-done:
		if got != host.Host {
			t.Fatalf("stopped %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not expire")
	}
	if _, ok := m.Get(host.Host); ok {
		t.Fatal("expired lease still active")
	}
}

func TestNeverLeaseDoesNotScheduleStop(t *testing.T) {
	called := make(chan struct{}, 1)
	m := NewManager(func(context.Context, hosts.Host) error {
		called <- struct{}{}
		return nil
	})
	host := hosts.Host{Host: "app.example.com"}
	active := m.Set(host, config.LeaseDuration{Label: "Never", Never: true}, time.Now())
	if !active.Never {
		t.Fatalf("active = %+v", active)
	}
	select {
	case <-called:
		t.Fatal("stop was called")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestCloseStopsTimers(t *testing.T) {
	called := make(chan struct{}, 1)
	m := NewManager(func(context.Context, hosts.Host) error {
		called <- struct{}{}
		return nil
	})
	host := hosts.Host{Host: "app.example.com"}
	m.Set(host, config.LeaseDuration{Label: "10ms", Duration: 10 * time.Millisecond}, time.Now())
	m.Close()

	select {
	case <-called:
		t.Fatal("stop was called after manager close")
	case <-time.After(25 * time.Millisecond):
	}
}
