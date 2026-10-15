package lab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func TestBuildScenarioCommand(t *testing.T) {
	cases := []struct {
		name         string
		artifact     string
		scenarioID   string
		format       domain.PackageFormat
		expectedArg0 string
	}{
		{
			name:         "AppImage smoke",
			artifact:     "myapp.AppImage",
			scenarioID:   "smoke",
			format:       domain.FormatAppImage,
			expectedArg0: "/artifact/myapp.AppImage",
		},
		{
			name:         "DEB install-clean",
			artifact:     "pkg.deb",
			scenarioID:   "install-clean",
			format:       domain.FormatDeb,
			expectedArg0: "dpkg",
		},
		{
			name:         "RPM install-clean",
			artifact:     "pkg.rpm",
			scenarioID:   "install-clean",
			format:       domain.FormatRPM,
			expectedArg0: "dnf",
		},
		{
			name:         "Native binary smoke",
			artifact:     "mybin",
			scenarioID:   "smoke",
			format:       domain.FormatUnknown,
			expectedArg0: "/artifact/mybin",
		},
		{
			name:         "Window scenario with xvfb",
			artifact:     "myapp.AppImage",
			scenarioID:   "window",
			format:       domain.FormatAppImage,
			expectedArg0: "xvfb-run",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := ScenarioDefinition{ID: tc.scenarioID}
			cmd := BuildScenarioCommand(tc.artifact, sc, tc.format)
			if len(cmd) == 0 {
				t.Fatalf("expected non-empty command")
			}
			if cmd[0] != tc.expectedArg0 {
				t.Errorf("expected arg0 %q, got %q", tc.expectedArg0, cmd[0])
			}
		})
	}
}

func TestMockContainerRuntimeAndReproScript(t *testing.T) {
	tmpDir := t.TempDir()
	mockRT := NewMockContainerRuntime()

	target, _ := FindTarget("fedora:42")
	scenario := ScenarioDefinition{
		ID:         "smoke",
		TimeoutSec: 10,
	}

	runOpts := ContainerRunOptions{
		Target:           *target,
		Scenario:         scenario,
		ArtifactHostPath: "/tmp/fake-artifact.AppImage",
		ArtifactBase:     "fake-artifact.AppImage",
		OutputDir:        tmpDir,
		Network:          false,
		Command:          []string{"/artifact/fake-artifact.AppImage", "--appimage-extract-and-run"},
	}

	res, err := mockRT.Run(context.Background(), runOpts)
	if err != nil {
		t.Fatalf("mock run failed: %v", err)
	}

	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}

	// Verify repro script was generated
	reproScript := res.ReproScriptPath
	if reproScript == "" {
		t.Fatalf("expected repro script to be generated")
	}

	scriptContent, err := os.ReadFile(reproScript)
	if err != nil {
		t.Fatalf("failed reading repro script: %v", err)
	}

	scriptStr := string(scriptContent)
	if !strings.Contains(scriptStr, "mock run --rm") {
		t.Errorf("expected repro script to include runtime invocation")
	}
	if !strings.Contains(scriptStr, "--security-opt=no-new-privileges") {
		t.Errorf("expected repro script to include security options")
	}
	if !strings.Contains(scriptStr, "--network=none") {
		t.Errorf("expected repro script to default to network none")
	}
}

