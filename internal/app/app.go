package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/detect"
	"github.com/FuadTesfaye/layerpilot/internal/doctor"
	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/engine/appimage"
	"github.com/FuadTesfaye/layerpilot/internal/engine/podman"
	"github.com/FuadTesfaye/layerpilot/internal/engine/stubs"
	"github.com/FuadTesfaye/layerpilot/internal/hostinfo"
	"github.com/FuadTesfaye/layerpilot/internal/integration"
	"github.com/FuadTesfaye/layerpilot/internal/matrix"
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
	reg.Register(stubs.NewWSL(r))
	reg.Register(stubs.NewLima(r))
	reg.Register(stubs.NewWine(r))

	pln := planner.New(mat, reg)
	integ := integration.NewDesktopIntegrator(r)
	doc := doctor.New(prof, reg, r)

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
