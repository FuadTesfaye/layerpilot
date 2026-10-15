package detect

import (
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func TestDetectDebPackage(t *testing.T) {
	tmpDir := t.TempDir()
	debPath := filepath.Join(tmpDir, "testapp_1.0.0_amd64.deb")

	// Create valid synthetic deb
	f, err := os.Create(debPath)
	if err != nil {
		t.Fatalf("failed to create deb file: %v", err)
	}

	// 1. Write ar magic
	f.WriteString("!<arch>\n")

	// 2. Write debian-binary entry
	debBinaryContent := "2.0\n"
	writeArHeader(t, f, "debian-binary", int64(len(debBinaryContent)))
	f.WriteString(debBinaryContent)

	// 3. Create control.tar.gz
	controlContent := `Package: testapp
Version: 1.0.0
Architecture: amd64
Depends: libc6, libgtk-3-0
Section: x11
Description: Test GUI Application for compatibility runner
`
	var controlTarGz []byte
	{
		var buf []byte
		gzBuf := &byteBufferWriter{buf: &buf}
		gw := gzip.NewWriter(gzBuf)
		tw := tar.NewWriter(gw)

		hdr := &tar.Header{
			Name: "control",
			Mode: 0644,
			Size: int64(len(controlContent)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed writing tar header: %v", err)
		}
		if _, err := tw.Write([]byte(controlContent)); err != nil {
			t.Fatalf("failed writing tar body: %v", err)
		}
		tw.Close()
		gw.Close()
		controlTarGz = buf
	}

	writeArHeader(t, f, "control.tar.gz", int64(len(controlTarGz)))
	f.Write(controlTarGz)
	if len(controlTarGz)%2 != 0 {
		f.WriteString("\n")
	}
	f.Close()

	// Inspect
	detector := New()
	info, err := detector.Inspect(debPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if info.Format != domain.FormatDeb {
		t.Errorf("expected format %s, got %s", domain.FormatDeb, info.Format)
	}
	if info.Name != "testapp" {
		t.Errorf("expected name 'testapp', got '%s'", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", info.Version)
	}
	if info.Architecture != "amd64" {
		t.Errorf("expected architecture 'amd64', got '%s'", info.Architecture)
	}
	if info.PackageType != domain.PackageTypeGUI {
		t.Errorf("expected GUI package type, got %s", info.PackageType)
	}
	if len(info.SHA256) != 64 {
		t.Errorf("expected 64-char sha256 hash, got %d chars", len(info.SHA256))
	}
}

func TestDetectAppImage(t *testing.T) {
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "SampleApp-x86_64.AppImage")

	f, err := os.Create(appPath)
	if err != nil {
		t.Fatalf("failed to create AppImage file: %v", err)
	}

	// Create synthetic AppImage Type 2 x86_64 header (64 bytes)
	hdr := make([]byte, 64)
	copy(hdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	hdr[4] = 2 // 64-bit
	hdr[5] = 1 // Little endian
	// AppImage magic at byte 8: 'A', 'I', 0x02
	hdr[8] = 'A'
	hdr[9] = 'I'
	hdr[10] = 0x02
	// e_machine at byte 18: 0x3E (x86-64)
	binary.LittleEndian.PutUint16(hdr[18:20], 0x3E)

	f.Write(hdr)
	f.Close()

	detector := New()
	info, err := detector.Inspect(appPath)
	if err != nil {
		t.Fatalf("Inspect AppImage failed: %v", err)
	}

	if info.Format != domain.FormatAppImage {
		t.Errorf("expected format %s, got %s", domain.FormatAppImage, info.Format)
	}
	if info.Architecture != "amd64" {
		t.Errorf("expected arch 'amd64', got '%s'", info.Architecture)
	}
	if info.PackageType != domain.PackageTypeGUI {
		t.Errorf("expected GUI package type, got %s", info.PackageType)
	}
}

func TestDetectCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupted.deb")
	os.WriteFile(corruptPath, []byte("too short"), 0644)

	detector := New()
	_, err := detector.Inspect(corruptPath)
	if err == nil {
		t.Fatalf("expected error on corrupted package, got nil")
	}
}

// Helpers
// Ubuntu 21.10+ ships debs whose control archive is control.tar.zst. If it is
// not parsed, Name falls back to the file name and native uninstall breaks.
func TestDetectDebPackageZstdControl(t *testing.T) {
	tmpDir := t.TempDir()
	debPath := filepath.Join(tmpDir, "zstdapp_2.1.0-1ubuntu1_amd64.deb")

	f, err := os.Create(debPath)
	if err != nil {
		t.Fatalf("failed to create deb file: %v", err)
	}
	f.WriteString("!<arch>\n")

	debBinaryContent := "2.0\n"
	writeArHeader(t, f, "debian-binary", int64(len(debBinaryContent)))
	f.WriteString(debBinaryContent)

	controlContent := "Package: zstdapp\nVersion: 2.1.0-1ubuntu1\nArchitecture: amd64\n"
	var controlTarZst []byte
	{
		zw, err := zstd.NewWriter(&byteBufferWriter{buf: &controlTarZst})
		if err != nil {
			t.Fatalf("failed creating zstd writer: %v", err)
		}
		tw := tar.NewWriter(zw)
		if err := tw.WriteHeader(&tar.Header{Name: "./control", Mode: 0644, Size: int64(len(controlContent))}); err != nil {
			t.Fatalf("failed writing tar header: %v", err)
		}
		tw.Write([]byte(controlContent))
		tw.Close()
		zw.Close()
	}

	writeArHeader(t, f, "control.tar.zst", int64(len(controlTarZst)))
	f.Write(controlTarZst)
	if len(controlTarZst)%2 != 0 {
		f.WriteString("\n")
	}
	f.Close()

	info, err := New().Inspect(debPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if info.Name != "zstdapp" {
		t.Errorf("expected name 'zstdapp' from control file, got '%s'", info.Name)
	}
	if info.Version != "2.1.0-1ubuntu1" {
		t.Errorf("expected version '2.1.0-1ubuntu1', got '%s'", info.Version)
	}
}

func writeArHeader(t *testing.T, f *os.File, name string, size int64) {
	hdr := make([]byte, 60)
	copy(hdr[0:16], fmt.Sprintf("%-16s", name+"/"))
	copy(hdr[16:28], fmt.Sprintf("%-12s", "1720000000"))
	copy(hdr[28:34], fmt.Sprintf("%-6s", "0"))
	copy(hdr[34:40], fmt.Sprintf("%-6s", "0"))
	copy(hdr[40:48], fmt.Sprintf("%-8s", "100644"))
	copy(hdr[48:58], fmt.Sprintf("%-10d", size))
	copy(hdr[58:60], "`\n")
	if _, err := f.Write(hdr); err != nil {
		t.Fatalf("failed writing ar header: %v", err)
	}
}

type byteBufferWriter struct {
	buf *[]byte
}

func (w *byteBufferWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func TestDetectTarball(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "myapp-v1.0.tar.gz")

	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "bin/myapp",
		Mode: 0755,
		Size: 4,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte("test"))
	tw.Close()
	gw.Close()
	f.Close()

	d := New()
	info, err := d.Inspect(tarPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if info.Format != domain.FormatTarball {
		t.Errorf("expected format tarball, got %s", info.Format)
	}
	if len(info.BinaryTargets) == 0 || info.BinaryTargets[0] != "bin/myapp" {
		t.Errorf("expected binary target bin/myapp, got %v", info.BinaryTargets)
	}
}

func TestDetectArchPackage(t *testing.T) {
	tmpDir := t.TempDir()
	archPath := filepath.Join(tmpDir, "archapp-2.0-1-x86_64.pkg.tar.gz")

	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	pkginfo := "pkgname = archapp\npkgver = 2.0-1\narch = x86_64\npkgdesc = An Arch package\n"
	hdr := &tar.Header{
		Name: ".PKGINFO",
		Mode: 0644,
		Size: int64(len(pkginfo)),
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte(pkginfo))
	tw.Close()
	gw.Close()
	f.Close()

	d := New()
	info, err := d.Inspect(archPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if info.Format != domain.FormatArchPkg {
		t.Errorf("expected archpkg format, got %s", info.Format)
	}
	if info.Name != "archapp" {
		t.Errorf("expected archapp name, got %s", info.Name)
	}
	if info.Architecture != "amd64" {
		t.Errorf("expected amd64 arch, got %s", info.Architecture)
	}
}

func TestDetectSelfExtract(t *testing.T) {
	tmpDir := t.TempDir()
	shPath := filepath.Join(tmpDir, "vendor-installer.run")

	content := "#!/bin/sh\n# Vendor Setup Script\necho 'extracting payload'\nexit 0\n"
	if err := os.WriteFile(shPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	d := New()
	info, err := d.Inspect(shPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if info.Format != domain.FormatSelfExtract {
		t.Errorf("expected run_sh format, got %s", info.Format)
	}
	if info.SelfExtractWarning == "" {
		t.Errorf("expected warning on self-extracting script")
	}
}

func TestDetectFlatpakRef(t *testing.T) {
	tmpDir := t.TempDir()
	fpPath := filepath.Join(tmpDir, "org.example.App.flatpakref")

	content := "[Flatpak Ref]\nName=org.example.App\nBranch=stable\nTitle=Example App\nUrl=https://repo.example.com\n"
	if err := os.WriteFile(fpPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := New()
	info, err := d.Inspect(fpPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if info.Format != domain.FormatFlatpak {
		t.Errorf("expected flatpak format, got %s", info.Format)
	}
	if info.Name != "org.example.App" {
		t.Errorf("expected org.example.App name, got %s", info.Name)
	}
}

func TestScanStaticELFOnSystemBinary(t *testing.T) {
	catPath := "/usr/bin/cat"
	if _, err := os.Stat(catPath); err != nil {
		t.Skip("system binary /usr/bin/cat not available")
	}

	info := &domain.PackageInfo{
		Metadata: make(map[string]string),
	}
	err := ScanStaticELF(catPath, info)
	if err != nil {
		t.Fatalf("ScanStaticELF failed: %v", err)
	}

	if info.Architecture != "amd64" && info.Architecture != "arm64" {
		t.Errorf("expected valid architecture, got %s", info.Architecture)
	}
	if len(info.NeededSharedLibs) == 0 {
		t.Errorf("expected needed shared libs, got none")
	}
	if len(info.GlibcVersions) == 0 {
		t.Errorf("expected glibc version requirements, got none")
	}
}
