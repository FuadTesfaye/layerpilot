package native

import (
	"context"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

func TestNativeEnginePlan(t *testing.T) {
	r := runner.New()
	eng := New(r)

	if eng.ID() != "native" {
		t.Fatalf("unexpected engine ID: %s", eng.ID())
	}

	plan := &domain.InstallPlan{
		AppID: "test-pkg",
		Package: domain.PackageInfo{
			Name:   "test-pkg",
			Format: domain.FormatRPM,
			SHA256: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		Decision: domain.StrategyDecision{
			StrategyID:        "S3",
			SingleRootCommand: []string{"pkexec", "dnf", "install", "-y", "/tmp/test.rpm"},
		},
	}

	ctx := context.Background()
	if err := eng.Plan(ctx, plan); err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	if !plan.RequiresApproval {
		t.Fatalf("native plan must require approval")
	}

	if len(plan.PrivilegedOperations) != 1 {
		t.Fatalf("expected 1 privileged operation, got %d", len(plan.PrivilegedOperations))
	}

	if plan.PrivilegedOperations[0] != "pkexec dnf install -y /tmp/test.rpm" {
		t.Fatalf("unexpected privileged operation string: %s", plan.PrivilegedOperations[0])
	}
}
