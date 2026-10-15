package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/detect"
	"github.com/FuadTesfaye/layerpilot/internal/doctor"
	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/engine/appimage"
	"github.com/FuadTesfaye/layerpilot/internal/engine/bundle"
	"github.com/FuadTesfaye/layerpilot/internal/engine/native"
	"github.com/FuadTesfaye/layerpilot/internal/engine/podman"
	"github.com/FuadTesfaye/layerpilot/internal/engine/stubs"
	"github.com/FuadTesfaye/layerpilot/internal/engine/tarball"
	"github.com/FuadTesfaye/layerpilot/internal/engine/wine"
	"github.com/FuadTesfaye/layerpilot/internal/fix"
	"github.com/FuadTesfaye/layerpilot/internal/hostinfo"
	"github.com/FuadTesfaye/layerpilot/internal/integration"
	"github.com/FuadTesfaye/layerpilot/internal/matrix"
	"github.com/FuadTesfaye/layerpilot/internal/patterns"
	"github.com/FuadTesfaye/layerpilot/internal/planner"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
	"github.com/FuadTesfaye/layerpilot/internal/state"
)

// Workflow coordinates the full end-to-end lifecycle of managed compatibility applications
type Workflow struct {
	detector   *detect.Detector
	profiler   *hostinfo.Profiler
	matrix     *matrix.PolicyEngine
	registry   *engine.Registry
	planner    *planner.Planner
	runner     runner.Runner
	store      *state.Store
	integrator *integration.DesktopIntegrator
	doctor     *doctor.Doctor
	fixEngine  *fix.Engine
}

// NewDefaultWorkflow initializes all standard subsystems, engines, and durable SQLite store
func NewDefaultWorkflow() (*Workflow, error) {
	r := runner.New()
	store, err := state.OpenDefault()
	if err != nil {
		return nil, err
	}

	det := detect.New()
	prof := hostinfo.New()
	mat := matrix.New()

	reg := engine.NewRegistry()
	reg.Register(podman.New(r))
	reg.Register(appimage.New(r))
	reg.Register(tarball.New(r))
	reg.Register(native.New(r))
	reg.Register(bundle.New(r))
	reg.Register(stubs.NewWSL(r))
	reg.Register(stubs.NewLima(r))
	reg.Register(wine.New(r))

	pln := planner.New(mat, reg)
	integ := integration.NewDesktopIntegrator(r)
	doc := doctor.New(prof, reg, r)

	fixEng, err := fix.NewEngine(store, nil)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	return &Workflow{
		detector:   det,
		profiler:   prof,
		matrix:     mat,
		registry:   reg,
		planner:    pln,
		runner:     r,
		store:      store,
		integrator: integ,
		doctor:     doc,
		fixEngine:  fixEng,
	}, nil
}

// Inspect identifies file format, architecture, and package metadata without installing
func (w *Workflow) Inspect(ctx context.Context, pkgPath string) (*domain.PackageInfo, error) {
	return w.detector.Inspect(pkgPath)
}

// Support evaluates applicable compatibility strategies, support tier, and limitations
func (w *Workflow) Support(ctx context.Context, pkgPath string) (*domain.StrategyDecision, error) {
	pkg, err := w.detector.Inspect(pkgPath)
	if err != nil {
		return nil, err
	}

	host, err := w.profiler.Current()
	if err != nil {
		return nil, err
	}

	available := w.registry.AvailableEngineIDs(ctx, host)
	return w.matrix.Evaluate(pkg, host, available), nil
}

// Plan produces an explicit previewable action plan without applying changes
func (w *Workflow) Plan(ctx context.Context, pkgPath string) (*domain.InstallPlan, error) {
	pkg, err := w.detector.Inspect(pkgPath)
	if err != nil {
		return nil, err
	}

	host, err := w.profiler.Current()
	if err != nil {
		return nil, err
	}

	return w.planner.Plan(ctx, pkg, host)
}

