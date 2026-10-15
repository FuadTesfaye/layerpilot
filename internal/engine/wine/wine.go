package wine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// Engine implements isolated Wine prefix management for Windows applications
type Engine struct {
	runner runner.Runner
}

// New creates a new Wine engine
func New(r runner.Runner) *Engine {
	return &Engine{runner: r}
}

func (e *Engine) ID() string {
	return "wine"
}

func (e *Engine) Name() string {
	return "Wine Prefix Engine (Experimental)"
}

func (e *Engine) Probe(ctx context.Context, host *domain.HostProfile) (*domain.CapabilityReport, error) {
	rep := &domain.CapabilityReport{
		EngineID:               e.ID(),
		EngineName:             e.Name(),
		SupportedFormats:       []domain.PackageFormat{domain.FormatWindowsExe},
		SupportedArchitectures: []string{"amd64", "386", "arm64"},
		SupportsGUI:            true,
		Rootless:               true,
		DiagnosticDetails:      make(map[string]string),
	}

	if host.OS != "linux" {
		rep.Available = false
		rep.MissingPrerequisites = append(rep.MissingPrerequisites, "Wine execution is only supported on Linux hosts")
		return rep, nil
	}

	res, err := e.runner.Run(ctx, "wine", "--version")
	if err != nil {
		rep.Available = false
		rep.MissingPrerequisites = append(rep.MissingPrerequisites,
			"wine executable not found in PATH; install via: 'sudo dnf install -y wine' (Fedora) or 'sudo apt install -y wine' (Ubuntu)")
		return rep, nil
	}

	rep.Available = true
	rep.Version = strings.TrimSpace(res.Stdout)
	rep.DiagnosticDetails["wine_version"] = rep.Version

	// Check wineserver
	if _, errServer := exec.LookPath("wineserver"); errServer == nil {
		rep.DiagnosticDetails["wineserver"] = "Available"
	}
	// Check winetricks
	if _, errTricks := exec.LookPath("winetricks"); errTricks == nil {
		rep.DiagnosticDetails["winetricks"] = "Available"
	}

	return rep, nil
}

func (e *Engine) Plan(ctx context.Context, plan *domain.InstallPlan) error {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}

	prefixDir := filepath.Join(home, ".local", "share", "compat", "prefixes", plan.AppID)
	binDir := filepath.Join(home, ".local", "share", "compat", "bin")
	binPath := filepath.Join(binDir, plan.AppID)
	exeName := filepath.Base(plan.Package.Path)
	destExePath := filepath.Join(prefixDir, "drive_c", "Program Files", plan.Package.Name, exeName)

	plan.Decision.SupportTier = domain.Tier3Experimental
	plan.RequiresApproval = true

	plan.Steps = []domain.PlanStep{
		{Order: 1, Name: "Prefix Initialization", Description: fmt.Sprintf("Create isolated Wine prefix at: %s", prefixDir), HostImpact: "User-local prefix directory"},
		{Order: 2, Name: "Staging Executable", Description: fmt.Sprintf("Copy Windows executable to prefix: %s", destExePath), HostImpact: "Copies binary into prefix drive_c"},
		{Order: 3, Name: "Launcher Script", Description: fmt.Sprintf("Generate native Linux launcher script at: %s", binPath), HostImpact: "Creates user-local wrapper script"},
	}

	plan.HostChanges = []string{
		fmt.Sprintf("Wine prefix: %s", prefixDir),
		fmt.Sprintf("Launcher script: %s", binPath),
	}

	plan.Decision.Limitations = append(plan.Decision.Limitations,
		"Untested Windows application; Wine translation is best-effort",
		"DirectX 12 / anti-cheat drivers requiring kernel-level access will not function",
	)

	return nil
}

