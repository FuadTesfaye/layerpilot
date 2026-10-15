package detect

import (
	"debug/elf"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

var glibcVersionRegex = regexp.MustCompile(`^GLIBC_(\d+)\.(\d+)(?:\.(\d+))?$`)

// ScanStaticELF performs read-only static analysis on an ELF binary without execution
func ScanStaticELF(filePath string, info *domain.PackageInfo) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return ScanStaticELFReader(f, info)
}

// ScanStaticELFReader performs read-only static analysis on an io.ReaderAt ELF stream
func ScanStaticELFReader(r io.ReaderAt, info *domain.PackageInfo) error {
	ef, err := elf.NewFile(r)
	if err != nil {
		return err
	}
	defer ef.Close()

	// 1. Architecture
	switch ef.Machine {
	case elf.EM_X86_64:
		info.Architecture = "amd64"
	case elf.EM_AARCH64:
		info.Architecture = "arm64"
	case elf.EM_386:
		info.Architecture = "386"
	case elf.EM_ARM:
		info.Architecture = "arm"
	case elf.EM_RISCV:
		info.Architecture = "riscv64"
	}

	// 2. Interpreter (.interp section)
	if sec := ef.Section(".interp"); sec != nil {
		if data, err := sec.Data(); err == nil {
			info.ELFInterpreter = strings.TrimSpace(strings.TrimRight(string(data), "\x00"))
		}
	}

	// 3. DT_NEEDED shared libraries
	if libs, err := ef.ImportedLibraries(); err == nil && len(libs) > 0 {
		info.NeededSharedLibs = libs
	}

	// 4. Imported Symbols & GLIBC symbol versioning
	glibcVersionsMap := make(map[string]bool)
	if syms, err := ef.ImportedSymbols(); err == nil {
		for _, s := range syms {
			if strings.HasPrefix(s.Version, "GLIBC_") {
				glibcVersionsMap[s.Version] = true
			}
		}
	}

	// Also check dynamic symbol table if available
	if dynSyms, err := ef.DynamicSymbols(); err == nil {
		for _, s := range dynSyms {
			if strings.HasPrefix(s.Version, "GLIBC_") {
				glibcVersionsMap[s.Version] = true
			}
		}
	}

	if len(glibcVersionsMap) > 0 {
		var glibcList []string
		for v := range glibcVersionsMap {
			glibcList = append(glibcList, v)
		}
		sort.Slice(glibcList, func(i, j int) bool {
			return compareGlibcVersions(glibcList[i], glibcList[j]) < 0
		})
		info.GlibcVersions = glibcList
	}

	// 5. Detect framework signatures from needed shared libraries
	framework := "generic"
	for _, lib := range info.NeededSharedLibs {
		lower := strings.ToLower(lib)
		if strings.Contains(lower, "libnode") || strings.Contains(lower, "libffmpeg") || strings.Contains(lower, "libelectron") {
			framework = "electron"
			break
		}
		if strings.Contains(lower, "libqt") {
			framework = "qt"
		} else if strings.Contains(lower, "libgtk") || strings.Contains(lower, "libgdk") {
			if framework != "qt" {
				framework = "gtk"
			}
		}
	}

	if info.Framework == "" || info.Framework == "generic" {
		info.Framework = framework
	}

	return nil
}

func compareGlibcVersions(a, b string) int {
	mA := glibcVersionRegex.FindStringSubmatch(a)
	mB := glibcVersionRegex.FindStringSubmatch(b)
	if len(mA) < 3 || len(mB) < 3 {
		return strings.Compare(a, b)
	}

	majorA, _ := strconv.Atoi(mA[1])
	majorB, _ := strconv.Atoi(mB[1])
	if majorA != majorB {
		return majorA - majorB
	}

	minorA, _ := strconv.Atoi(mA[2])
	minorB, _ := strconv.Atoi(mB[2])
	if minorA != minorB {
		return minorA - minorB
	}

	patchA, patchB := 0, 0
	if len(mA) > 3 && mA[3] != "" {
		patchA, _ = strconv.Atoi(mA[3])
	}
	if len(mB) > 3 && mB[3] != "" {
		patchB, _ = strconv.Atoi(mB[3])
	}
	return patchA - patchB
}
