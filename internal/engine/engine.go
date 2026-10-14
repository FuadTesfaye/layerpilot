package engine

import (
	"context"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// Engine defines the pluggable lifecycle contract for compatibility backends
type Engine interface {
	// ID returns the unique machine identifier for the engine (e.g. "podman", "appimage", "wine")
	ID() string

	// Name returns human-readable engine name
	Name() string

	// Probe checks if the engine is available and healthy on the host
	Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error)

	// Plan builds engine-specific plan steps, container definitions, and resource reservations
	Plan(ctx context.Context, plan *domain.InstallPlan) error

	// Apply executes the plan within the engine, configuring containers, permissions, or files
	Apply(ctx context.Context, plan *domain.InstallPlan) error

	// Launch executes an installed application with user arguments
	Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error

	// Uninstall removes all resources owned by the application
	Uninstall(ctx context.Context, app *domain.ApplicationRecord) error

	// Status queries the application's current runtime or installation state
	Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error)

	// Logs retrieves diagnostic logs for the application
	Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error)
}
