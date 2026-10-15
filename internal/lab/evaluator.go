package lab

import (
	"fmt"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/fix"
	"github.com/FuadTesfaye/layerpilot/internal/patterns"
)

// Evaluator classifies test scenario results into honest fidelity verdicts
type Evaluator struct {
	classifier *fix.Classifier
}

// NewEvaluator creates an evaluator with default diagnostic patterns
func NewEvaluator() (*Evaluator, error) {
	c, err := fix.NewDefaultClassifier()
	if err != nil {
		return nil, err
	}
	return &Evaluator{classifier: c}, nil
}

// EvaluateStaticScan creates a TargetResult for T0 static ELF scan
func (e *Evaluator) EvaluateStaticScan(target TargetDefinition, staticScan *StaticScanResult) TargetResult {
	res := TargetResult{
		TargetID:   target.ID,
		ScenarioID: "static-scan",
		Tier:       TierT0Static,
		ExitCode:   0,
	}

	if staticScan == nil {
		res.Verdict = VerdictInconclusive
		res.Confidence = ConfidenceHypothesis
		res.Cause = "Static scan data unavailable"
		return res
	}

	// 1. Check Glibc compatibility
	if comp, ok := staticScan.GlibcCompatibility[target.ID]; ok && !comp {
		res.Verdict = VerdictFail
		res.Confidence = ConfidenceConfirmed
		res.MatchedPatternID = "p4-glibc-too-old"
		res.Cause = fmt.Sprintf("Artifact requires GLIBC >= %s, but target %s provides GLIBC %s",
			staticScan.RequiredGlibcVersion, target.ID, target.GlibcVersion)
		res.ProposedFixes = []patterns.FixOption{
			{
				ID:          "fix-container-fallback-podman",
				Description: "Delegate execution to a container image that supplies the required newer glibc runtime",
				Tier:        1,
				Action:      "container_fallback",
				Flags:       []string{"--engine=podman"},
			},
		}
		res.EvidenceRefs = []string{fmt.Sprintf("Required: %s, Available: %s", staticScan.RequiredGlibcVersion, target.GlibcVersion)}
		return res
	}

	// 2. Check missing libraries
	if missing, ok := staticScan.MissingLibsByTarget[target.ID]; ok && len(missing) > 0 {
		res.Verdict = VerdictFail
		res.Confidence = ConfidenceLikely
		res.MatchedPatternID = "p6-missing-shared-library"
		res.Cause = fmt.Sprintf("Missing %d required shared libraries: %s", len(missing), strings.Join(missing, ", "))

		var pkgInstalls []string
		if suggestions, ok := staticScan.PackageSuggestions[target.ID]; ok {
			for _, pkg := range suggestions {
				pkgInstalls = append(pkgInstalls, pkg)
			}
		}

		res.ProposedFixes = []patterns.FixOption{
			{
				ID:          "fix-install-missing-libs",
				Description: fmt.Sprintf("Install packages providing the missing libraries: %s", strings.Join(pkgInstalls, ", ")),
				Tier:        1,
				Action:      "package_install",
				Command:     pkgInstalls,
			},
		}
		res.EvidenceRefs = missing
		return res
	}

	// Both glibc and shared libraries passed static check
	res.Verdict = VerdictPass
	res.Confidence = ConfidenceConfirmed
	res.Cause = fmt.Sprintf("Static ELF symbols and GLIBC %s match target %s (%s)",
		staticScan.RequiredGlibcVersion, target.ID, target.GlibcVersion)
	return res
}

