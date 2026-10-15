package stubs

import (
	"context"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// WSLEngine provides the adapter contract for Windows WSL2
type WSLEngine struct {
	runner runner.Runner
}

func NewWSL(r runner.Runner) *WSLEngine { return &WSLEngine{runner: r} }
func (w *WSLEngine) ID() string         { return "wsl2" }
func (w *WSLEngine) Name() string       { return "Windows Subsystem for Linux (WSL2)" }
func (w *WSLEngine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:         w.ID(),
		EngineName:       w.Name(),
		SupportedFormats: []domain.PackageFormat{domain.FormatDeb, domain.FormatRPM},
	}
	if host.OS != "windows" {
		rep.Available = false
		rep.MissingPrerequisites = []string{"WSL2 engine is only available on Windows hosts"}
		return rep, nil
	}
	rep.Available = true
	return rep, nil
}
func (w *WSLEngine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "WSL2 engine is architecture-ready but requires Windows host", "Run compat on a Windows host", "Planning refused")
}
func (w *WSLEngine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "WSL2 cannot be applied on non-Windows host", "Switch to Windows host", "Apply refused")
}
func (w *WSLEngine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "WSL2 launch requires Windows host", "", "")
}
func (w *WSLEngine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	return nil
}
func (w *WSLEngine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	return domain.StateInstalled, nil
}
func (w *WSLEngine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return nil, nil
}

// LimaEngine provides the adapter contract for macOS Lima VM
type LimaEngine struct {
	runner runner.Runner
}

func NewLima(r runner.Runner) *LimaEngine { return &LimaEngine{runner: r} }
func (l *LimaEngine) ID() string          { return "lima" }
func (l *LimaEngine) Name() string        { return "macOS Linux VM (Lima)" }
func (l *LimaEngine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:         l.ID(),
		EngineName:       l.Name(),
		SupportedFormats: []domain.PackageFormat{domain.FormatDeb, domain.FormatRPM},
	}
	if host.OS != "darwin" {
		rep.Available = false
		rep.MissingPrerequisites = []string{"Lima engine is only available on macOS hosts"}
		return rep, nil
	}
	rep.Available = true
	return rep, nil
}
func (l *LimaEngine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "Lima engine requires macOS host", "Run compat on a macOS machine", "Planning refused")
}
func (l *LimaEngine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "Lima cannot be applied on non-macOS host", "", "")
}
func (l *LimaEngine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "Lima launch requires macOS host", "", "")
}
func (l *LimaEngine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	return nil
}
func (l *LimaEngine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	return domain.StateInstalled, nil
}
func (l *LimaEngine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return nil, nil
}
