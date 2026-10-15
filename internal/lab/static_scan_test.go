package lab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareGlibc(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"GLIBC_2.38", "2.36", 1},
		{"2.31", "2.36", -1},
		{"2.41", "2.41", 0},
		{"2.4", "2.40", -1},
		{"2.38.1", "2.38", 1},
	}

	for _, tc := range tests {
		got := compareGlibc(tc.v1, tc.v2)
		if got != tc.expected {
			t.Errorf("compareGlibc(%s, %s): expected %d, got %d", tc.v1, tc.v2, tc.expected, got)
		}
	}
}

func TestStaticScanAndBundleGeneration(t *testing.T) {
	targets := DefaultTargets()

	scanResult := &StaticScanResult{
		Architecture:         "amd64",
		ELFInterpreter:       "/lib64/ld-linux-x86-64.so.2",
		RequiredGlibcVersion: "2.38",
		GlibcCompatibility:   make(map[string]bool),
		NeededSharedLibs:     []string{"libasound.so.2", "libgtk-3.so.0"},
		MissingLibsByTarget:  make(map[string][]string),
		PackageSuggestions:   make(map[string]map[string]string),
		AppProfile: AppProfile{
			Name:         "test-app",
			Format:       "appimage",
			Architecture: "amd64",
			Framework:    "electron",
		},
	}

	scanner := NewStaticScanner()

	// Evaluate per target
	var results []TargetResult
	for _, target := range targets {
		// Calculate compatibility
		compat := compareGlibc(scanResult.RequiredGlibcVersion, target.GlibcVersion) <= 0
		scanResult.GlibcCompatibility[target.ID] = compat
		scanResult.PackageSuggestions[target.ID] = make(map[string]string)

		for _, lib := range scanResult.NeededSharedLibs {
			if pkg, found := target.KnownPackages[lib]; found {
				scanResult.MissingLibsByTarget[target.ID] = append(scanResult.MissingLibsByTarget[target.ID], lib)
				scanResult.PackageSuggestions[target.ID][lib] = pkg
			}
		}

		res := scanner.EvaluateT0Scenario(target, scanResult)
		results = append(results, res)
	}

	// Verify Debian 12 (glibc 2.36) fails against required 2.38
	foundDebianFail := false
	for _, r := range results {
		if r.TargetID == "debian:12" {
			if r.Verdict != VerdictFail {
				t.Errorf("expected debian:12 to fail due to glibc 2.36 < 2.38, got %s", r.Verdict)
			}
			foundDebianFail = true
		}
		if r.TargetID == "fedora:42" {
			if r.Verdict != VerdictPass {
				t.Errorf("expected fedora:42 (glibc 2.41) to pass, got %s", r.Verdict)
			}
		}
	}
	if !foundDebianFail {
		t.Errorf("did not find debian:12 in evaluated results")
	}

	// Write Bundle
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "compat-report")

	report := &LabReport{
		RunID:               "run-test-1234",
		Timestamp:           time.Now().UTC(),
		ArtifactPath:        "/tmp/test-app.AppImage",
		ArtifactSHA256:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		ArtifactSize:        1024 * 1024,
		RuntimeID:           "podman",
		IsEmulated:          false,
		Targets:             []string{"fedora:42", "debian:12"},
		Scenarios:           []string{"static-scan"},
		Results:             results,
		StaticScan:          scanResult,
		Summary: LabSummary{
			TotalScenarios: len(results),
			Passed:         1,
			Failed:         1,
		},
		FidelityLimitations: T1FidelityLimitations,
	}

	writer := NewBundleWriter(nil)
	if err := writer.WriteBundle(outDir, report); err != nil {
		t.Fatalf("failed writing bundle: %v", err)
	}

	// Verify files created
	for _, name := range []string{"report.json", "REPORT.md", "AGENT_CONTEXT.md", "static/elf.json", "static/app-profile.json"} {
		p := filepath.Join(outDir, name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s to exist: %v", name, err)
		}
	}

	// Verify AGENT_CONTEXT.md contains strict sections 1-7 in order
	agentData, err := os.ReadFile(filepath.Join(outDir, "AGENT_CONTEXT.md"))
	if err != nil {
		t.Fatalf("failed reading AGENT_CONTEXT.md: %v", err)
	}
	agentStr := string(agentData)

	sections := []string{
		"## 1. Goal and Constraints",
		"## 2. Application Profile",
		"## 3. Results Matrix",
		"## 4. Failure Investigations",
		"## 5. Candidate Fixes",
		"## 6. What Was NOT Testable at This Tier",
		"## 7. Verification Command",
	}

	lastIdx := -1
	for _, sec := range sections {
		idx := strings.Index(agentStr, sec)
		if idx == -1 {
			t.Errorf("AGENT_CONTEXT.md missing section: %s", sec)
		}
		if idx < lastIdx {
			t.Errorf("AGENT_CONTEXT.md section %s is out of order", sec)
		}
		lastIdx = idx
	}
}
