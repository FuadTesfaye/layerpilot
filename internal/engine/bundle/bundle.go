package bundle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// Engine implements Desktop Bundle installation for Flatpak and Snap (S4)
type Engine struct {
	runner runner.Runner
}

// New creates a new Bundle engine
func New(r runner.Runner) *Engine {
	return &Engine{runner: r}
}

func (e *Engine) ID() string {
	return "bundle"
}

func (e *Engine) Name() string {
	return "Desktop Bundle Engine (Flatpak/Snap)"
}

func (e *Engine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:               e.ID(),
		EngineName:             e.Name(),
		SupportedFormats:       []domain.PackageFormat{domain.FormatFlatpak, domain.FormatSnap},
		SupportedArchitectures: []string{"amd64", "arm64"},
		SupportsGUI:            true,
		Rootless:               true,
		DiagnosticDetails:      make(map[string]string),
	}

	hasFlatpak := false
	if _, err := exec.LookPath("flatpak"); err == nil {
		hasFlatpak = true
		rep.DiagnosticDetails["flatpak"] = "flatpak CLI available (user scope supported without root)"
	}

	hasSnap := false
	if _, err := exec.LookPath("snap"); err == nil {
		hasSnap = true
		rep.DiagnosticDetails["snap"] = "snap CLI available"
	}

	rep.Available = hasFlatpak || hasSnap
	if !rep.Available {
		rep.MissingPrerequisites = append(rep.MissingPrerequisites, "Neither flatpak nor snap found in PATH")
	}

	return rep, nil
}

func (e *Engine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	if plan.Package.Format == domain.FormatFlatpak {
		plan.RequiresApproval = false
		plan.Steps = []domain.PlanStep{
			{
				Order:       1,
				Name:        "FlatpakUserInstall",
				Description: fmt.Sprintf("Install %s into user flatpak repository without root", plan.Package.Name),
				HostImpact:  "Registers application in ~/.local/share/flatpak",
			},
		}
		plan.HostChanges = []string{
			fmt.Sprintf("User Flatpak package: %s", plan.Package.Name),
		}
		return nil
	}

	// Snap requires single root elevation
	plan.RequiresApproval = true
	cmdStr := fmt.Sprintf("pkexec snap install --dangerous %s", plan.Package.Path)
	plan.PrivilegedOperations = []string{cmdStr}
	plan.Steps = []domain.PlanStep{
		{
			Order:       1,
			Name:        "SnapInstall",
			Description: fmt.Sprintf("Execute privileged command: %s", cmdStr),
			HostImpact:  "Installs snap package in /var/lib/snapd",
		},
	}
	plan.HostChanges = []string{
		fmt.Sprintf("System Snap package: %s", plan.Package.Name),
		fmt.Sprintf("Privileged execution: %s", cmdStr),
	}

	return nil
}

func (e *Engine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	var cmdArgs []string
	if plan.Package.Format == domain.FormatFlatpak {
		if strings.HasSuffix(strings.ToLower(plan.Package.Path), ".flatpakref") {
			cmdArgs = []string{"flatpak", "install", "--user", "--noninteractive", plan.Package.Path}
		} else {
			cmdArgs = []string{"flatpak", "install", "--user", "--bundle", "--noninteractive", plan.Package.Path}
		}
	} else {
		cmdArgs = []string{"pkexec", "snap", "install", "--dangerous", plan.Package.Path}
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	var cmdArgs []string
	if app.PackageFormat == domain.FormatFlatpak {
		cmdArgs = append([]string{"flatpak", "run", app.DisplayName}, args...)
	} else {
		cmdArgs = append([]string{"snap", "run", app.DisplayName}, args...)
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	var cmdArgs []string
	if app.PackageFormat == domain.FormatFlatpak {
		cmdArgs = []string{"flatpak", "uninstall", "--user", "-y", app.DisplayName}
	} else {
		cmdArgs = []string{"pkexec", "snap", "remove", app.DisplayName}
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	if app.PackageFormat == domain.FormatFlatpak {
		cmd := exec.CommandContext(ctx, "flatpak", "info", app.DisplayName)
		if err := cmd.Run(); err == nil {
			return domain.StateInstalled, nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "snap", "list", app.DisplayName)
		if err := cmd.Run(); err == nil {
			return domain.StateInstalled, nil
		}
	}
	return domain.StateRemoved, nil
}

func (e *Engine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return []string{"Desktop bundle applications log to host system journal"}, nil
}
