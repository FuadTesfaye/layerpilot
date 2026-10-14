package detect

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectPE(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatWindowsExe

	// Read e_lfanew at offset 0x3C (4 bytes)
	if _, err := f.Seek(0x3C, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek PE offset", err)
	}

	var peOffset uint32
	if err := binary.Read(f, binary.LittleEndian, &peOffset); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot read PE header offset", err)
	}

	// Seek to PE signature
	if _, err := f.Seek(int64(peOffset), io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek to PE header", err)
	}

	peSig := make([]byte, 4)
	if _, err := io.ReadFull(f, peSig); err != nil || string(peSig) != "PE\x00\x00" {
		return nil, domain.NewError(domain.ErrCodePackageInvalid, "invalid or missing PE signature in DOS executable", "Provide a valid Windows PE binary", "Inspection failed")
	}

	// Read COFF machine type (2 bytes)
	var machine uint16
	if err := binary.Read(f, binary.LittleEndian, &machine); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot read COFF machine", err)
	}

	switch machine {
	case 0x8664:
		info.Architecture = "amd64"
	case 0xAA64:
		info.Architecture = "arm64"
	case 0x014C:
		info.Architecture = "386"
	default:
		info.Architecture = "unknown"
	}

	// Read Optional Header Subsystem
	// Offset: peOffset + 4 (sig) + 20 (coff) + 68 (subsystem in standard optional header)
	if _, err := f.Seek(int64(peOffset)+24+68, io.SeekStart); err == nil {
		var subsystem uint16
		if err := binary.Read(f, binary.LittleEndian, &subsystem); err == nil {
			if subsystem == 2 {
				info.PackageType = domain.PackageTypeGUI
			} else if subsystem == 3 {
				info.PackageType = domain.PackageTypeCLI
			}
		}
	}

	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(base, filepath.Ext(base))

	return info, nil
}