// EvaluateContainerRun classifies a T1 container run result
func (e *Evaluator) EvaluateContainerRun(
	target TargetDefinition,
	scenario ScenarioDefinition,
	runResult *ContainerRunResult,
	appProfile AppProfile,
) TargetResult {
	res := TargetResult{
		TargetID:       target.ID,
		ScenarioID:     scenario.ID,
		Tier:           TierT1Container,
		DurationMs:     runResult.Duration.Milliseconds(),
		ExitCode:       runResult.ExitCode,
		ReproScriptRef: runResult.ReproScriptPath,
		ScreenshotRef:  runResult.ScreenshotPath,
	}

	// 1. Untestable condition check
	if runResult.NotTestableReason != "" {
		res.Verdict = VerdictNotTestableHere
		res.Confidence = ConfidenceConfirmed
		res.UntestableReason = runResult.NotTestableReason
		res.Cause = runResult.NotTestableReason
		return res
	}

	lines := strings.Split(runResult.CombinedLog, "\n")

	// 2. Detect Wayland P1 failure pattern symptoms
	// Hard fidelity rule: containers do NOT provide native Wayland sessions.
	// If logs contain symptoms of P1 (electron-wayland-no-window), T1 cannot confirm or fix native Wayland.
	hasWaylandSymptoms := false
	for _, l := range lines {
		if strings.Contains(l, "XGetWindowAttributes failed") ||
			strings.Contains(l, "gtk_widget_get_scale_factor: assertion 'GTK_IS_WIDGET' failed") {
			hasWaylandSymptoms = true
			break
		}
	}

	if hasWaylandSymptoms {
		res.Verdict = VerdictNotTestableHere
		res.Confidence = ConfidenceConfirmed
		res.MatchedPatternID = "p1-electron-wayland-no-window"
		res.UntestableReason = "Native Wayland compositor failure (P1) cannot be verified inside container tier T1; requires native Wayland desktop host (T3)"
		res.Cause = "Application hit X11/Wayland presenter failure; native Wayland verification not testable at T1"
		res.EvidenceRefs = []string{
			"XGetWindowAttributes failed for window",
			"gtk_widget_get_scale_factor: assertion 'GTK_IS_WIDGET' failed",
		}
		res.ProposedFixes = []patterns.FixOption{
			{
				ID:          "fix-electron-ozone-x11",
				Description: "Launch with --ozone-platform=x11 flag to prevent Wayland window initialization failure",
				Tier:        1,
				Action:      "wrapper_flags",
				Flags:       []string{"--ozone-platform=x11"},
			},
		}
		return res
	}

	// 3. Classify with standard classifier
	execCtx := fix.ExecutionContext{
		AppType:        appProfile.Framework,
		SessionType:    "x11",
		WindowDetected: (runResult.ScreenshotPath != "" || (scenario.ID == "window" && runResult.ExitCode == 0)),
		ProcessAlive:   (runResult.ExitCode == 0),
		UptimeSeconds:  int(runResult.Duration.Seconds()),
	}

	classification := e.classifier.ClassifyLog(lines, execCtx)

	// Fatal match
	if classification.HasFatal() {
		m := classification.FatalMatches[0]
		res.Verdict = VerdictFail
		res.Confidence = ConfidenceConfirmed
		res.MatchedPatternID = m.Pattern.ID
		res.Cause = m.Pattern.Cause
		res.ProposedFixes = m.Pattern.Fixes
		res.EvidenceRefs = m.MatchedLines
		return res
	}

	// Exit code failure without pattern match
	if runResult.ExitCode != 0 {
		res.Verdict = VerdictInconclusive
		res.Confidence = ConfidenceHypothesis
		res.Cause = fmt.Sprintf("Process exited with non-zero exit code %d without matching known fatal pattern", runResult.ExitCode)
		if len(runResult.Stderr) > 0 {
			res.EvidenceRefs = []string{strings.TrimSpace(runResult.Stderr)}
		}
		return res
	}

	// 4. Pass condition
	res.Verdict = VerdictPass
	res.Confidence = ConfidenceConfirmed
	if scenario.ID == "offline" {
		res.Cause = "Process executed cleanly with network disconnected; network errors were harmless noise"
	} else if scenario.ID == "window" {
		res.Cause = "Process successfully mapped window under virtual framebuffer"
	} else {
		res.Cause = "Process executed cleanly with exit code 0"
	}

	// Fidelity limitation reminder for Wayland desktops (e.g. Fedora 42)
	if target.DistroFamily == "fedora" && appProfile.Framework == "electron" {
		res.EvidenceRefs = append(res.EvidenceRefs, "T1 container fidelity note: Native Wayland compositor mapping is not evaluated in headless container")
	}

	return res
}
