package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func TestRunnerSuccess(t *testing.T) {
	r := New()
	ctx := context.Background()

	res, err := r.Run(ctx, "echo", "hello", "world")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", res.Stdout)
	}
}

func TestRunnerExitCode(t *testing.T) {
	r := New()
	ctx := context.Background()

	res, err := r.Run(ctx, "false")
	if err == nil {
		t.Fatalf("expected error on false command, got nil")
	}
	if res.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", res.ExitCode)
	}

	compatErr, ok := err.(*domain.CompatError)
	if !ok {
		t.Fatalf("expected *domain.CompatError, got %T", err)
	}
	if compatErr.Code != domain.ErrCodeRunnerFailed {
		t.Errorf("expected code %s, got %s", domain.ErrCodeRunnerFailed, compatErr.Code)
	}
}

func TestRunnerTimeout(t *testing.T) {
	r := New()
	ctx := context.Background()

	_, err := r.RunWithOptions(ctx, RunOptions{
		Command: "sleep",
		Args:    []string{"1"},
		Timeout: 50 * time.Millisecond,
	})

	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}

	compatErr, ok := err.(*domain.CompatError)
	if !ok {
		t.Fatalf("expected *domain.CompatError, got %T", err)
	}
	if compatErr.Code != domain.ErrCodeRunnerTimeout {
		t.Errorf("expected code %s, got %s", domain.ErrCodeRunnerTimeout, compatErr.Code)
	}
}

func TestRunnerBufferCap(t *testing.T) {
	r := New()
	ctx := context.Background()

	// Output 2000 lines, capped at 256 bytes
	res, err := r.RunWithOptions(ctx, RunOptions{
		Command:        "sh",
		Args:           []string{"-c", "for i in $(seq 1 500); do echo \"line $i\"; done"},
		MaxOutputBytes: 256,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Stdout) > 256 {
		t.Errorf("expected stdout to be capped at 256 bytes, got %d bytes", len(res.Stdout))
	}
}
