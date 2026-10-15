//go:build windows

package fix

import (
	"context"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

type DefaultVerifier struct {
	MinUptime    time.Duration
	PollInterval time.Duration
}

func NewDefaultVerifier() *DefaultVerifier {
	return &DefaultVerifier{
		MinUptime:    10 * time.Second,
		PollInterval: 500 * time.Millisecond,
	}
}

func (v *DefaultVerifier) Verify(ctx context.Context, app *domain.ApplicationRecord, launcherPath string) (*VerificationResult, error) {
	return &VerificationResult{
		Success: true,
		Message: "Verification stub on Windows",
	}, nil
}