func TestEvaluatorNuzzWaylandGoldenLog(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("failed creating evaluator: %v", err)
	}

	target, _ := FindTarget("fedora:42")
	scenario := ScenarioDefinition{ID: "smoke"}

	// Real Nuzz log on Fedora Wayland
	nuzzGoldenLog := `
[8940:1014/195342.383111:ERROR:x11_software_bitmap_presenter.cc(139)] XGetWindowAttributes failed for window 1
(nuzz:8940): Gtk-CRITICAL **: 19:53:42.384: gtk_widget_get_scale_factor: assertion 'GTK_IS_WIDGET (widget)' failed
[8940:1014/195342.450123:INFO:electron_updater.cc(42)] HttpError: 404 Not Found at latest-linux.yml
`

	runRes := &ContainerRunResult{
		TargetID:    "fedora:42",
		ScenarioID:  "smoke",
		ExitCode:    0, // Process did not crash, but window failed to map
		CombinedLog: nuzzGoldenLog,
		Duration:    5 * time.Second,
	}

	appProf := AppProfile{
		Name:      "Nuzz",
		Framework: "electron",
	}

	res := eval.EvaluateContainerRun(*target, scenario, runRes, appProf)

	// Hard fidelity rule check:
	// A container CANNOT reproduce or confirm native Wayland blank window.
	// Must NEVER report "pass" for this failure at T1!
	if res.Verdict == VerdictPass {
		t.Fatalf("VIOLATION OF HARD RULE 2: T1 container runner reported 'pass' on real Nuzz Wayland failure log!")
	}

	if res.Verdict != VerdictNotTestableHere {
		t.Errorf("expected verdict 'not-testable-at-this-tier', got %s", res.Verdict)
	}

	if res.MatchedPatternID != "p1-electron-wayland-no-window" {
		t.Errorf("expected matched pattern p1-electron-wayland-no-window, got %s", res.MatchedPatternID)
	}

	if !strings.Contains(res.UntestableReason, "Native Wayland compositor failure") {
		t.Errorf("expected untestable reason mentioning Wayland compositor, got %s", res.UntestableReason)
	}

	// Verify proposed fix includes --ozone-platform=x11
	if len(res.ProposedFixes) == 0 || !strings.Contains(res.ProposedFixes[0].Flags[0], "--ozone-platform=x11") {
		t.Errorf("expected proposed fix to suggest --ozone-platform=x11")
	}
}

func TestEvaluatorOfflineResiliency(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("failed creating evaluator: %v", err)
	}

	target, _ := FindTarget("ubuntu:24.04")
	scenario := ScenarioDefinition{ID: "offline"}

	// Offline log with updater noise but clean exit
	offlineLog := `
[info] Starting application in offline mode
[info] Checking for updates...
[electron-updater] HttpError: 404 Not Found at latest-linux.yml
[warn] Update server unreachable, continuing offline execution
[info] Application ready
`
	runRes := &ContainerRunResult{
		TargetID:    "ubuntu:24.04",
		ScenarioID:  "offline",
		ExitCode:    0,
		CombinedLog: offlineLog,
		Duration:    3 * time.Second,
	}

	appProf := AppProfile{Framework: "electron"}
	res := eval.EvaluateContainerRun(*target, scenario, runRes, appProf)

	if res.Verdict != VerdictPass {
		t.Errorf("expected offline test with updater noise to pass, got %s", res.Verdict)
	}
}

func TestRunLabSuiteAndVerifyFix(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a dummy ELF artifact
	dummyArtifact := filepath.Join(tmpDir, "testapp.bin")
	if err := os.WriteFile(dummyArtifact, []byte("\x7fELF\x02\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0755); err != nil {
		t.Fatalf("failed creating dummy artifact: %v", err)
	}

	target, _ := FindTarget("fedora:42")
	mockRT := NewMockContainerRuntime()

	bundleDir := filepath.Join(tmpDir, "report-bundle")
	report, err := RunLabSuite(context.Background(), RunLabOptions{
		ArtifactPath: dummyArtifact,
		Targets:      []TargetDefinition{*target},
		Scenarios: []ScenarioDefinition{
			{ID: "static-scan", RequiredTier: TierT0Static},
			{ID: "smoke", RequiredTier: TierT1Container},
		},
		OutputDir: bundleDir,
		Runtime:   mockRT,
	})

	if err != nil {
		t.Fatalf("RunLabSuite failed: %v", err)
	}

	if report.Summary.TotalScenarios != 2 {
		t.Errorf("expected 2 total scenarios, got %d", report.Summary.TotalScenarios)
	}

	// Verify report.json, REPORT.md, AGENT_CONTEXT.md exist
	for _, fname := range []string{"report.json", "REPORT.md", "AGENT_CONTEXT.md"} {
		fpath := filepath.Join(bundleDir, fname)
		if _, statErr := os.Stat(fpath); statErr != nil {
			t.Errorf("expected file %s to exist in bundle", fname)
		}
	}

	// Test verify-fix on this report
	verifyRes, err := VerifyFix(context.Background(), VerificationRequest{
		OriginalReportPath: bundleDir,
		NewArtifactPath:    dummyArtifact,
		TargetScenarioID:   "smoke",
		OutputDir:          filepath.Join(tmpDir, "verify-out"),
		Runtime:            mockRT,
	})

	if err != nil {
		t.Fatalf("VerifyFix failed: %v", err)
	}

	if !verifyRes.Success {
		t.Errorf("expected verification to succeed, got %s", verifyRes.SummaryMessage)
	}
}
