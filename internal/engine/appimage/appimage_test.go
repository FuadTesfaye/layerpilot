package appimage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

func TestAppImageEngineProbe(t *testing.T) {
	r := runner.New()
	eng := New(r)

	hostLinux := &domain.HostProfile{
		OS:          "linux",
		Arch:        "amd64",
		FUSESupport: true,
	}

	rep, err := eng.Probe(context.Background(), hostLinux)
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !rep.Available {
		t.Errorf("expected engine to be available on Linux host")
	}

	hostDarwin := &domain.HostProfile{
		OS:   "darwin",
		Arch: "arm64",
	}
	repDarwin, err := eng.Probe(context.Background(), hostDarwin)
	if err != nil {
		t.Fatalf("Probe on Darwin failed: %v", err)
	}
	if repDarwin.Available {
		t.Errorf("expected engine to be unavailable on Darwin host")
	}
}

func TestAppImagePlan(t *testing.T) {
	r := runner.New()
	eng := New(r)

	plan := &domain.InstallPlan{
		AppID: "test-app",
		Package: domain.PackageInfo{
			Path:      "/tmp/fake.AppImage",
			SizeBytes: 1024,
		},
	}

	err := eng.Plan(context.Background(), plan)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(plan.Steps))
	}
}

func TestAppImageExecPathForApp(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	appID := "test-app"
	appDir := filepath.Join(tmpDir, ".local", "share", "compat", "apps", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. When empty, should return error
	if _, err := ExecPathForApp(appID); err == nil {
		t.Errorf("expected error when no binary present")
	}

	// 2. When app.AppImage exists
	appImagePath := filepath.Join(appDir, "app.AppImage")
	if err := os.WriteFile(appImagePath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	p, err := ExecPathForApp(appID)
	if err != nil || p != appImagePath {
		t.Errorf("expected %s, got %s (err: %v)", appImagePath, p, err)
	}

	// 3. When AppRun exists, it takes precedence (extracted AppDir)
	appRunPath := filepath.Join(appDir, "AppRun")
	if err := os.WriteFile(appRunPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	p2, err := ExecPathForApp(appID)
	if err != nil || p2 != appRunPath {
		t.Errorf("expected %s to take precedence, got %s", appRunPath, p2)
	}
}

func TestAppImageStatusAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	appID := "test-status-app"
	appDir := filepath.Join(tmpDir, ".local", "share", "compat", "apps", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app.AppImage"), []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	r := runner.New()
	eng := New(r)

	rec := &domain.ApplicationRecord{AppID: appID}
	st, err := eng.Status(context.Background(), rec)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if st != domain.StateInstalled {
		t.Errorf("expected StateInstalled, got %s", st)
	}

	if err := eng.Uninstall(context.Background(), rec); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	st2, err := eng.Status(context.Background(), rec)
	if err != nil {
		t.Fatalf("Status post-uninstall failed: %v", err)
	}
	if st2 != domain.StateRemoved {
		t.Errorf("expected StateRemoved, got %s", st2)
	}
}
