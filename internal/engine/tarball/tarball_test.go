package tarball

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

func TestTarballLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Create a dummy tar.gz with an executable script
	tarPath := filepath.Join(tmpDir, "sample-app.tar.gz")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("failed creating tar file: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	scriptContent := []byte("#!/bin/sh\necho 'hello from test app'\n")
	hdr := &tar.Header{
		Name: "sample-app/bin/sample-app",
		Mode: 0755,
		Size: int64(len(scriptContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed writing header: %v", err)
	}
	if _, err := tw.Write(scriptContent); err != nil {
		t.Fatalf("failed writing content: %v", err)
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	r := runner.New()
	eng := New(r)

	if eng.ID() != "tarball" {
		t.Fatalf("unexpected engine ID: %s", eng.ID())
	}

	plan := &domain.InstallPlan{
		AppID: "sample-app",
		Package: domain.PackageInfo{
			Path:      tarPath,
			Name:      "sample-app",
			Format:    domain.FormatTarball,
			SizeBytes: 1024,
		},
	}

	ctx := context.Background()
	if err := eng.Plan(ctx, plan); err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	if len(plan.Steps) == 0 {
		t.Fatalf("expected plan steps")
	}

	if err := eng.Apply(ctx, plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Verify installed files
	installedBin := filepath.Join(tmpDir, ".local", "opt", "sample-app", "sample-app", "bin", "sample-app")
	if _, err := os.Stat(installedBin); err != nil {
		t.Fatalf("expected binary at %s: %v", installedBin, err)
	}

	wrapperBin := filepath.Join(tmpDir, ".local", "share", "compat", "bin", "sample-app")
	if _, err := os.Stat(wrapperBin); err != nil {
		t.Fatalf("expected wrapper at %s: %v", wrapperBin, err)
	}

	appRec := &domain.ApplicationRecord{
		AppID:       "sample-app",
		DisplayName: "sample-app",
	}

	status, err := eng.Status(ctx, appRec)
	if err != nil || status != domain.StateInstalled {
		t.Fatalf("expected StateInstalled, got %s, err: %v", status, err)
	}

	// Uninstall
	if err := eng.Uninstall(ctx, appRec); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	statusAfter, _ := eng.Status(ctx, appRec)
	if statusAfter != domain.StateRemoved {
		t.Fatalf("expected StateRemoved after uninstall, got %s", statusAfter)
	}
}
