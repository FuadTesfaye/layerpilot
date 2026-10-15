package native

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// Engine implements single-command native host package installation (S3)
type Engine struct {
	runner runner.Runner
}

// New creates a new Native package engine
func New(r runner.Runner) *Engine {
	return &Engine{runner: r}
}

func (e *Engine) ID() string {
	return "native"
}

func (e *Engine) Name() string {
	return "Native Host Package Engine"
}

func (e *Engine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:               e.ID(),
		EngineName:             e.Name(),
		SupportedFormats:       []domain.PackageFormat{domain.FormatDeb, domain.FormatRPM, domain.FormatArchPkg},
		SupportedArchitectures: []string{"amd64", "arm64", "386"},
		SupportsGUI:            true,
		Rootless:               false,
		DiagnosticDetails:      make(map[string]string),
	}

	if host.OS != "linux" {
		rep.Available = false
		rep.MissingPrerequisites = append(rep.MissingPrerequisites, "Native package management requires Linux host")
		return rep, nil
	}

	hasAuth := false
	if _, err := exec.LookPath("pkexec"); err == nil {
		hasAuth = true
		rep.DiagnosticDetails["auth"] = "pkexec available for single-command privilege elevation"
	} else if _, err := exec.LookPath("sudo"); err == nil {
		hasAuth = true
		rep.DiagnosticDetails["auth"] = "sudo available for single-command privilege elevation"
	}

	rep.Available = hasAuth && len(host.PackageManagers) > 0
	if !rep.Available {
		rep.MissingPrerequisites = append(rep.MissingPrerequisites, "Requires pkexec or sudo and a recognized host package manager")
	}

	return rep, nil
}

func (e *Engine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	cmdStr := strings.Join(plan.Decision.SingleRootCommand, " ")
	plan.RequiresApproval = true
	plan.PrivilegedOperations = []string{cmdStr}

	plan.Steps = []domain.PlanStep{
		{
			Order:       1,
			Name:        "VerifyPackageChecksum",
			Description: fmt.Sprintf("Verify SHA256 (%s) and package integrity before execution", plan.Package.SHA256[:12]),
			HostImpact:  "Read-only cryptographic check",
		},
		{
			Order:       2,
			Name:        "NativePackageInstall",
			Description: fmt.Sprintf("Execute single privileged command: %s", cmdStr),
			HostImpact:  "Installs package files into system directories using host package manager",
		},
		{
			Order:       3,
			Name:        "RecordOwnership",
			Description: fmt.Sprintf("Register package ownership for %s in SQLite state store", plan.Package.Name),
			HostImpact:  "Enables clean uninstallation via 'compat uninstall'",
		},
	}

	plan.HostChanges = []string{
		fmt.Sprintf("Installed system package: %s", plan.Package.Name),
		fmt.Sprintf("Privileged execution: %s", cmdStr),
	}

	return nil
}

func (e *Engine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	cmdArgs := plan.Decision.SingleRootCommand
	if len(cmdArgs) == 0 {
		return domain.NewError(domain.ErrCodePackageInvalid, "no privileged command defined for native package install", "", "")
	}

	// Strictly exec.CommandContext with argument arrays - zero shell interpolation
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, fmt.Sprintf("native package install failed: %v", err), err)
	}

	return nil
}

func (e *Engine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	binName := app.BinaryEntrypoint
	if binName == "" {
		binName = app.DisplayName
	}

	binPath, err := exec.LookPath(binName)
	if err != nil {
		// Check common install dirs
		for _, dir := range []string{"/usr/bin", "/usr/local/bin", "/opt/" + app.DisplayName} {
			p := filepath.Join(dir, binName)
			if fi, err := os.Stat(p); err == nil && fi.Mode()&0111 != 0 {
				binPath = p
				break
			}
		}
	}

	if binPath == "" {
		return domain.NewError(domain.ErrCodeAppNotFound, "cannot locate installed binary in PATH: "+binName, "Ensure package installed correctly", "Launch failed")
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	var removeCmd []string
	switch app.HostDistro {
	case "fedora", "rhel", "centos":
		removeCmd = []string{"pkexec", "dnf", "remove", "-y", app.DisplayName}
	case "debian", "ubuntu", "mint":
		removeCmd = []string{"pkexec", "apt-get", "remove", "-y", app.DisplayName}
	case "arch", "manjaro":
		removeCmd = []string{"pkexec", "pacman", "-R", "--noconfirm", app.DisplayName}
	case "suse", "opensuse":
		removeCmd = []string{"pkexec", "zypper", "--non-interactive", "remove", app.DisplayName}
	default:
		return domain.NewError(domain.ErrCodeFormatUnsupported, "unsupported distro for native package removal: "+app.HostDistro, "", "")
	}

	cmd := exec.CommandContext(ctx, removeCmd[0], removeCmd[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	binName := app.BinaryEntrypoint
	if binName == "" {
		binName = app.DisplayName
	}
	if _, err := exec.LookPath(binName); err == nil {
		return domain.StateInstalled, nil
	}
	return domain.StateRemoved, nil
}

func (e *Engine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return []string{"Native application runs on host system directly"}, nil
}
