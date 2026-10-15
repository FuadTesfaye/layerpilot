package domain

import (
	"fmt"
)

// ErrorCode is a stable identifier for machine-readable error classification
type ErrorCode string

const (
	ErrCodePackageNotFound       ErrorCode = "PACKAGE_NOT_FOUND"
	ErrCodePackageInvalid        ErrorCode = "PACKAGE_INVALID"
	ErrCodeFormatUnsupported     ErrorCode = "FORMAT_UNSUPPORTED"
	ErrCodeArchMismatch          ErrorCode = "ARCHITECTURE_MISMATCH"
	ErrCodeEngineUnavailable     ErrorCode = "ENGINE_UNAVAILABLE"
	ErrCodeExecutionRefused      ErrorCode = "EXECUTION_REFUSED"
	ErrCodeApprovalRequired      ErrorCode = "APPROVAL_REQUIRED"
	ErrCodeAppNotFound           ErrorCode = "APPLICATION_NOT_FOUND"
	ErrCodeStateTransitionFailed ErrorCode = "STATE_TRANSITION_FAILED"
	ErrCodeStateCorrupt          ErrorCode = "STATE_CORRUPT"
	ErrCodeRunnerTimeout         ErrorCode = "RUNNER_TIMEOUT"
	ErrCodeRunnerFailed          ErrorCode = "RUNNER_FAILED"
	ErrCodePrivilegeDenied       ErrorCode = "PRIVILEGE_DENIED"
	ErrCodeDatabaseError         ErrorCode = "DATABASE_ERROR"
)

// CompatError provides structured, explainable errors with actionable remediation
type CompatError struct {
	Code       ErrorCode         `json:"code"`
	Message    string            `json:"message"`
	Impact     string            `json:"impact,omitempty"`
	Suggestion string            `json:"suggestion,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	Err        error             `json:"-"`
}

func (e *CompatError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("[%s] %s (Suggestion: %s)", e.Code, e.Message, e.Suggestion)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *CompatError) Unwrap() error {
	return e.Err
}

// NewError creates a new CompatError
func NewError(code ErrorCode, message string, suggestion string, impact string) *CompatError {
	return &CompatError{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
		Impact:     impact,
		Details:    make(map[string]string),
	}
}

// WrapError wraps an existing Go error inside a CompatError
func WrapError(code ErrorCode, message string, err error) *CompatError {
	return &CompatError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: make(map[string]string),
	}
}
