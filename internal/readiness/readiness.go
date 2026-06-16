package readiness

type State string

const (
	StateReady             State = "ready"
	StateWaitingForHealth  State = "waiting_for_health"
	StateHealthUnreachable State = "health_unreachable"
	StateHealthUnhealthy   State = "health_unhealthy"
)

type Snapshot struct {
	Healthy      bool
	LeaseActive  bool
	Never        bool
	HealthError  string
	HealthStatus int
}

type Evaluation struct {
	State   State
	Message string
}

func Evaluate(snapshot Snapshot) Evaluation {
	state := StateFor(snapshot)
	return Evaluation{
		State:   state,
		Message: Message(snapshot, state),
	}
}

func StateFor(snapshot Snapshot) State {
	switch {
	case snapshot.Healthy:
		return StateReady
	case snapshot.HealthError != "":
		return StateHealthUnreachable
	case snapshot.HealthStatus != 0:
		return StateHealthUnhealthy
	default:
		return StateWaitingForHealth
	}
}

func Message(snapshot Snapshot, state State) string {
	switch {
	case snapshot.Healthy && snapshot.LeaseActive:
		return "App is ready. Redirecting now."
	case snapshot.Healthy:
		return "App is ready. Start a timer to continue."
	case snapshot.LeaseActive && state == StateHealthUnreachable && snapshot.Never:
		return "App start was requested, but Reveille cannot reach the health endpoint yet. Automatic stop is disabled."
	case snapshot.LeaseActive && state == StateHealthUnreachable:
		return "App start was requested, but Reveille cannot reach the health endpoint yet."
	case snapshot.LeaseActive && state == StateHealthUnhealthy && snapshot.Never:
		return "App start was requested, but the health endpoint is responding with a non-healthy status. Automatic stop is disabled."
	case snapshot.LeaseActive && state == StateHealthUnhealthy:
		return "App start was requested, but the health endpoint is responding with a non-healthy status."
	case snapshot.LeaseActive && snapshot.Never:
		return "App start was requested. Waiting for health check before redirect. Automatic stop is disabled."
	case snapshot.LeaseActive:
		return "App start was requested. Waiting for health check before redirect."
	case state == StateHealthUnreachable:
		return "Choose a timer to continue. Reveille is still waiting for the health endpoint."
	case state == StateHealthUnhealthy:
		return "Choose a timer to continue. Reveille reached the app, but the health endpoint is not healthy yet."
	default:
		return "Choose a timer to continue. Reveille is starting the app in the background."
	}
}
