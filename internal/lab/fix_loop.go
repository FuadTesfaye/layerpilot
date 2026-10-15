package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VerificationRequest parameters for testing an AI-agent fix
type VerificationRequest struct {
	OriginalReportPath string
	NewArtifactPath    string
	TargetScenarioID   string
	OutputDir          string
	Runtime            ContainerRuntime
}

// VerificationResult summarizes the outcome of the fix verification
type VerificationResult struct {
	Success           bool       `json:"success"`
	ScenarioID        string     `json:"scenario_id"`
	OriginalVerdict   LabVerdict `json:"original_verdict"`
	NewVerdict        LabVerdict `json:"new_verdict"`
	RegressionsCount  int        `json:"regressions_count"`
	RegressionDetails []string   `json:"regression_details,omitempty"`
	SummaryMessage    string     `json:"summary_message"`
	NewReport         *LabReport `json:"new_report,omitempty"`
}

// VerifyFix verifies whether a new artifact successfully fixes a prior failure without regressions
func VerifyFix(ctx context.Context, req VerificationRequest) (*VerificationResult, error) {
	// 1. Locate and load original report
	reportPath := req.OriginalReportPath
	if stat, err := os.Stat(reportPath); err == nil && stat.IsDir() {
		reportPath = filepath.Join(reportPath, "report.json")
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read original report from %s: %w", reportPath, err)
	}

	var originalReport LabReport
	if err := json.Unmarshal(data, &originalReport); err != nil {
		return nil, fmt.Errorf("failed to parse original report: %w", err)
	}

	// 2. Validate new artifact path
	if _, err := os.Stat(req.NewArtifactPath); err != nil {
		return nil, fmt.Errorf("candidate artifact not found at %s: %w", req.NewArtifactPath, err)
	}

	// 3. Determine target scenario
	targetScenarioID := req.TargetScenarioID
	if targetScenarioID == "" {
		// Default to first failing scenario
		for _, r := range originalReport.Results {
			if r.Verdict == VerdictFail {
				targetScenarioID = r.ScenarioID
				break
			}
		}
	}
	if targetScenarioID == "" {
		return nil, fmt.Errorf("no failing scenario found in original report to verify")
	}

	// Find original verdict for this scenario
	var origVerdict LabVerdict = VerdictInconclusive
	for _, r := range originalReport.Results {
		if r.ScenarioID == targetScenarioID {
			origVerdict = r.Verdict
			break
		}
	}

	// 4. Resolve targets and scenarios to run
	targets, err := FilterTargets(originalReport.Targets)
	if err != nil {
		return nil, err
	}

	scenarios := FilterScenarios(originalReport.Scenarios)

	outDir := req.OutputDir
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), fmt.Sprintf("compat-verify-%d", os.Getpid()))
	}

	// 5. Run full suite against the new artifact
	newReport, err := RunLabSuite(ctx, RunLabOptions{
		ArtifactPath: req.NewArtifactPath,
		Targets:      targets,
		Scenarios:    scenarios,
		OutputDir:    outDir,
		Runtime:      req.Runtime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute verification test suite: %w", err)
	}

	// 6. Check outcome of target scenario
	var newVerdict LabVerdict = VerdictInconclusive
	for _, r := range newReport.Results {
		if r.ScenarioID == targetScenarioID {
			newVerdict = r.Verdict
			if r.Verdict == VerdictFail {
				newVerdict = VerdictFail
				break
			}
		}
	}

	res := &VerificationResult{
		ScenarioID:      targetScenarioID,
		OriginalVerdict: origVerdict,
		NewVerdict:      newVerdict,
		NewReport:       newReport,
	}

	// Check if target scenario passed
	targetScenarioPassed := (newVerdict == VerdictPass)

	// 7. Check for regressions in other scenarios
	var regressions []string
	origPassedMap := make(map[string]bool)
	for _, r := range originalReport.Results {
		key := fmt.Sprintf("%s/%s", r.TargetID, r.ScenarioID)
		if r.Verdict == VerdictPass {
			origPassedMap[key] = true
		}
	}

	for _, r := range newReport.Results {
		key := fmt.Sprintf("%s/%s", r.TargetID, r.ScenarioID)
		if origPassedMap[key] && r.Verdict != VerdictPass {
			reg := fmt.Sprintf("Regression on target %s in scenario %s: previously 'pass', now '%s'",
				r.TargetID, r.ScenarioID, r.Verdict)
			regressions = append(regressions, reg)
		}
	}

	res.RegressionsCount = len(regressions)
	res.RegressionDetails = regressions

	if targetScenarioPassed && len(regressions) == 0 {
		res.Success = true
		res.SummaryMessage = fmt.Sprintf("Fix verified successfully! Scenario '%s' now passes with zero regressions.", targetScenarioID)
	} else if !targetScenarioPassed {
		res.Success = false
		res.SummaryMessage = fmt.Sprintf("Verification failed: scenario '%s' did not pass (verdict: %s).", targetScenarioID, newVerdict)
	} else {
		res.Success = false
		res.SummaryMessage = fmt.Sprintf("Verification failed: scenario '%s' passed, but %d regression(s) were detected.", targetScenarioID, len(regressions))
	}

	return res, nil
}