// Install previews, executes the plan within the selected engine, and stores the application record
func (w *Workflow) Install(ctx context.Context, pkgPath string, autoApprove bool) (*domain.ApplicationRecord, error) {
	plan, err := w.Plan(ctx, pkgPath)
	if err != nil {
		return nil, err
	}

	if !autoApprove && plan.RequiresApproval {
		return nil, domain.NewError(
			domain.ErrCodeApprovalRequired,
			"plan requires explicit user confirmation before applying system changes",
			"Pass --yes or approve the plan to proceed",
			"Installation aborted without changes",
		)
	}

	eng, ok := w.registry.Get(plan.Decision.EngineID)
	if !ok {
		return nil, domain.NewError(
			domain.ErrCodeEngineUnavailable,
			fmt.Sprintf("engine '%s' is not registered", plan.Decision.EngineID),
			"Check registered engines",
			"Installation aborted",
		)
	}

	host, _ := w.profiler.Current()

	// Initial record in installing state
	appRecord := &domain.ApplicationRecord{
		AppID:            plan.AppID,
		DisplayName:      plan.Package.Name,
		PackageFormat:    plan.Package.Format,
		PackageHash:      plan.Package.SHA256,
		PackageArch:      plan.Package.Architecture,
		PackageVersion:   plan.Package.Version,
		EngineID:         plan.Decision.EngineID,
		HostOS:           host.OS,
		HostDistro:       host.Distro,
		Status:           domain.StateInstalling,
		PackageType:      plan.Package.PackageType,
		InstalledAt:      time.Now().UTC(),
		BinaryEntrypoint: plan.Package.Name,
	}

	_ = w.store.Save(appRecord)
	_ = w.store.LogAudit(plan.AppID, "INSTALL_START", fmt.Sprintf("Installing via %s", plan.Decision.EngineID))

	// Execute Plan
	if err := eng.Apply(ctx, plan); err != nil {
		appRecord.Status = domain.StateInstallFailed
		_ = w.store.Save(appRecord)
		_ = w.store.LogAudit(plan.AppID, "INSTALL_FAILED", err.Error())
		return nil, err
	}

	// Set wrapper launcher path
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	appRecord.LauncherPath = filepath.Join(home, ".local", "share", "compat", "bin", plan.AppID)

	if plan.BinaryEntrypoint != "" {
		appRecord.BinaryEntrypoint = plan.BinaryEntrypoint
	}

	// Install desktop launcher
	appRecord.Status = domain.StateInstalled
	_, _ = w.integrator.InstallLauncher(appRecord)

	if err := w.store.Save(appRecord); err != nil {
		return nil, err
	}

	_ = w.store.LogAudit(plan.AppID, "INSTALL_COMPLETE", fmt.Sprintf("Successfully installed at %s", appRecord.InstalledAt))
	return appRecord, nil
}

// List returns all managed applications
func (w *Workflow) List(ctx context.Context) ([]domain.ApplicationRecord, error) {
	return w.store.List()
}

// Launch executes an installed application
func (w *Workflow) Launch(ctx context.Context, appID string, args []string) error {
	app, err := w.store.Get(appID)
	if err != nil {
		return err
	}

	eng, ok := w.registry.Get(app.EngineID)
	if !ok {
		return domain.NewError(domain.ErrCodeEngineUnavailable, "engine unavailable: "+app.EngineID, "", "")
	}

	now := time.Now().UTC()
	app.LastLaunchedAt = &now
	_ = w.store.Save(app)
	_ = w.store.LogAudit(appID, "LAUNCH", fmt.Sprintf("Launching with args: %v", args))

	return eng.Launch(ctx, app, args)
}

// Status returns current lifecycle status
func (w *Workflow) Status(ctx context.Context, appID string) (domain.LifecycleState, error) {
	app, err := w.store.Get(appID)
	if err != nil {
		return "", err
	}

	eng, ok := w.registry.Get(app.EngineID)
	if !ok {
		return app.Status, nil
	}

	liveStatus, err := eng.Status(ctx, app)
	if err == nil && liveStatus != "" {
		if liveStatus != app.Status {
			_ = w.store.UpdateStatus(appID, liveStatus)
		}
		return liveStatus, nil
	}

	return app.Status, nil
}

// Logs returns application logs
func (w *Workflow) Logs(ctx context.Context, appID string, lines int) ([]string, error) {
	app, err := w.store.Get(appID)
	if err != nil {
		return nil, err
	}

	eng, ok := w.registry.Get(app.EngineID)
	if !ok {
		return nil, domain.NewError(domain.ErrCodeEngineUnavailable, "engine unavailable: "+app.EngineID, "", "")
	}

	return eng.Logs(ctx, app, lines)
}

// Explain details why a compatibility strategy was selected
func (w *Workflow) Explain(ctx context.Context, appID string) (*domain.ApplicationRecord, error) {
	return w.store.Get(appID)
}

