package planner

import (
	"context"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/matrix"
)

// FakeEngine is a test double implementing domain.Engine
type FakeEngine struct {
	id        string
	available bool
}

func (f *FakeEngine) ID() string   { return f.id }
func (f *FakeEngine) Name() string { return "Fake Engine " + f.id }
func (f *FakeEngine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	return &domain.CapabilityReport{
		EngineID:  f.id,
		Available: f.available,
	}, nil
}
func (f *FakeEngine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	plan.Steps = append(plan.Steps, domain.PlanStep{
		Order:       1,
		Name:        "FakeStep",
		Description: "Simulate container creation",
	})
	plan.HostChanges = append(plan.HostChanges, "fake-container")
	return nil
}
func (f *FakeEngine) Apply(ctx context.Context, plan *domain.InstallPlan) error { return nil }
func (f *FakeEngine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	return nil
}
func (f *FakeEngine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error { return nil }
func (f *FakeEngine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	return domain.StateInstalled, nil
}
func (f *FakeEngine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return nil, nil
}

func TestPlannerSuccessfulPlan(t *testing.T) {
	pe := matrix.New()
	reg := engine.NewRegistry()
	reg.Register(&FakeEngine{id: "podman", available: true})

	p := New(pe, reg)
	ctx := context.Background()

	pkg := &domain.PackageInfo{
		Name:         "htop",
		Format:       domain.FormatDeb,
		Architecture: "amd64",
		SHA256:       "abcdef1234567890",
	}
	host := &domain.HostProfile{
		OS:            "linux",
		Distro:        "fedora",
		Arch:          "amd64",
		DisplayServer: "wayland",
	}

	plan, err := p.Plan(ctx, pkg, host)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	if plan.Decision.SupportTier != domain.Tier1Supported {
		t.Errorf("expected Tier 1, got %s", plan.Decision.SupportTier)
	}
	if len(plan.Steps) == 0 {
		t.Errorf("expected non-empty steps")
	}
	if plan.AppID != "htop-abcdef12" {
		t.Errorf("expected app ID 'htop-abcdef12', got '%s'", plan.AppID)
	}
}

func TestPlannerUnsupportedRefusal(t *testing.T) {
	pe := matrix.New()
	reg := engine.NewRegistry()

	p := New(pe, reg)
	ctx := context.Background()

	pkg := &domain.PackageInfo{
		Name:         "Safari",
		Format:       domain.FormatMacOSApp,
		Architecture: "amd64",
	}
	host := &domain.HostProfile{
		OS:   "linux",
		Arch: "amd64",
	}

	_, err := p.Plan(ctx, pkg, host)
	if err == nil {
		t.Fatalf("expected plan refusal error for macOS on Linux, got nil")
	}

	compatErr, ok := err.(*domain.CompatError)
	if !ok {
		t.Fatalf("expected *domain.CompatError, got %T", err)
	}
	if compatErr.Code != domain.ErrCodeExecutionRefused {
		t.Errorf("expected %s, got %s", domain.ErrCodeExecutionRefused, compatErr.Code)
	}
}
