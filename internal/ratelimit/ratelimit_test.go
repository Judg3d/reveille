package ratelimit

import (
	"testing"
	"time"
)

func TestBurstThenDeny(t *testing.T) {
	l := New(1, 3)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("key") {
			t.Fatalf("request %d denied within burst", i+1)
		}
	}
	if l.Allow("key") {
		t.Fatal("request beyond burst allowed")
	}
}

func TestRefillOverTime(t *testing.T) {
	l := New(1, 1)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	if !l.Allow("key") {
		t.Fatal("first request denied")
	}
	if l.Allow("key") {
		t.Fatal("second immediate request allowed")
	}
	now = now.Add(2 * time.Second)
	if !l.Allow("key") {
		t.Fatal("request after refill denied")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(1, 1)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	if !l.Allow("a") {
		t.Fatal("first key denied")
	}
	if !l.Allow("b") {
		t.Fatal("second key denied")
	}
}
