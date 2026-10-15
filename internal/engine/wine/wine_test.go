package wine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

type mockRunner struct {
	commands []string
	version  string
}

func (m *mockRunner) Run(ctx context.Context, command string, args ...string) (*runner.RunResult, error) {
	cmdStr := command + " " + strings.Join(args, " ")
	m.commands = append(m.commands, cmdStr)
	if command == "wine" && len(args) > 0 && args[0] == "--version" {
		return &runner.RunResult{ExitCode: 0, Stdout: m.version}, nil
	}
	return &runner.RunResult{ExitCode: 0}, nil
}

func (m *mockRunner) RunWithOptions(ctx context.Context, opts runner.RunOptions) (*runner.RunResult, error) {
	cmdStr := opts.Command + " " + strings.Join(opts.Args, " ")
	m.commands = append(m.commands, cmdStr)
	if opts.Command == "wineserver" && len(opts.Args) > 0 && opts.Args[0] == "-p" {
		return &runner.RunResult{ExitCode: 1}, nil
	}
	return &runner.RunResult{ExitCode: 0}, nil
}

func TestWineEngineProbe(t *testing.T) {
	mockR := &mockRunner{version: "wine-9.0\n"}
	eng := New(mockR)

	host := &domain.HostProfile{OS: "linux"}
	rep, err := eng.Probe(context.Background(), host)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	if !rep.Available {
		t.Errorf("expected wine engine to be available")
	}
	if rep.Version != "wine-9.0" {
		t.Errorf("expected version wine-9.0, got %s", rep.Version)
	}
}

func TestWineEnginePlan(t *testing.T) {
	mockR := &mockRunner{version: "wine-9.0\n"}
	eng := New(mockR)

	plan := &domain.InstallPlan{
		AppID: "test-wine-app-1234",
		Package: domain.PackageInfo{
			Name:         "testapp",
			Path:         "/tmp/testapp.exe",
			Format:       domain.FormatWindowsExe,
			Architecture: "amd64",
		},
		Decision: domain.StrategyDecision{},
	}

	err := eng.Plan(context.Background(), plan)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	if plan.Decision.SupportTier != domain.Tier3Experimental {
		t.Errorf("expected Tier 3 Experimental, got %s", plan.Decision.SupportTier)
	}

	if !plan.RequiresApproval {
		t.Errorf("expected RequiresApproval to be true for Tier 3")
	}

	if len(plan.Steps) == 0 {
		t.Errorf("expected non-empty plan steps")
	}
}

func TestWineEngineApplyAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create fake source exe
	srcExe := filepath.Join(tmpDir, "sample.exe")
	if err := os.WriteFile(srcExe, []byte("MZfake-pe-binary"), 0755); err != nil {
		t.Fatalf("failed creating sample.exe: %v", err)
	}

	mockR := &mockRunner{version: "wine-9.0\n"}
	eng := New(mockR)

	plan := &domain.InstallPlan{
		AppID: "sample-win-app-5678",
		Package: domain.PackageInfo{
			Name:         "sample",
			Path:         srcExe,
			Format:       domain.FormatWindowsExe,
			Architecture: "amd64",
		},
	}

	// Fake lookPath by creating a mock wine binary in a temp bin on PATH
	binDir := filepath.Join(tmpDir, "testbin")
	_ = os.MkdirAll(binDir, 0755)
	fakeWine := filepath.Join(binDir, "wine")
	_ = os.WriteFile(fakeWine, []byte("#!/bin/sh\nexit 0\n"), 0755)
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	err := eng.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Verify prefix directory was created
	prefixDir := filepath.Join(tmpDir, ".local", "share", "compat", "prefixes", plan.AppID)
	if _, err := os.Stat(prefixDir); err != nil {
		t.Errorf("expected prefix dir %s to exist", prefixDir)
	}

	// Verify executable was copied to prefix
	destExe := filepath.Join(prefixDir, "drive_c", "Program Files", "sample", "sample.exe")
	if _, err := os.Stat(destExe); err != nil {
		t.Errorf("expected dest exe %s to exist", destExe)
	}

	// Verify launcher wrapper was created
	wrapperPath := filepath.Join(tmpDir, ".local", "share", "compat", "bin", plan.AppID)
	wrapperContent, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed reading wrapper: %v", err)
	}

	if !strings.Contains(string(wrapperContent), "WINEPREFIX=") {
		t.Errorf("expected wrapper script to set WINEPREFIX")
	}

	// Test status
	appRec := &domain.ApplicationRecord{
		AppID:        plan.AppID,
		LauncherPath: wrapperPath,
	}
	st, err := eng.Status(context.Background(), appRec)
	if err != nil || st != domain.StateInstalled {
		t.Errorf("expected status installed, got %s (err: %v)", st, err)
	}

	// Test uninstall
	err = eng.Uninstall(context.Background(), appRec)
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if _, err := os.Stat(prefixDir); !os.IsNotExist(err) {
		t.Errorf("expected prefix dir to be deleted after uninstall")
	}
	if _, err := os.Stat(wrapperPath); !os.IsNotExist(err) {
		t.Errorf("expected wrapper to be deleted after uninstall")
	}
}
