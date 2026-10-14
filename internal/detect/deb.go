package detect

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectDeb(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatDeb

	if _, err := f.Seek(8, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "failed to seek in debian archive", err)
	}

	headerBuf := make([]byte, 60)
	for {
		_, err := io.ReadFull(f, headerBuf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, domain.WrapError(domain.ErrCodePackageInvalid, "malformed ar header in debian archive", err)
		}

		name := strings.TrimSpace(string(headerBuf[0:16]))
		name = strings.TrimSuffix(name, "/")
		sizeStr := strings.TrimSpace(string(headerBuf[48:58]))
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return nil, domain.WrapError(domain.ErrCodePackageInvalid, "invalid entry size in debian ar archive", err)
		}

		// Read control archive
		if strings.HasPrefix(name, "control.tar") {
			limitReader := io.LimitReader(f, size)
			if err := d.parseDebControlTar(name, limitReader, info); err != nil {
				return nil, err
			}
			break
		}

		// Skip data with 2-byte alignment
		skipBytes := size
		if skipBytes%2 != 0 {
			skipBytes++
		}
		if _, err := f.Seek(skipBytes, io.SeekCurrent); err != nil {
			return nil, domain.WrapError(domain.ErrCodePackageInvalid, "failed to seek past ar member", err)
		}
	}

	if info.Name == "" {
		info.Name = strings.TrimSuffix(info.Name, ".deb")
	}

	return info, nil
}

func (d *Detector) parseDebControlTar(archiveName string, r io.Reader, info *domain.PackageInfo) error {
	var tarReader *tar.Reader

	if strings.HasSuffix(archiveName, ".gz") {
		gzReader, err := gzip.NewReader(r)
		if err != nil {
			return domain.WrapError(domain.ErrCodePackageInvalid, "failed to decompress gzip control archive", err)
		}
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	} else if strings.HasSuffix(archiveName, ".xz") {
		xzReader, err := xz.NewReader(r)
		if err != nil {
			return domain.WrapError(domain.ErrCodePackageInvalid, "failed to decompress xz control archive", err)
		}
		tarReader = tar.NewReader(xzReader)
	} else {
		// Plain tar or unsupported compressor fallback
		tarReader = tar.NewReader(r)
	}

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			// If format is .xz and we don't have pure Go xz, parse whatever fields we have
			break
		}

		cleanName := strings.TrimPrefix(hdr.Name, "./")
		if cleanName == "control" {
			scanner := bufio.NewScanner(io.LimitReader(tarReader, 256*1024)) // 256KB cap
			var currentKey string
			var descBuilder strings.Builder

			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
					// Continuation line
					if currentKey == "Description" {
						descBuilder.WriteString(" ")
						descBuilder.WriteString(strings.TrimSpace(line))
					}
					continue
				}

				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				currentKey = key

				switch key {
				case "Package":
					info.Name = val
				case "Version":
					info.Version = val
				case "Architecture":
					info.Architecture = normalizeArch(val)
				case "Depends":
					info.Dependencies = splitDependencies(val)
					// Check for GUI indicators in dependencies
					for _, dep := range info.Dependencies {
						depLower := strings.ToLower(dep)
						if strings.Contains(depLower, "libgtk") ||
							strings.Contains(depLower, "libqt") ||
							strings.Contains(depLower, "libx11") ||
							strings.Contains(depLower, "wayland") ||
							strings.Contains(depLower, "libgl") {
							info.PackageType = domain.PackageTypeGUI
						}
					}
				case "Section":
					secLower := strings.ToLower(val)
					if strings.Contains(secLower, "x11") ||
						strings.Contains(secLower, "gnome") ||
						strings.Contains(secLower, "kde") ||
						strings.Contains(secLower, "graphics") ||
						strings.Contains(secLower, "games") {
						info.PackageType = domain.PackageTypeGUI
					}
					info.Metadata["section"] = val
				case "Description":
					descBuilder.WriteString(val)
				}
			}

			if descBuilder.Len() > 0 {
				info.Description = strings.TrimSpace(descBuilder.String())
			}
			return nil
		}
	}

	return nil
}

func normalizeArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "amd64", "x86_64", "x64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "i386", "i686", "x86":
		return "386"
	case "all", "noarch":
		return "all"
	default:
		return arch
	}
}

func splitDependencies(depStr string) []string {
	raw := strings.Split(depStr, ",")
	var result []string
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" {
			result = append(result, r)
		}
	}
	return result
}
