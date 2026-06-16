package readiness

import "testing"

func TestEvaluateReadyWithLease(t *testing.T) {
	got := Evaluate(Snapshot{Healthy: true, LeaseActive: true})
	if got.State != StateReady {
		t.Fatalf("state = %q, want %q", got.State, StateReady)
	}
	if got.Message != "App is ready. Redirecting now." {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestEvaluateHealthyWithoutLeaseNeedsTimer(t *testing.T) {
	got := Evaluate(Snapshot{Healthy: true})
	if got.State != StateReady {
		t.Fatalf("state = %q, want %q", got.State, StateReady)
	}
	if got.Message != "App is ready. Start a timer to continue." {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestEvaluateUnhealthyLease(t *testing.T) {
	got := Evaluate(Snapshot{LeaseActive: true, HealthStatus: 503})
	if got.State != StateHealthUnhealthy {
		t.Fatalf("state = %q, want %q", got.State, StateHealthUnhealthy)
	}
	if got.Message != "App start was requested, but the health endpoint is responding with a non-healthy status." {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestEvaluateUnreachableNoLease(t *testing.T) {
	got := Evaluate(Snapshot{HealthError: "connection refused"})
	if got.State != StateHealthUnreachable {
		t.Fatalf("state = %q, want %q", got.State, StateHealthUnreachable)
	}
	if got.Message != "Choose a timer to continue. Reveille is still waiting for the health endpoint." {
		t.Fatalf("message = %q", got.Message)
	}
}
