package lab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundleWriter handles writing all files in compat-report/
type BundleWriter struct {
	redactor *Redactor
}

// NewBundleWriter creates a new bundle writer
func NewBundleWriter(redactor *Redactor) *BundleWriter {
	if redactor == nil {
		redactor = NewRedactor()
	}
	return &BundleWriter{redactor: redactor}
}

// WriteBundle writes all structured artifacts to outDir
func (b *BundleWriter) WriteBundle(outDir string, report *LabReport) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	// 1. Write report.json
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), reportJSON, 0644); err != nil {
		return err
	}

	// 2. Write REPORT.md (Human Summary)
	reportMD := b.generateReportMD(report)
	if err := os.WriteFile(filepath.Join(outDir, "REPORT.md"), []byte(reportMD), 0644); err != nil {
		return err
	}

	// 3. Write AGENT_CONTEXT.md (Strict 7-section AI Agent Brief)
	agentMD := b.generateAgentContextMD(report)
	if err := os.WriteFile(filepath.Join(outDir, "AGENT_CONTEXT.md"), []byte(agentMD), 0644); err != nil {
		return err
	}

	// 4. Dump static scan files if available
	if report.StaticScan != nil {
		_ = DumpStaticFiles(outDir, report.StaticScan)
	}

	return nil
}

func (b *BundleWriter) generateReportMD(report *LabReport) string {
	var sb strings.Builder
	sb.WriteString("# Compatibility Lab Test Report\n\n")
	sb.WriteString(fmt.Sprintf("**Run ID**: `%s`  \n", report.RunID))
	sb.WriteString(fmt.Sprintf("**Timestamp**: %s  \n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("**Artifact**: `%s` (SHA256: `%s`, Size: %d bytes)  \n",
		filepath.Base(report.ArtifactPath), report.ArtifactSHA256[:12], report.ArtifactSize))
	sb.WriteString(fmt.Sprintf("**Execution Runtime**: `%s` (Emulated: %v)  \n\n", report.RuntimeID, report.IsEmulated))

	// Summary tallies
	sb.WriteString("## 1. Summary Matrix\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Scenarios**: %d\n", report.Summary.TotalScenarios))
	sb.WriteString(fmt.Sprintf("- **Passed**: %d\n", report.Summary.Passed))
	sb.WriteString(fmt.Sprintf("- **Failed**: %d\n", report.Summary.Failed))
	sb.WriteString(fmt.Sprintf("- **Inconclusive**: %d\n", report.Summary.Inconclusive))
	sb.WriteString(fmt.Sprintf("- **Not Testable at this Tier**: %d\n\n", report.Summary.NotTestable))

	// Result Table
	sb.WriteString("### Detailed Results\n\n")
	sb.WriteString("| Target | Scenario | Tier | Verdict | Confidence | Cause / Details |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

	for _, r := range report.Results {
		cause := r.Cause
		if cause == "" && r.Verdict == VerdictPass {
			cause = "Scenario passed requirements"
		} else if cause == "" && r.UntestableReason != "" {
			cause = r.UntestableReason
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | **%s** | %s | %s |\n",
			r.TargetID, r.ScenarioID, r.Tier, r.Verdict, r.Confidence, cause))
	}

	// Fidelity limitations
	sb.WriteString("\n## 2. Fidelity & Testing Boundaries (What was NOT tested)\n\n")
	sb.WriteString("The following known failure classes cannot be caught at container tier T1:\n\n")
	for _, lim := range report.FidelityLimitations {
		sb.WriteString(fmt.Sprintf("- %s\n", lim))
	}

	return sb.String()
}

