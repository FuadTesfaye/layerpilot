package podman

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

// Engine implements domain.Engine for rootless Linux containers
type Engine struct {
	runner runner.Runner
}

// New creates a new Podman engine instance
func New(r runner.Runner) *Engine {
	return &Engine{runner: r}
}

func (e *Engine) ID() string {
	return "podman"
}

func (e *Engine) Name() string {
	return "Linux Container Engine (Podman)"
}

func (e *Engine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:               e.ID(),
		EngineName:             e.Name(),
		SupportedFormats:       []domain.PackageFormat{domain.FormatDeb, domain.FormatRPM},
		SupportedArchitectures: []string{"amd64", "arm64", "386"},
		SupportsGUI:            true,
		DiagnosticDetails:      make(map[string]string),
	}

	// 1. Check podman binary
	res, err := e.runner.Run(ctx, "podman", "--version")
	if err != nil {
		rep.Available = false
		rep.MissingPrerequisites = append(rep.MissingPrerequisites, "podman executable not found in PATH")
		return rep, nil
	}

	rep.Version = strings.TrimSpace(res.Stdout)
	rep.DiagnosticDetails["version"] = rep.Version

	// 2. Check rootless security status
	infoRes, err := e.runner.Run(ctx, "podman", "info", "--format", "{{.Host.Security.Rootless}}")
	if err == nil && strings.TrimSpace(infoRes.Stdout) == "true" {
		rep.Rootless = true
	} else {
		rep.Rootless = false
		rep.DiagnosticDetails["rootless_warning"] = "Podman is not running in rootless mode"
	}

	rep.Available = true
	return rep, nil
}

func (e *Engine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	containerName := fmt.Sprintf("compat-app-%s", plan.AppID)
	baseImage := plan.BaseImage
	if baseImage == "" {
		if plan.Package.Format == domain.FormatDeb {
			baseImage = "docker.io/library/debian:bookworm-slim"
		} else {
			baseImage = "docker.io/library/fedora:40"
		}
	}
	plan.BaseImage = baseImage

	plan.Steps = []domain.PlanStep{
		{
			Order:       1,
			Name:        "CheckBaseImage",
			Description: fmt.Sprintf("Verify base image '%s' in local storage or pull", baseImage),
			HostImpact:  "Downloads container image layers if not cached",
		},
		{
			Order:       2,
			Name:        "CreateContainer",
			Description: fmt.Sprintf("Create rootless container '%s' with display and audio sockets", containerName),
			HostImpact:  fmt.Sprintf("Creates local container '%s'", containerName),
		},
		{
			Order:       3,
			Name:        "InstallPackage",
			Description: fmt.Sprintf("Install '%s' inside the managed container", plan.Package.Name),
			HostImpact:  "Installs software and dependencies within container storage",
		},
		{
			Order:       4,
			Name:        "VerifyAndExport",
			Description: "Verify executable presence and generate host launcher wrapper",
			HostImpact:  fmt.Sprintf("Creates wrapper in ~/.local/share/compat/bin/%s", plan.AppID),
		},
	}

	plan.HostChanges = []string{
		fmt.Sprintf("Container: %s", containerName),
		fmt.Sprintf("Launcher: ~/.local/share/compat/bin/%s", plan.AppID),
	}

	return nil
}

