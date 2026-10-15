package tarball

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
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

func writeTestTar(t *testing.T, path string, hdrs []*tar.Header, bodies map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed creating tar: %v", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for _, hdr := range hdrs {
		body := bodies[hdr.Name]
		hdr.Size = int64(len(body))
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed writing header %s: %v", hdr.Name, err)
		}
		tw.Write([]byte(body))
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed closing tar: %v", err)
	}
}

func TestExtractTarPreservesLinks(t *testing.T) {
	tmpDir := t.TempDir()
	archive := filepath.Join(tmpDir, "app.tar")
	writeTestTar(t, archive, []*tar.Header{
		{Name: "app/lib/libfoo.so.1.2.3", Typeflag: tar.TypeReg, Mode: 0755},
		{Name: "app/lib/libfoo.so.1", Typeflag: tar.TypeSymlink, Linkname: "libfoo.so.1.2.3"},
		{Name: "app/lib/libfoo-hard.so", Typeflag: tar.TypeLink, Linkname: "app/lib/libfoo.so.1.2.3"},
		{Name: "../escaped.txt", Typeflag: tar.TypeReg, Mode: 0644},
	}, map[string]string{
		"app/lib/libfoo.so.1.2.3": "ELF-ish",
		"../escaped.txt":          "should not be written",
	})

	dest := filepath.Join(tmpDir, "dest")
	os.MkdirAll(dest, 0755)
	if err := extractArchive(archive, dest); err != nil {
		t.Fatalf("extractArchive failed: %v", err)
	}

	link := filepath.Join(dest, "app", "lib", "libfoo.so.1")
	if target, err := os.Readlink(link); err != nil || target != "libfoo.so.1.2.3" {
		t.Errorf("expected symlink to libfoo.so.1.2.3, got %q (err %v)", target, err)
	}
	if data, _ := os.ReadFile(link); string(data) != "ELF-ish" {
		t.Errorf("expected symlink to resolve to library contents, got %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "app", "lib", "libfoo-hard.so")); string(data) != "ELF-ish" {
		t.Errorf("expected hardlink to share library contents, got %q", data)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "escaped.txt")); !os.IsNotExist(err) {
		t.Errorf("traversal entry was written outside the target directory")
	}
}

func TestExtractTarRefusesWritesThroughEscapingSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	outside := filepath.Join(tmpDir, "outside")
	os.MkdirAll(outside, 0755)

	archive := filepath.Join(tmpDir, "evil.tar")
	writeTestTar(t, archive, []*tar.Header{
		{Name: "evil", Typeflag: tar.TypeSymlink, Linkname: outside},
		{Name: "evil/pwned.txt", Typeflag: tar.TypeReg, Mode: 0644},
	}, map[string]string{"evil/pwned.txt": "pwned"})

	dest := filepath.Join(tmpDir, "dest")
	os.MkdirAll(dest, 0755)
	if err := extractArchive(archive, dest); err == nil {
		t.Errorf("expected extraction to fail when writing through an escaping symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
		t.Fatalf("file was written outside the target directory through a symlink")
	}
}

func TestExtractZipPreservesSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	archive := filepath.Join(tmpDir, "app.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("failed creating zip: %v", err)
	}
	zw := zip.NewWriter(f)
	lib, _ := zw.CreateHeader(&zip.FileHeader{Name: "lib/libbar.so.2.0"})
	lib.Write([]byte("bar"))
	linkHdr := &zip.FileHeader{Name: "lib/libbar.so.2"}
	linkHdr.SetMode(os.ModeSymlink | 0777)
	link, _ := zw.CreateHeader(linkHdr)
	link.Write([]byte("libbar.so.2.0"))
	zw.Close()
	f.Close()

	dest := filepath.Join(tmpDir, "dest")
	os.MkdirAll(dest, 0755)
	if err := extractArchive(archive, dest); err != nil {
		t.Fatalf("extractArchive failed: %v", err)
	}

	target, err := os.Readlink(filepath.Join(dest, "lib", "libbar.so.2"))
	if err != nil || target != "libbar.so.2.0" {
		t.Errorf("expected zip symlink to libbar.so.2.0, got %q (err %v)", target, err)
	}
}

// File names inside an archive are untrusted and end up in the launcher
// script; they must not be expanded by the shell when the app is launched.
func TestTarballWrapperDoesNotExpandArchivePaths(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	evilName := "app/bin/run$(touch PWNED)"
	tarPath := filepath.Join(tmpDir, "evil.tar")
	writeTestTar(t, tarPath, []*tar.Header{
		{Name: evilName, Typeflag: tar.TypeReg, Mode: 0755},
	}, map[string]string{evilName: "#!/bin/sh\necho launched\n"})

	eng := New(runner.New())
	plan := &domain.InstallPlan{
		AppID:   "evil-app",
		Package: domain.PackageInfo{Path: tarPath, Name: "evil", Format: domain.FormatTarball},
	}
	if err := eng.Apply(context.Background(), plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	cmd := exec.Command(filepath.Join(tmpDir, ".local", "share", "compat", "bin", "evil-app"))
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if _, statErr := os.Stat(filepath.Join(tmpDir, "PWNED")); statErr == nil {
		t.Fatalf("launcher executed a command embedded in an archive file name")
	}
	if err != nil || string(out) != "launched\n" {
		t.Errorf("expected wrapper to run the app, got %q (err %v)", out, err)
	}
}