func (b *BundleWriter) generateAgentContextMD(report *LabReport) string {
	var sb strings.Builder

	// Section 1: Goal and constraints
	sb.WriteString("# AGENT BRIEF: Application Compatibility Context & Fix Instructions\n\n")
	sb.WriteString("## 1. Goal and Constraints\n\n")
	sb.WriteString("You are tasked with fixing compatibility failures detected for this application on Linux.\n\n")
	sb.WriteString("### Non-Negotiable Agent Rules:\n")
	sb.WriteString("- Edit only the developer's own source repository, on a new git branch: `compat/fix-<id>`.\n")
	sb.WriteString("- Never automatically push or merge branches.\n")
	sb.WriteString("- Fix one failure at a time (smallest change first), then rebuild and verify with `compat lab verify-fix`.\n")
	sb.WriteString("- Stop at a maximum of 5 iterations or when no progress is made.\n")
	sb.WriteString("- A fix counts only if the failing scenario passes and no other scenario regresses.\n")
	sb.WriteString("- Do NOT add `--no-sandbox` automatically; do NOT disable SELinux/AppArmor; do NOT edit test expectations.\n")
	sb.WriteString("- All logs and code sent to this brief are pre-redacted; maintain strict confidentiality.\n\n")

	// Section 2: App profile
	sb.WriteString("## 2. Application Profile\n\n")
	if report.StaticScan != nil {
		p := report.StaticScan.AppProfile
		sb.WriteString(fmt.Sprintf("- **Name**: %s\n", p.Name))
		sb.WriteString(fmt.Sprintf("- **Package Format**: %s\n", p.Format))
		sb.WriteString(fmt.Sprintf("- **Architecture**: %s\n", p.Architecture))
		sb.WriteString(fmt.Sprintf("- **Framework**: %s\n", p.Framework))
		if p.ElectronVersion != "" {
			sb.WriteString(fmt.Sprintf("- **Electron Version**: %s\n", p.ElectronVersion))
		}
		if p.ChromiumVersion != "" {
			sb.WriteString(fmt.Sprintf("- **Chromium Version**: %s\n", p.ChromiumVersion))
		}
		if report.StaticScan.RequiredGlibcVersion != "" {
			sb.WriteString(fmt.Sprintf("- **Required GLIBC Version**: %s\n", report.StaticScan.RequiredGlibcVersion))
		}
		if report.StaticScan.ELFInterpreter != "" {
			sb.WriteString(fmt.Sprintf("- **ELF Interpreter**: %s\n", report.StaticScan.ELFInterpreter))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("Static scan profile not available.\n\n")
	}

	// Section 3: Result matrix with tiers (failures first)
	sb.WriteString("## 3. Results Matrix (Failures Ranked First)\n\n")
	sb.WriteString("| Target | Scenario | Tier | Verdict | Confidence | Summary |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")

	// Print failures first
	for _, r := range report.Results {
		if r.Verdict == VerdictFail {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | **FAIL** | %s | %s |\n",
				r.TargetID, r.ScenarioID, r.Tier, r.Confidence, r.Cause))
		}
	}
	// Print non-failures
	for _, r := range report.Results {
		if r.Verdict != VerdictFail {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s | %s | %s |\n",
				r.TargetID, r.ScenarioID, r.Tier, r.Verdict, r.Confidence, r.Cause))
		}
	}
	sb.WriteString("\n")

	// Section 4: For each failure: redacted excerpt, pattern, cause, evidence for and against
	sb.WriteString("## 4. Failure Investigations & Evidence\n\n")
	hasFailures := false
	for _, r := range report.Results {
		if r.Verdict == VerdictFail {
			hasFailures = true
			sb.WriteString(fmt.Sprintf("### Failure: `%s` on `%s`\n\n", r.ScenarioID, r.TargetID))
			sb.WriteString(fmt.Sprintf("- **Tier**: %s\n", r.Tier))
			if r.MatchedPatternID != "" {
				sb.WriteString(fmt.Sprintf("- **Matched Pattern ID**: `%s`\n", r.MatchedPatternID))
			}
			sb.WriteString(fmt.Sprintf("- **Cause**: %s\n", r.Cause))
			sb.WriteString(fmt.Sprintf("- **Confidence**: %s\n", r.Confidence))
			if len(r.EvidenceRefs) > 0 {
				sb.WriteString("- **Evidence Supporting**:\n")
				for _, ev := range r.EvidenceRefs {
					sb.WriteString(fmt.Sprintf("  - %s\n", ev))
				}
			}
			sb.WriteString("- **Evidence that would Refute**:\n")
			sb.WriteString("  - If re-running the exact same scenario with the candidate fix passes without regressions.\n\n")
		}
	}
	if !hasFailures {
		sb.WriteString("No fatal failures detected in the executed test scenarios.\n\n")
	}

	// Section 5: Candidate fixes in two lists: DEV-SIDE and USER-SIDE
	sb.WriteString("## 5. Candidate Fixes\n\n")
	sb.WriteString("### A. Developer-Side Remedies (Source / Build Changes):\n")
	sb.WriteString("1. **GLIBC Incompatibilities**: Build application packages on an older Linux baseline (e.g. Ubuntu 20.04 or Debian 11/12) to ensure backward libc compatibility across all distros.\n")
	sb.WriteString("2. **Missing Libraries**: Statically bundle required `.so` shared libraries in AppImage `/usr/lib` or add them as package dependencies in `.deb`/`.rpm` control files.\n")
	sb.WriteString("3. **Display Server Timing (Electron/Chromium)**: Pass `--ozone-platform=x11` or `--ozone-platform-hint=auto` before process initialization rather than calling in-JS switch appending too late in `main.js`.\n")
	sb.WriteString("4. **FUSE Fallback**: Package with a modern Type-2 AppImage runtime that supports `--appimage-extract-and-run` out of the box when FUSE is unavailable.\n\n")

	sb.WriteString("### B. User-Side Remedies (compat Runtime Wrappers):\n")
	sb.WriteString("1. `compat fix <app-id>` automatically rewrites the launcher wrapper with verified `--ozone-platform=x11` or `--appimage-extract-and-run` flags.\n")
	sb.WriteString("2. Foreign package container fallback: `compat install --engine=podman <pkg>` isolates foreign packages in managed rootless containers.\n\n")

	// Section 6: "Not testable here"
	sb.WriteString("## 6. What Was NOT Testable at This Tier\n\n")
	sb.WriteString("The following cannot be caught in container tier T1:\n")
	for _, lim := range report.FidelityLimitations {
		sb.WriteString(fmt.Sprintf("- %s\n", lim))
	}
	sb.WriteString("\n*Recommended next step for untested classes: Execute a T2 VM test (Lima/QEMU) or T3 bare-metal test.*\n\n")

	// Section 7: Exact command to rerun
	sb.WriteString("## 7. Verification Command\n\n")
	sb.WriteString("After applying changes to the application source and rebuilding the artifact, run:\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("compat lab verify-fix %s --scenario <scenario-id>\n", filepath.Join(".", filepath.Base(report.ArtifactPath))))
	sb.WriteString("```\n")

	return sb.String()
}
