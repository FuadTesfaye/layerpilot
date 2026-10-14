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

// WineEngine provides the experimental adapter contract for Windows applications
type WineEngine struct {
	runner runner.Runner
}

func NewWine(r runner.Runner) *WineEngine { return &WineEngine{runner: r} }
func (w *WineEngine) ID() string          { return "wine" }
func (w *WineEngine) Name() string        { return "Wine Translation Engine (Experimental)" }
func (w *WineEngine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:         w.ID(),
		EngineName:       w.Name(),
		SupportedFormats: []domain.PackageFormat{domain.FormatWindowsExe},
	}
	res, err := w.runner.Run(ctx, "wine", "--version")
	if err != nil {
		rep.Available = false
		rep.MissingPrerequisites = []string{"wine executable not found in PATH"}
		return rep, nil
	}
	rep.Available = true
	rep.Version = res.Stdout
	return rep, nil
}
func (w *WineEngine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	plan.Decision.SupportTier = domain.Tier3Experimental
	plan.Decision.Limitations = append(plan.Decision.Limitations, "Wine prefix isolation is experimental; DirectX 12/anti-cheat may fail")
	return nil
}
func (w *WineEngine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "Wine execution is experimental and requires explicit tested application profile", "Refer to tested applications catalogue", "Refused untested binary")
}
func (w *WineEngine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	return domain.NewError(domain.ErrCodeExecutionRefused, "Wine launch is experimental", "", "")
}
func (w *WineEngine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	return nil
}
func (w *WineEngine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	return domain.StateInstalled, nil
}
func (w *WineEngine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return nil, nil
}
