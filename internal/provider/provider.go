package provider

import (
	"context"

	"reveille/internal/hosts"
)

// Provider starts, stops, and health-checks managed targets. Dockhand and the
// direct Docker socket client both implement it.
type Provider interface {
	Name() string
	Start(ctx context.Context, target hosts.Target) error
	Stop(ctx context.Context, target hosts.Target) error
	Healthy(ctx context.Context, target hosts.Target) (bool, error)
}
