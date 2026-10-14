package detect

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectAppImage(f *os.File, info *domain.PackageInfo, magic []byte) (*domain.PackageInfo, error) {
	// Verify ELF and AppImage magic
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

	// Read ELF header fields to determine architecture
	elfHdr := make([]byte, 64)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek ELF header", err)
	}
	if _, err := io.ReadFull(f, elfHdr); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot read ELF header", err)
	}

	// e_machine is at offset 18 (2 bytes)
	var machine uint16
	if elfHdr[5] == 1 { // little endian
		machine = binary.LittleEndian.Uint16(elfHdr[18:20])
	} else {
		machine = binary.BigEndian.Uint16(elfHdr[18:20])
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
	default:
		info.Architecture = "unknown"
	}

	info.Format = domain.FormatAppImage
	info.PackageType = domain.PackageTypeGUI // AppImages default to desktop applications
	info.Metadata["appimage_type"] = appImageType

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
