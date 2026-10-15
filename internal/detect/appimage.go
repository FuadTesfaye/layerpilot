package detect

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectAppImage(f *os.File, info *domain.PackageInfo, magic []byte) (*domain.PackageInfo, error) {
	isAppImage := false
	appImageType := "unknown"

	// Byte 8..11 check for 'A', 'I', 0x01 or 0x02
	if len(magic) >= 11 && magic[8] == 'A' && magic[9] == 'I' {
		if magic[10] == 0x01 {
			isAppImage = true
			appImageType = "Type 1"
		} else if magic[10] == 0x02 {
			isAppImage = true
			appImageType = "Type 2"
		}
	}

	// Perform static ELF scan on the executable wrapper (collects arch, interpreter, DT_NEEDED libs, glibc versions)
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		_ = ScanStaticELFReader(f, info)
	}

	// Fallback to raw ELF header parsing for architecture if static scan was partial
	if info.Architecture == "" {
		elfHdr := make([]byte, 64)
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if _, err := io.ReadFull(f, elfHdr); err == nil && len(elfHdr) >= 20 {
				var machine uint16
				if elfHdr[5] == 1 { // little endian
					machine = uint16(elfHdr[18]) | uint16(elfHdr[19])<<8
				} else {
					machine = uint16(elfHdr[19]) | uint16(elfHdr[18])<<8
				}
				switch machine {
				case 0x3E:
					info.Architecture = "amd64"
				case 0xB7:
					info.Architecture = "arm64"
				case 0x03:
					info.Architecture = "386"
				case 0x28:
					info.Architecture = "arm"
				}
			}
		}
	}

	info.Format = domain.FormatAppImage
	info.PackageType = domain.PackageTypeGUI // AppImages default to desktop applications
	info.Metadata["appimage_type"] = appImageType

	// Detect squashfs payload (Type 2 AppImages embed squashfs payload after the ELF runtime)
	searchBuf := make([]byte, 512*1024)
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		if n, _ := io.ReadFull(f, searchBuf); n > 0 {
			if bytes.Contains(searchBuf[:n], []byte("hsqs")) {
				info.Metadata["payload"] = "squashfs"
				if appImageType == "unknown" {
					appImageType = "Type 2"
					info.Metadata["appimage_type"] = "Type 2"
					isAppImage = true
				}
			}
		}
	}

	// Clean base filename
	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(strings.TrimSuffix(base, ".AppImage"), ".appimage")

	// If no explicit AI magic was found, but filename has .AppImage extension, still allow it
	if !isAppImage && !strings.HasSuffix(strings.ToLower(base), ".appimage") {
		// It's a raw ELF binary, not an AppImage
		info.Format = domain.FormatUnknown
		return info, domain.NewError(
			domain.ErrCodeFormatUnsupported,
			"file is a standard ELF executable, not an AppImage container",
			"AppImages require embedded AI magic headers or .AppImage packaging",
			"Direct execution requires AppImage format",
		)
	}

	return info, nil
}
