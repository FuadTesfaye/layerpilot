package detect

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// Detector provides package inspection without executing untrusted package scripts
type Detector struct{}

// New creates a new Detector instance
func New() *Detector {
	return &Detector{}
}

// Inspect identifies file format, architecture, and package metadata safely
func (d *Detector) Inspect(pkgPath string) (*domain.PackageInfo, error) {
	fileInfo, err := os.Stat(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.NewError(
				domain.ErrCodePackageNotFound,
				"package file does not exist: "+pkgPath,
				"Verify the file path and ensure it is accessible",
				"Cannot inspect nonexistent package",
			)
		}
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot stat package file", err)
	}

	if fileInfo.IsDir() {
		return nil, domain.NewError(
			domain.ErrCodePackageInvalid,
			"specified path is a directory, expected a package file",
			"Provide the path to a .deb, .rpm, or .AppImage file",
			"Directories cannot be installed directly",
		)
	}

	// Calculate SHA256
	f, err := os.Open(pkgPath)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot open package file", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "failed to compute package sha256 checksum", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Re-open for header inspection
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek package file", err)
	}

	magic := make([]byte, 16)
	n, err := f.Read(magic)
	if err != nil && err != io.EOF {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot read package header", err)
	}
	if n < 4 {
		return nil, domain.NewError(
			domain.ErrCodePackageInvalid,
			"package file too short to be a valid installer",
			"Ensure the file is not empty or corrupted",
			"Inspection refused",
		)
	}

	info := &domain.PackageInfo{
		Path:        pkgPath,
		Name:        filepath.Base(pkgPath),
		SizeBytes:   fileInfo.Size(),
		SHA256:      checksum,
		PackageType: domain.PackageTypeCLI,
		Metadata:    make(map[string]string),
	}

	// Check for Debian package: ar header "!<arch>\n"
	if n >= 8 && string(magic[:8]) == "!<arch>\n" {
		return d.inspectDeb(f, info)
	}

	// Check for RPM package: lead magic 0xed 0xab 0xee 0xdb
	if n >= 4 && magic[0] == 0xed && magic[1] == 0xab && magic[2] == 0xee && magic[3] == 0xdb {
		return d.inspectRPM(f, info)
	}

	// Check for AppImage / ELF: 0x7f 'E' 'L' 'F'
	if n >= 4 && magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F' {
		return d.inspectAppImage(f, info, magic)
	}

	// Check for Windows PE: MZ
	if n >= 2 && magic[0] == 'M' && magic[1] == 'Z' {
		return d.inspectPE(f, info)
	}

	// Check for Mach-O
	if (magic[0] == 0xfe && magic[1] == 0xed && magic[2] == 0xfa) ||
		(magic[0] == 0xcf && magic[1] == 0xfa && magic[2] == 0xed) {
		info.Format = domain.FormatMacOSApp
		info.Description = "macOS Mach-O executable"
		return info, nil
	}

	// Check for Arch package (.pkg.tar.zst / .pkg.tar.xz)
	if isArchPackage(pkgPath, magic) {
		return d.inspectArchPackage(f, info, magic)
	}

	// Check for Flatpak bundle / .flatpakref
	if isFlatpak(pkgPath, magic) {
		return d.inspectFlatpak(f, info)
	}

	// Check for Snap package
	if isSnap(pkgPath, magic) {
		return d.inspectSnap(f, info)
	}

	// Check for Self-extracting .run / .sh scripts
	if isSelfExtractingScript(pkgPath, magic) {
		return d.inspectSelfExtract(f, info)
	}

	// Check for Tarballs (.tar.gz, .tar.xz, .tar.zst, .zip)
	if isZip(magic) || isGzip(magic) || isXZ(magic) || isZstd(magic) || isBzip2(magic) || strings.HasSuffix(strings.ToLower(pkgPath), ".tar") {
		return d.inspectTarball(f, info, magic)
	}

	info.Format = domain.FormatUnknown
	return info, domain.NewError(
		domain.ErrCodeFormatUnsupported,
		"unrecognized package format",
		"Supported formats are AppImage, Debian (.deb), RedHat (.rpm), Arch (.pkg.tar.zst), Tarball, Flatpak, and Snap",
		"Package cannot be scheduled for execution",
	)
}
