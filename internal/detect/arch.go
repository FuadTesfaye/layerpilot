package detect

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func (d *Detector) inspectArchPackage(f *os.File, info *domain.PackageInfo, magic []byte) (*domain.PackageInfo, error) {
	info.Format = domain.FormatArchPkg
	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(base, ".zst"), ".xz"), ".pkg.tar")

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek arch package", err)
	}

	var tr *tar.Reader
	// Detect compression
	if len(magic) >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd {
		// zstd
		zr, err := zstd.NewReader(f)
		if err != nil {
			return info, nil
		}
		defer zr.Close()
		tr = tar.NewReader(zr)
	} else if len(magic) >= 6 && magic[0] == 0xfd && magic[1] == '7' && magic[2] == 'z' && magic[3] == 'X' && magic[4] == 'Z' && magic[5] == 0x00 {
		// xz
		xr, err := xz.NewReader(f)
		if err != nil {
			return info, nil
		}
		tr = tar.NewReader(xr)
	} else if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		// gz
		gr, err := gzip.NewReader(f)
		if err != nil {
			return info, nil
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(f)
	}

	// Search for .PKGINFO
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		clean := strings.TrimPrefix(hdr.Name, "./")
		if clean == ".PKGINFO" {
			scanner := bufio.NewScanner(io.LimitReader(tr, 256*1024))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])

				switch key {
				case "pkgname":
					info.Name = val
				case "pkgver":
					info.Version = val
				case "pkgdesc":
					info.Description = val
				case "arch":
					info.Architecture = normalizeArch(val)
				case "depend":
					info.Dependencies = append(info.Dependencies, val)
					depLower := strings.ToLower(val)
					if strings.Contains(depLower, "gtk") || strings.Contains(depLower, "qt") || strings.Contains(depLower, "xorg") || strings.Contains(depLower, "wayland") {
						info.PackageType = domain.PackageTypeGUI
					}
				}
			}
			break
		}
	}

	return info, nil
}

func isArchPackage(name string, magic []byte) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, ".pkg.tar.") {
		return true
	}
	return false
}
