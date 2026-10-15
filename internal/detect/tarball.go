package detect

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func (d *Detector) inspectTarball(f *os.File, info *domain.PackageInfo, magic []byte) (*domain.PackageInfo, error) {
	info.Format = domain.FormatTarball
	base := filepath.Base(info.Path)
	info.Name = stripArchiveExtension(base)

	if isZip(magic) {
		return d.inspectZip(f, info)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek in tarball", err)
	}

	var tr *tar.Reader
	if isGzip(magic) {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return info, nil
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else if isXZ(magic) {
		xr, err := xz.NewReader(f)
		if err != nil {
			return info, nil
		}
		tr = tar.NewReader(xr)
	} else if isZstd(magic) {
		zr, err := zstd.NewReader(f)
		if err != nil {
			return info, nil
		}
		defer zr.Close()
		tr = tar.NewReader(zr)
	} else if isBzip2(magic) {
		br := bzip2.NewReader(f)
		tr = tar.NewReader(br)
	} else {
		tr = tar.NewReader(f)
	}

	entriesRead := 0
	for entriesRead < 500 {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		entriesRead++

		clean := strings.TrimPrefix(hdr.Name, "./")
		if strings.HasSuffix(clean, ".desktop") {
			info.PackageType = domain.PackageTypeGUI
		}

		// Detect executable targets in tarball
		if (hdr.FileInfo().Mode()&0111 != 0 || strings.HasPrefix(clean, "bin/") || strings.HasSuffix(clean, ".sh")) && !hdr.FileInfo().IsDir() {
			if len(info.BinaryTargets) < 10 {
				info.BinaryTargets = append(info.BinaryTargets, clean)
			}
		}
	}

	return info, nil
}

func (d *Detector) inspectZip(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	stat, err := f.Stat()
	if err != nil {
		return info, nil
	}

	zr, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return info, nil
	}

	for _, file := range zr.File {
		clean := strings.TrimPrefix(file.Name, "./")
		if strings.HasSuffix(clean, ".desktop") {
			info.PackageType = domain.PackageTypeGUI
		}
		if (file.Mode()&0111 != 0 || strings.HasPrefix(clean, "bin/")) && !file.FileInfo().IsDir() {
			if len(info.BinaryTargets) < 10 {
				info.BinaryTargets = append(info.BinaryTargets, clean)
			}
		}
	}

	return info, nil
}

func isZip(magic []byte) bool {
	return len(magic) >= 4 && magic[0] == 'P' && magic[1] == 'K' && magic[2] == 0x03 && magic[3] == 0x04
}

func isGzip(magic []byte) bool {
	return len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b
}

func isXZ(magic []byte) bool {
	return len(magic) >= 6 && magic[0] == 0xfd && magic[1] == '7' && magic[2] == 'z' && magic[3] == 'X' && magic[4] == 'Z' && magic[5] == 0x00
}

func isZstd(magic []byte) bool {
	return len(magic) >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd
}

func isBzip2(magic []byte) bool {
	return len(magic) >= 3 && magic[0] == 'B' && magic[1] == 'Z' && magic[2] == 'h'
}

func stripArchiveExtension(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.zst", ".tar.bz2", ".tgz", ".txz", ".zip", ".tar"} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}
