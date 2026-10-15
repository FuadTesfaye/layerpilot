package detect

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectFlatpak(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatFlatpak
	info.PackageType = domain.PackageTypeGUI
	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(strings.TrimSuffix(base, ".flatpak"), ".flatpakref")

	// If .flatpakref, parse key-value lines
	if strings.HasSuffix(strings.ToLower(info.Path), ".flatpakref") {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			scanner := bufio.NewScanner(io.LimitReader(f, 64*1024))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "Name=") {
					info.Name = strings.TrimPrefix(line, "Name=")
				} else if strings.HasPrefix(line, "Branch=") {
					info.Version = strings.TrimPrefix(line, "Branch=")
				} else if strings.HasPrefix(line, "Title=") {
					info.Description = strings.TrimPrefix(line, "Title=")
				} else if strings.HasPrefix(line, "Url=") {
					info.Metadata["repo_url"] = strings.TrimPrefix(line, "Url=")
				}
			}
		}
	}

	return info, nil
}

func (d *Detector) inspectSnap(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatSnap
	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(base, ".snap")

	// Inspect snap squashfs for meta/snap.yaml
	// If squashfs reading is available, extract name/version/apps
	info.Metadata["snap_type"] = "squashfs"

	return info, nil
}

func isFlatpak(name string, magic []byte) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".flatpak") || strings.HasSuffix(lower, ".flatpakref") {
		return true
	}
	return false
}

func isSnap(name string, magic []byte) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".snap") {
		return true
	}
	// Check squashfs magic 'hsqs'
	if len(magic) >= 4 && magic[0] == 0x68 && magic[1] == 0x73 && magic[2] == 0x71 && magic[3] == 0x73 {
		if strings.HasSuffix(lower, ".snap") {
			return true
		}
	}
	return false
}