func (e *Engine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	containerName := fmt.Sprintf("compat-app-%s", plan.AppID)

	// 1. Ensure base image exists or pull
	imgCheck, _ := e.runner.Run(ctx, "podman", "image", "exists", plan.BaseImage)
	if imgCheck.ExitCode != 0 {
		if _, err := e.runner.Run(ctx, "podman", "pull", plan.BaseImage); err != nil {
			return domain.WrapError(domain.ErrCodeRunnerFailed, "failed to pull container base image "+plan.BaseImage, err)
		}
	}

	// 2. Prepare container create arguments
	appDataDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "compat", "apps", plan.AppID, "data")
	_ = os.MkdirAll(appDataDir, 0755)

	createArgs := []string{
		"create",
		"--name", containerName,
		"-v", fmt.Sprintf("%s:/data:rw", appDataDir),
	}

	// Display socket passthrough
	if waylandDisplay := os.Getenv("WAYLAND_DISPLAY"); waylandDisplay != "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		createArgs = append(createArgs,
			"-e", fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			"-v", fmt.Sprintf("%s/%s:/tmp/%s:ro", runtimeDir, waylandDisplay, waylandDisplay),
		)
	}
	if display := os.Getenv("DISPLAY"); display != "" {
		createArgs = append(createArgs,
			"-e", fmt.Sprintf("DISPLAY=%s", display),
			"-v", "/tmp/.X11-unix:/tmp/.X11-unix:ro",
		)
	}

	createArgs = append(createArgs, plan.BaseImage, "sleep", "infinity")

	// Check if container already exists, remove if stale
	_, _ = e.runner.Run(ctx, "podman", "rm", "-f", containerName)

	if _, err := e.runner.Run(ctx, "podman", createArgs...); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed to create container", err)
	}

	// 3. Start container
	if _, err := e.runner.Run(ctx, "podman", "start", containerName); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed to start container", err)
	}

	// 4. Copy package into container
	destInContainer := fmt.Sprintf("%s:/tmp/package.%s", containerName, plan.Package.Format)
	if _, err := e.runner.Run(ctx, "podman", "cp", plan.Package.Path, destInContainer); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed to copy package into container", err)
	}

	// 5. Install package inside container
	pkgFileInContainer := fmt.Sprintf("/tmp/package.%s", plan.Package.Format)
	if plan.Package.Format == domain.FormatDeb {
		// Update and install
		_, _ = e.runner.Run(ctx, "podman", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "update")
		if _, err := e.runner.Run(ctx, "podman", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "install", "-y", pkgFileInContainer); err != nil {
			// Try dpkg + apt-get -f install
			_, _ = e.runner.Run(ctx, "podman", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "dpkg", "-i", pkgFileInContainer)
			if _, errFix := e.runner.Run(ctx, "podman", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "install", "-f", "-y"); errFix != nil {
				return domain.WrapError(domain.ErrCodeRunnerFailed, "package installation failed inside container", err)
			}
		}
	} else if plan.Package.Format == domain.FormatRPM {
		if _, err := e.runner.Run(ctx, "podman", "exec", containerName, "dnf", "install", "-y", pkgFileInContainer); err != nil {
			return domain.WrapError(domain.ErrCodeRunnerFailed, "rpm installation failed inside container", err)
		}
	}

	// 6. Create host launcher script
	binDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "compat", "bin")
	_ = os.MkdirAll(binDir, 0755)
	launcherScript := filepath.Join(binDir, plan.AppID)

	scriptContent := fmt.Sprintf(`#!/bin/sh
# Generated by compat application manager
podman start %s >/dev/null 2>&1
exec podman exec -it %s %s "$@"
`, containerName, containerName, plan.Package.Name)

	if err := os.WriteFile(launcherScript, []byte(scriptContent), 0755); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed to write launcher wrapper", err)
	}

	return nil
}

func (e *Engine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	containerName := fmt.Sprintf("compat-app-%s", app.AppID)
	// Ensure container is running
	_, _ = e.runner.Run(ctx, "podman", "start", containerName)

	execCmd := app.BinaryEntrypoint
	if execCmd == "" {
		execCmd = app.DisplayName
	}

	execArgs := []string{"exec"}
	execArgs = append(execArgs, containerName, execCmd)
	execArgs = append(execArgs, args...)

	cmd := exec.CommandContext(ctx, "podman", execArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	containerName := fmt.Sprintf("compat-app-%s", app.AppID)
	_, _ = e.runner.Run(ctx, "podman", "stop", "-t", "2", containerName)
	_, _ = e.runner.Run(ctx, "podman", "rm", "-f", containerName)

	// Remove launcher wrapper
	launcherScript := filepath.Join(os.Getenv("HOME"), ".local", "share", "compat", "bin", app.AppID)
	_ = os.Remove(launcherScript)

	// Remove app directory
	appDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "compat", "apps", app.AppID)
	_ = os.RemoveAll(appDir)

	return nil
}

func (e *Engine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	containerName := fmt.Sprintf("compat-app-%s", app.AppID)
	res, err := e.runner.Run(ctx, "podman", "container", "inspect", containerName, "--format", "{{.State.Status}}")
	if err != nil {
		return domain.StateStopped, nil
	}

	status := strings.TrimSpace(res.Stdout)
	switch status {
	case "running":
		return domain.StateRunning, nil
	case "exited", "stopped", "configured":
		return domain.StateStopped, nil
	default:
		return domain.StateInstalled, nil
	}
}

func (e *Engine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	containerName := fmt.Sprintf("compat-app-%s", app.AppID)
	if lines <= 0 {
		lines = 50
	}
	res, err := e.runner.Run(ctx, "podman", "logs", "--tail", fmt.Sprintf("%d", lines), containerName)
	if err != nil {
		return nil, err
	}

	output := strings.Split(res.Stdout, "\n")
	if res.Stderr != "" {
		output = append(output, strings.Split(res.Stderr, "\n")...)
	}
	return output, nil
}
