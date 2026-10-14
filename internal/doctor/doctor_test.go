package doctor

import (
	"context"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/hostinfo"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

func TestDoctorDiagnose(t *testing.T) {
	p := hostinfo.New()
	reg := engine.NewRegistry()
	r := runner.New()

	doc := New(p, reg, r)
	report := doc.Diagnose(context.Background())

	if len(report.Checks) == 0 {
		t.Fatalf("expected diagnostic checks in report, got none")
	}

	foundHostCheck := false
	for _, c := range report.Checks {
		if c.Category == "Host" {
			foundHostCheck = true
			break
		}
	}

	if !foundHostCheck {
		t.Errorf("expected Host check category in doctor report")
	}
}