func (e *Engine) Apply(ctx context.Context, plan *domain.InstallPlan) error {
	// Verify wine is available
	if _, err := exec.LookPath("wine"); err != nil {
		return domain.NewError(domain.ErrCodeEngineUnavailable,
			"wine executable not found in PATH",
			"Install Wine via: 'sudo dnf install -y wine' (Fedora) or 'sudo apt install -y wine' (Ubuntu/Debian)",
			"Installation aborted")
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}

	prefixDir := filepath.Join(home, ".local", "share", "compat", "prefixes", plan.AppID)
	appDir := filepath.Join(prefixDir, "drive_c", "Program Files", plan.Package.Name)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed creating prefix directory structure", err)
	}

	// 1. Initialize Wine prefix non-interactively
	wineArch := "win64"
	if plan.Package.Architecture == "386" {
		wineArch = "win32"
	}

	env := []string{
		"WINEPREFIX=" + prefixDir,
		"WINEARCH=" + wineArch,
		"WINEDLLOVERRIDES=mscoree,mshtml=", // Disable prompts for Mono and Gecko
		"WINEDEBUG=-all",
	}

	_, _ = e.runner.RunWithOptions(ctx, runner.RunOptions{
		Command: "wineboot",
		Args:    []string{"-i"},
		Env:     env,
	})

	// 2. Copy the .exe safely
	exeName := filepath.Base(plan.Package.Path)
	destExePath := filepath.Join(appDir, exeName)

	srcFile, err := os.Open(plan.Package.Path)
	if err != nil {
		return domain.WrapError(domain.ErrCodePackageInvalid, "cannot open Windows executable", err)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(destExePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "cannot create destination executable in Wine prefix", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed copying executable to Wine prefix", err)
	}
	_ = destFile.Close()

	// 3. Generate native launcher script
	binDir := filepath.Join(home, ".local", "share", "compat", "bin")
	_ = os.MkdirAll(binDir, 0755)
	wrapperPath := filepath.Join(binDir, plan.AppID)

	script := fmt.Sprintf(`#!/usr/bin/env bash
# Generated by compat application manager for Windows application: %s
set -euo pipefail

export WINEPREFIX="%s"
export WINEARCH="%s"
export WINEDEBUG="-all"
export DXVK_LOG_LEVEL="none"

exec wine "%s" "$@"
`, plan.Package.Name, prefixDir, wineArch, destExePath)

	if err := os.WriteFile(wrapperPath, []byte(script), 0755); err != nil {
		return domain.WrapError(domain.ErrCodeRunnerFailed, "failed writing Wine launcher script", err)
	}

	return nil
}

func (e *Engine) Launch(ctx context.Context, app *domain.ApplicationRecord, args []string) error {
	if app.LauncherPath == "" {
		return domain.NewError(domain.ErrCodeAppNotFound, "launcher path not set for application", "", "")
	}

	if _, err := os.Stat(app.LauncherPath); err != nil {
		return domain.NewError(domain.ErrCodeAppNotFound, "launcher script not found: "+app.LauncherPath, "", "")
	}

	cmd := exec.CommandContext(ctx, app.LauncherPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *Engine) Status(ctx context.Context, app *domain.ApplicationRecord) (domain.LifecycleState, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	prefixDir := filepath.Join(home, ".local", "share", "compat", "prefixes", app.AppID)
	if _, err := os.Stat(prefixDir); err != nil {
		return domain.StateInstallFailed, nil
	}

	// Check if wineserver is currently running processes for this prefix
	res, err := e.runner.RunWithOptions(ctx, runner.RunOptions{
		Command: "wineserver",
		Args:    []string{"-p"},
		Env:     []string{"WINEPREFIX=" + prefixDir},
	})
	if err == nil && res.ExitCode == 0 {
		return domain.StateRunning, nil
	}

	return domain.StateInstalled, nil
}

func (e *Engine) Logs(ctx context.Context, app *domain.ApplicationRecord, lines int) ([]string, error) {
	return nil, nil
}

func (e *Engine) Uninstall(ctx context.Context, app *domain.ApplicationRecord) error {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}

	prefixDir := filepath.Join(home, ".local", "share", "compat", "prefixes", app.AppID)

	// Terminate any running processes in the prefix
	_, _ = e.runner.RunWithOptions(ctx, runner.RunOptions{
		Command: "wineserver",
		Args:    []string{"-k"},
		Env:     []string{"WINEPREFIX=" + prefixDir},
	})

	// Remove prefix directory
	_ = os.RemoveAll(prefixDir)

	// Remove launcher wrapper
	if app.LauncherPath != "" {
		_ = os.Remove(app.LauncherPath)
	}

	return nil
}
