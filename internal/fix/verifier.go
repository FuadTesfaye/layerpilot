package fix

import (
	"context"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// VerificationResult contains the detailed outcome of an active verification test
type VerificationResult struct {
	Success          bool          `json:"success"`
	Uptime           time.Duration `json:"uptime"`
	WindowDetected   bool          `json:"window_detected"`
	WindowProbeUsed  string        `json:"window_probe_used,omitempty"`
	ProbeUnavailable bool          `json:"probe_unavailable"`
	ExitCode         int           `json:"exit_code"`
	Message          string        `json:"message"`
	CapturedLog      string        `json:"captured_log,omitempty"`
}

// Verifier executes active health verification on an application
type Verifier interface {
	Verify(ctx context.Context, app *domain.ApplicationRecord, launcherPath string) (*VerificationResult, error)
}
