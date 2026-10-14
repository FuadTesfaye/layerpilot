package compat

import (
	"context"

	"github.com/FuadTesfaye/layerpilot/internal/app"
	"github.com/FuadTesfaye/layerpilot/internal/doctor"
	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// Client provides the public Go API for universal application compatibility
type Client struct {
	workflow *app.Workflow
}

// NewClient initializes a new compat client instance
func NewClient() (*Client, error) {
	wf, err := app.NewDefaultWorkflow()
	if err != nil {
		return nil, err
	}
	return &Client{workflow: wf}, nil
}

// Inspect identifies file format, architecture, and package metadata without installing
func (c *Client) Inspect(ctx context.Context, pkgPath string) (*domain.PackageInfo, error) {
	return c.workflow.Inspect(ctx, pkgPath)
}

// Support evaluates compatibility strategies and support tiers
func (c *Client) Support(ctx context.Context, pkgPath string) (*domain.StrategyDecision, error) {
	return c.workflow.Support(ctx, pkgPath)
}

// Plan builds a deterministic installation plan
func (c *Client) Plan(ctx context.Context, pkgPath string) (*domain.InstallPlan, error) {
	return c.workflow.Plan(ctx, pkgPath)
}

// Install previews, executes the plan within the selected engine, and records application state
func (c *Client) Install(ctx context.Context, pkgPath string, autoApprove bool) (*domain.ApplicationRecord, error) {
	return c.workflow.Install(ctx, pkgPath, autoApprove)
}

// List returns all managed applications
func (c *Client) List(ctx context.Context) ([]domain.ApplicationRecord, error) {
	return c.workflow.List(ctx)
}

// Launch executes an installed application
func (c *Client) Launch(ctx context.Context, appID string, args []string) error {
	return c.workflow.Launch(ctx, appID, args)
}

// Status returns current application status
func (c *Client) Status(ctx context.Context, appID string) (domain.LifecycleState, error) {
	return c.workflow.Status(ctx, appID)
}

// Logs returns application logs
func (c *Client) Logs(ctx context.Context, appID string, lines int) ([]string, error) {
	return c.workflow.Logs(ctx, appID, lines)
}

// Uninstall cleanly removes an application and all owned resources
func (c *Client) Uninstall(ctx context.Context, appID string, autoApprove bool) error {
	return c.workflow.Uninstall(ctx, appID, autoApprove)
}

// Doctor evaluates system health and prerequisites
func (c *Client) Doctor(ctx context.Context) *doctor.DoctorReport {
	return c.workflow.Doctor(ctx)
}

// Close closes any open storage resources
func (c *Client) Close() error {
	return c.workflow.Close()
}
