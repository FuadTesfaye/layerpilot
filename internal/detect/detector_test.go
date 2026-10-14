package detect

import (
	"archive/tar"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
