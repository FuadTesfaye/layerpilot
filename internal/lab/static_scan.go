package lab

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/detect"
	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// StaticScanner performs T0 offline package and binary inspection without executing code
type StaticScanner struct {
	detector *detect.Detector
}

// NewStaticScanner creates a new static scanner
func NewStaticScanner() *StaticScanner {
	return &StaticScanner{
		detector: detect.New(),
	}
}

// Scan performs deep static ELF inspection and target GLIBC/library predictions
func (s *StaticScanner) Scan(artifactPath string, targets []TargetDefinition) (*StaticScanResult, error) {
	pkg, err := s.detector.Inspect(artifactPath)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "failed inspecting artifact", err)
	}

	highestGlibc := ""
	if len(pkg.GlibcVersions) > 0 {
		highestGlibc = pkg.GlibcVersions[len(pkg.GlibcVersions)-1]
	}

	res := &StaticScanResult{
		Architecture:         pkg.Architecture,
		ELFInterpreter:       pkg.ELFInterpreter,
		RequiredGlibcVersion: highestGlibc,
		GlibcCompatibility:   make(map[string]bool),
		NeededSharedLibs:     pkg.NeededSharedLibs,
		MissingLibsByTarget:  make(map[string][]string),
		PackageSuggestions:   make(map[string]map[string]string),
		AppProfile: AppProfile{
			Name:         pkg.Name,
			Format:       string(pkg.Format),
			Architecture: pkg.Architecture,
			Framework:    pkg.Framework,
			Entrypoint:   pkg.Name,
		},
	}

	if res.AppProfile.Framework == "" {
		res.AppProfile.Framework = "native"
	}

	if len(pkg.BinaryTargets) > 0 {
		res.AppProfile.Entrypoint = pkg.BinaryTargets[0]
	}

	// Evaluate GLIBC compatibility and library availability per target
	reqGlibc := highestGlibc
	for _, target := range targets {
		res.PackageSuggestions[target.ID] = make(map[string]string)
		var missing []string

		// 1. Compare GLIBC versions
		compat := true
		if reqGlibc != "" && target.GlibcVersion != "" {
			if compareGlibc(reqGlibc, target.GlibcVersion) > 0 {
				compat = false
			}
		}
		res.GlibcCompatibility[target.ID] = compat

		// 2. Predict missing libraries
		for _, lib := range pkg.NeededSharedLibs {
			if pkgName, found := target.KnownPackages[lib]; found {
				// Library is non-standard and requires explicit package installation
				missing = append(missing, lib)
				res.PackageSuggestions[target.ID][lib] = pkgName
			}
		}
		res.MissingLibsByTarget[target.ID] = missing
	}

	return res, nil
}

// EvaluateT0Scenario returns the static scan result as a TargetResult for the target
func (s *StaticScanner) EvaluateT0Scenario(target TargetDefinition, scan *StaticScanResult) TargetResult {
	res := TargetResult{
		TargetID:   target.ID,
		ScenarioID: "static-scan",
		Tier:       TierT0Static,
		Verdict:    VerdictPass,
		Confidence: ConfidenceConfirmed,
	}

	// 1. Check Architecture compatibility
	if scan.Architecture != "" && scan.Architecture != "all" && scan.Architecture != "noarch" {
		// Host is amd64 or arm64
		if scan.Architecture != "amd64" && scan.Architecture != "x86_64" && scan.Architecture != "386" {
			res.Verdict = VerdictFail
			res.Cause = fmt.Sprintf("Architecture mismatch: artifact '%s' is not supported on target '%s'", scan.Architecture, target.ID)
			return res
		}
	}

	// 2. Check GLIBC compatibility
	compat, ok := scan.GlibcCompatibility[target.ID]
	if ok && !compat {
		res.Verdict = VerdictFail
		res.Cause = fmt.Sprintf("Target glibc %s is older than binary requirement (requires GLIBC >= %s)", target.GlibcVersion, scan.RequiredGlibcVersion)
		res.EvidenceRefs = append(res.EvidenceRefs, fmt.Sprintf("Target: %s (GLIBC %s) vs Required: GLIBC %s", target.ID, target.GlibcVersion, scan.RequiredGlibcVersion))
		return res
	}

	// 3. Check predicted missing libraries
	missing := scan.MissingLibsByTarget[target.ID]
	if len(missing) > 0 {
		var pkgs []string
		for _, lib := range missing {
			pkgName := scan.PackageSuggestions[target.ID][lib]
			pkgs = append(pkgs, fmt.Sprintf("%s (%s)", lib, pkgName))
		}
		res.EvidenceRefs = append(res.EvidenceRefs, fmt.Sprintf("Predicted %d libraries requiring host package install: %s", len(missing), strings.Join(pkgs, ", ")))
	}

	return res
}

// compareGlibc returns 1 if v1 > v2, -1 if v1 < v2, and 0 if equal
func compareGlibc(v1, v2 string) int {
	parts1 := parseVersionParts(v1)
	parts2 := parseVersionParts(v2)

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1 := 0
		p2 := 0
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}
		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimPrefix(v, "GLIBC_")
	v = strings.TrimPrefix(v, "v")
	chunks := strings.Split(v, ".")
	var parts []int
	for _, c := range chunks {
		n, _ := strconv.Atoi(c)
		parts = append(parts, n)
	}
	return parts
}

// DumpStaticFiles writes static/elf.json and static/app-profile.json to output directory
func DumpStaticFiles(outDir string, scan *StaticScanResult) error {
	staticDir := filepath.Join(outDir, "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		return err
	}

	elfData := fmt.Sprintf(`{
  "architecture": %q,
  "elf_interpreter": %q,
  "required_glibc_version": %q,
  "needed_shared_libs": %s
}`, scan.Architecture, scan.ELFInterpreter, scan.RequiredGlibcVersion, toJSONString(scan.NeededSharedLibs))

	if err := os.WriteFile(filepath.Join(staticDir, "elf.json"), []byte(elfData), 0644); err != nil {
		return err
	}

	profileData := fmt.Sprintf(`{
  "name": %q,
  "format": %q,
  "architecture": %q,
  "framework": %q,
  "electron_version": %q,
  "chromium_version": %q,
  "entrypoint": %q,
  "native_addons": %s
}`, scan.AppProfile.Name, scan.AppProfile.Format, scan.AppProfile.Architecture, scan.AppProfile.Framework,
		scan.AppProfile.ElectronVersion, scan.AppProfile.ChromiumVersion, scan.AppProfile.Entrypoint,
		toJSONString(scan.AppProfile.NativeAddons))

	return os.WriteFile(filepath.Join(staticDir, "app-profile.json"), []byte(profileData), 0644)
}

func toJSONString(slice []string) string {
	if len(slice) == 0 {
		return "[]"
	}
	var quoted []string
	for _, s := range slice {
		quoted = append(quoted, fmt.Sprintf("%q", s))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
