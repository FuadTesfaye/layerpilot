package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// DefaultMaxOutputBytes is the maximum captured stdout/stderr size (10 MB)
const DefaultMaxOutputBytes = 10 * 1024 * 1024

// RunOptions configures controlled subprocess execution
type RunOptions struct {
	Command        string
	Args           []string
	Env            []string
	Dir            string
	Stdin          io.Reader
	Timeout        time.Duration
	MaxOutputBytes int64
}

// RunResult contains the captured results of a process run
type RunResult struct {
	ExitCode int           `json:"exit_code"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	Duration time.Duration `json:"duration"`
}

// Runner defines the execution boundary for all host commands and engine operations
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (*RunResult, error)
	RunWithOptions(ctx context.Context, opts RunOptions) (*RunResult, error)
}

// ExecRunner implements Runner using os/exec with safe parameter passing
type ExecRunner struct{}

// New creates a new ExecRunner
func New() *ExecRunner {
	return &ExecRunner{}
}

// Run executes a command with default options
func (r *ExecRunner) Run(ctx context.Context, command string, args ...string) (*RunResult, error) {
	return r.RunWithOptions(ctx, RunOptions{
		Command: command,
		Args:    args,
	})
}

// RunWithOptions executes a command with strict controls: no shell interpolation, timeouts, and buffer caps
func (r *ExecRunner) RunWithOptions(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Command == "" {
		return nil, domain.NewError(domain.ErrCodeRunnerFailed, "empty command", "Specify an executable name", "Subprocess execution failed")
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, opts.Command, opts.Args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	maxBytes := opts.MaxOutputBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}

	stdoutBuf := &cappedBuffer{max: maxBytes}
	stderrBuf := &cappedBuffer{max: maxBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &RunResult{
		ExitCode: 0,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
	}

	if err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return result, domain.NewError(
				domain.ErrCodeRunnerTimeout,
				fmt.Sprintf("command '%s' timed out after %s", opts.Command, opts.Timeout),
				"Increase operation timeout or check if process is waiting on input",
				"Subprocess was terminated",
			)
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			compatErr := domain.NewError(
				domain.ErrCodeRunnerFailed,
				fmt.Sprintf("command '%s' failed with exit code %d", opts.Command, result.ExitCode),
				"Inspect process stderr for failure details",
				"Operation failed",
			)
			compatErr.Details["stderr"] = result.Stderr
			compatErr.Details["stdout"] = result.Stdout
			compatErr.Err = err
			return result, compatErr
		}

		return result, domain.WrapError(domain.ErrCodeRunnerFailed, fmt.Sprintf("failed to start command '%s'", opts.Command), err)
	}

	return result, nil
}

type cappedBuffer struct {
	buf bytes.Buffer
	max int64
}

func (c *cappedBuffer) Write(p []byte) (n int, err error) {
	remaining := c.max - int64(c.buf.Len())
	if remaining <= 0 {
		return len(p), nil // drop extra bytes silently to prevent OOM
	}
	if int64(len(p)) > remaining {
		_, _ = c.buf.Write(p[:remaining])
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *cappedBuffer) String() string {
	return c.buf.String()
}
