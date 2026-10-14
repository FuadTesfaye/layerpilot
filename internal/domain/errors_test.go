package domain

import (
	"errors"
	"testing"
)

func TestStructuredError(t *testing.T) {
	origErr := errors.New("underlying disk failure")
	wrapped := WrapError(ErrCodeDatabaseError, "failed to query applications", origErr)

	if wrapped.Code != ErrCodeDatabaseError {
		t.Fatalf("expected code %s, got %s", ErrCodeDatabaseError, wrapped.Code)
	}

	if !errors.Is(wrapped, origErr) {
		t.Fatalf("expected wrapped error to match origErr via errors.Is")
	}

	customErr := NewError(ErrCodeEngineUnavailable, "podman not found in PATH", "Install podman via package manager", "Cannot execute container strategies")
	expectedStr := "[ENGINE_UNAVAILABLE] podman not found in PATH (Suggestion: Install podman via package manager)"
	if customErr.Error() != expectedStr {
		t.Fatalf("expected %q, got %q", expectedStr, customErr.Error())
	}
}
