package fix

import (
	"os"
	"strings"
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/patterns"
	"gopkg.in/yaml.v3"
)

// ProposedPattern models the proposed pattern schema for human review
type ProposedPattern struct {
	ID             string `yaml:"id"`
	Status         string `yaml:"status"`
	Confidence     string `yaml:"confidence"`
	DiscoveredWith string `yaml:"discovered_with"`
	AppliesTo      struct {
		AppType string `yaml:"app_type"`
		Host    struct {
			Session string `yaml:"session"`
		} `yaml:"host"`
	} `yaml:"applies_to"`
	Detect struct {
		LogAny  []string `yaml:"log_any"`
		Symptom string   `yaml:"symptom"`
	} `yaml:"detect"`
	NoiseExcluded []string `yaml:"noise_excluded"`
	Severity      string   `yaml:"severity"`
	Cause         string   `yaml:"cause"`
	Evidence      []struct {
		Kind string `yaml:"kind"`
		Ref  string `yaml:"ref"`
		Line string `yaml:"line,omitempty"`
	} `yaml:"evidence"`
	Fixes []struct {
		ID         string   `yaml:"id"`
		Tier       string   `yaml:"tier"`
		Apply      string   `yaml:"apply"`
		Args       []string `yaml:"args"`
		Undo       string   `yaml:"undo"`
		VerifiedOn []string `yaml:"verified_on"`
	} `yaml:"fixes"`
	Verify struct {
		AliveSeconds               int  `yaml:"alive_seconds"`
		RequireWindow              bool `yaml:"require_window"`
		FatalPatternMustNotRematch bool `yaml:"fatal_pattern_must_not_rematch"`
	} `yaml:"verify"`
	ManualSteps []string `yaml:"manual_steps"`
	Limitations []string `yaml:"limitations"`
}

func TestProposedPatternSchemaValidation(t *testing.T) {
	data, err := os.ReadFile("../patterns/proposed/electron-wayland-no-window.yaml")
	if err != nil {
		t.Fatalf("failed reading proposed pattern: %v", err)
	}

	var p ProposedPattern
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatalf("failed unmarshaling proposed pattern schema: %v", err)
	}

	if p.ID != "electron-wayland-no-window" {
		t.Errorf("expected ID 'electron-wayland-no-window', got %s", p.ID)
	}
	if p.Status != "proposed" {
		t.Errorf("expected status 'proposed', got %s", p.Status)
	}
	if p.Confidence != "confirmed" {
		t.Errorf("expected confidence 'confirmed', got %s", p.Confidence)
	}
	if !strings.Contains(p.DiscoveredWith, "rea") {
		t.Errorf("expected discovered_with to cite rea, got %s", p.DiscoveredWith)
	}
	if len(p.Evidence) == 0 {
		t.Errorf("expected evidence entries in proposed pattern")
	}
	if len(p.Fixes) == 0 || p.Fixes[0].Args[0] != "--ozone-platform=x11" {
		t.Errorf("expected fix #1 args to contain --ozone-platform=x11, got %v", p.Fixes)
	}
	if !p.Verify.RequireWindow || p.Verify.AliveSeconds < 10 {
		t.Errorf("expected verification to require window and >= 10s alive")
	}
}