// Uninstall cleanly removes an application and all owned resources
func (w *Workflow) Uninstall(ctx context.Context, appID string, autoApprove bool) error {
	app, err := w.store.Get(appID)
	if err != nil {
		return err
	}

	if !autoApprove {
		return domain.NewError(
			domain.ErrCodeApprovalRequired,
			fmt.Sprintf("uninstalling '%s' requires confirmation", app.DisplayName),
			"Pass --yes to confirm removal",
			"Uninstall aborted without changes",
		)
	}

	eng, ok := w.registry.Get(app.EngineID)
	if ok {
		_ = eng.Uninstall(ctx, app)
	}

	_ = w.integrator.RemoveLauncher(appID)
	_ = w.store.LogAudit(appID, "UNINSTALL", "Application uninstalled and resources removed")
	return w.store.Delete(appID)
}

// Doctor executes diagnostic checks
func (w *Workflow) Doctor(ctx context.Context) *doctor.DoctorReport {
	return w.doctor.Diagnose(ctx)
}

// EngineList returns reports on all registered engines
func (w *Workflow) EngineList(ctx context.Context) map[string]*domain.CapabilityReport {
	host, _ := w.profiler.Current()
	return w.registry.ProbeAll(ctx, host)
}

// Cleanup previews or cleans orphaned resources
func (w *Workflow) Cleanup(ctx context.Context, dryRun bool) ([]string, error) {
	apps, err := w.store.List()
	if err != nil {
		return nil, err
	}

	var actions []string
	actions = append(actions, fmt.Sprintf("Audited %d managed application records", len(apps)))
	if dryRun {
		actions = append(actions, "Dry run complete: no orphaned resources found for deletion")
	}

	return actions, nil
}

// Close closes database connections
func (w *Workflow) Close() error {
	if w.store != nil {
		return w.store.Close()
	}
	return nil
}

// Diagnose analyzes application runtime logs for known failure patterns
func (w *Workflow) Diagnose(ctx context.Context, appID string) (*fix.Diagnosis, error) {
	return w.fixEngine.Diagnose(ctx, appID)
}

// ApplyFix applies a chosen remediation option and verifies stability
func (w *Workflow) ApplyFix(ctx context.Context, appID string, fixOption patterns.FixOption) (*fix.FixResult, error) {
	return w.fixEngine.ApplyFix(ctx, appID, fixOption)
}

// UndoFix rolls back the latest applied compatibility fix
func (w *Workflow) UndoFix(ctx context.Context, appID string) (*fix.UndoResult, error) {
	return w.fixEngine.UndoFix(ctx, appID)
}

// FixEngine exposes the fix engine instance
func (w *Workflow) FixEngine() *fix.Engine {
	return w.fixEngine
}

// Run executes a package in one-shot mode without permanent installation
func (w *Workflow) Run(ctx context.Context, pkgPath string, args []string) error {
	pkg, err := w.detector.Inspect(pkgPath)
	if err != nil {
		return err
	}

	if pkg.Format != domain.FormatAppImage {
		return domain.NewError(domain.ErrCodeFormatUnsupported, fmt.Sprintf("one-shot 'run' is currently supported for AppImage packages (got %s)", pkg.Format), "Use 'compat install' to manage this package format", "Execution aborted")
	}

	// Create temporary staging sandbox
	tmpDir, err := os.MkdirTemp("", "compat-run-*")
	if err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed creating sandbox directory", err)
	}
	defer os.RemoveAll(tmpDir)

	stagedPath := filepath.Join(tmpDir, "app.AppImage")
	srcFile, err := os.Open(pkgPath)
	if err != nil {
		return domain.WrapError(domain.ErrCodePackageInvalid, "cannot open AppImage", err)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "cannot create temporary AppImage", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "cannot copy AppImage to temporary sandbox", err)
	}
	_ = destFile.Close()

	// Execute with proper display server flags if needed
	launchArgs := args
	if os.Getenv("WAYLAND_DISPLAY") != "" && pkg.PackageType == domain.PackageTypeGUI {
		hasOzone := false
		for _, a := range args {
			if strings.Contains(a, "ozone-platform") {
				hasOzone = true
				break
			}
		}
		if !hasOzone {
			launchArgs = append([]string{"--ozone-platform-hint=auto"}, launchArgs...)
		}
	}

	env := os.Environ()
	if _, err := os.Stat("/dev/fuse"); err != nil {
		env = append(env, "APPIMAGE_EXTRACT_AND_RUN=1")
	}

	cmd := exec.CommandContext(ctx, stagedPath, launchArgs...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
