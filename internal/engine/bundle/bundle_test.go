package bundle

import (
	"context"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

func TestBundleEnginePlan(t *testing.T) {
	r := runner.New()
	eng := New(r)

	if eng.ID() != "bundle" {
		t.Fatalf("unexpected engine ID: %s", eng.ID())
	}

	// Flatpak plan should NOT require root approval
	flatpakPlan := &domain.InstallPlan{
		AppID: "org.example.app",
		Package: domain.PackageInfo{
			Name:   "org.example.app",
			Format: domain.FormatFlatpak,
			Path:   "/tmp/app.flatpakref",
		},
	}
	ctx := context.Background()
	if err := eng.Plan(ctx, flatpakPlan); err != nil {
		t.Fatalf("flatpak plan failed: %v", err)
	}
	if flatpakPlan.RequiresApproval {
		t.Fatalf("flatpak user install should not require root approval")
	}

	// Snap plan MUST require single root approval
	snapPlan := &domain.InstallPlan{
		AppID: "example-snap",
		Package: domain.PackageInfo{
			Name:   "example-snap",
			Format: domain.FormatSnap,
			Path:   "/tmp/example.snap",
		},
	}
	if err := eng.Plan(ctx, snapPlan); err != nil {
		t.Fatalf("snap plan failed: %v", err)
	}
	if !snapPlan.RequiresApproval {
		t.Fatalf("snap install must require root approval")
	}
}