func TestNuzzDiscovery_GoldenLogs(t *testing.T) {
	c, err := NewDefaultClassifier()
	if err != nil {
		t.Fatalf("failed initializing classifier: %v", err)
	}

	// 1. Real failing log from Nuzz on Fedora Wayland
	failBytes, err := os.ReadFile("testdata/nuzz/failing_run.log")
	if err != nil {
		t.Fatalf("failed reading failing_run.log: %v", err)
	}
	failLines := strings.Split(string(failBytes), "\n")

	failingCtx := ExecutionContext{
		AppType:        "electron",
		SessionType:    "wayland",
		WindowDetected: false,
		ProcessAlive:   true,
		UptimeSeconds:  12,
	}

	res := c.ClassifyLog(failLines, failingCtx)
	if !res.HasFatal() {
		t.Fatalf("expected fatal failure match in real failing log, got none")
	}

	foundP1 := false
	for _, m := range res.FatalMatches {
		if strings.Contains(m.Pattern.ID, "electron") || strings.Contains(m.Pattern.ID, "wayland") {
			foundP1 = true
			break
		}
	}
	if !foundP1 {
		t.Errorf("expected fatal match to identify electron wayland issue, got %v", res.FatalMatches)
	}

	// 2. Real fixed log (with --ozone-platform=x11) must NOT match fatal pattern
	fixedBytes, err := os.ReadFile("testdata/nuzz/fixed_run.log")
	if err != nil {
		t.Fatalf("failed reading fixed_run.log: %v", err)
	}
	fixedLines := strings.Split(string(fixedBytes), "\n")

	fixedCtx := ExecutionContext{
		AppType:        "electron",
		SessionType:    "wayland",
		WindowDetected: true,
		ProcessAlive:   true,
		UptimeSeconds:  12,
	}

	fixedRes := c.ClassifyLog(fixedLines, fixedCtx)
	if fixedRes.HasFatal() {
		t.Errorf("fixed log must NOT trigger fatal match, got %v", fixedRes.FatalMatches)
	}

	// 3. Noise-only log must NOT match fatal pattern
	noiseBytes, err := os.ReadFile("testdata/nuzz/noise_only.log")
	if err != nil {
		t.Fatalf("failed reading noise_only.log: %v", err)
	}
	noiseLines := strings.Split(string(noiseBytes), "\n")

	noiseRes := c.ClassifyLog(noiseLines, fixedCtx)
	if noiseRes.HasFatal() {
		t.Errorf("noise-only log must NOT trigger fatal match, got %v", noiseRes.FatalMatches)
	}
	if len(noiseRes.NoiseMatches) == 0 {
		t.Errorf("expected noise filter to identify noise lines in noise_only.log")
	}

	// 4. False-positive test: unrelated app log must NOT match any pattern
	unrelatedBytes, err := os.ReadFile("testdata/nuzz/unrelated_app.log")
	if err != nil {
		t.Fatalf("failed reading unrelated_app.log: %v", err)
	}
	unrelatedLines := strings.Split(string(unrelatedBytes), "\n")

	unrelatedCtx := ExecutionContext{
		AppType:        "generic",
		SessionType:    "wayland",
		WindowDetected: false,
		ProcessAlive:   true,
		UptimeSeconds:  20,
	}

	unrelatedRes := c.ClassifyLog(unrelatedLines, unrelatedCtx)
	if unrelatedRes.HasFatal() {
		t.Errorf("unrelated app log triggered false-positive fatal match: %v", unrelatedRes.FatalMatches)
	}
	if len(unrelatedRes.NoiseMatches) > 0 {
		t.Errorf("unrelated app log triggered false-positive noise match: %v", unrelatedRes.NoiseMatches)
	}
}

func TestNuzzWrapperGenerationExactArgv(t *testing.T) {
	originalWrapper := `#!/bin/sh
BIN="/home/user/.local/opt/nuzz/AppRun"
FLAGS=""
exec "$BIN" $FLAGS "$@"
`
	fixOption := patterns.FixOption{
		ID:          "force-x11",
		Description: "Configure wrapper for X11/XWayland ozone platform",
		Action:      "wrapper_flags",
		Flags:       []string{"--ozone-platform=x11"},
	}

	modified := ModifyWrapperContent(originalWrapper, fixOption)

	// Verify exact argv structure without shell string interpolation
	if !strings.Contains(modified, `FLAGS="--ozone-platform=x11"`) {
		t.Errorf("expected FLAGS=\"--ozone-platform=x11\" in wrapper, got:\n%s", modified)
	}
	if !strings.Contains(modified, `exec "$BIN" $FLAGS "$@"`) {
		t.Errorf("expected safe exec line, got:\n%s", modified)
	}
	if !strings.Contains(modified, "# COMPAT_FIX_BEGIN: force-x11") {
		t.Errorf("expected fix block boundary comment in wrapper")
	}

	// Verify rollback cleanly removes fix block
	cleanOption := patterns.FixOption{
		ID:     "none",
		Action: "none",
	}
	cleaned := ModifyWrapperContent(modified, cleanOption)
	if strings.Contains(cleaned, "--ozone-platform=x11") {
		t.Errorf("expected clean removal of flags during undo, got:\n%s", cleaned)
	}
}
