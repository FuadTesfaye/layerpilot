package lab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	rt "runtime"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// RunLabOptions configures a multi-distro compatibility lab test run
type RunLabOptions struct {
	ArtifactPath string
	Targets      []TargetDefinition
	Scenarios    []ScenarioDefinition
	OutputDir    string
	Runtime      ContainerRuntime
	Network      bool
	Timeout      time.Duration
}

// RunLabSuite executes the full compatibility matrix across targets and scenarios
func RunLabSuite(ctx context.Context, opts RunLabOptions) (*LabReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Verify artifact exists
	artStat, err := os.Stat(opts.ArtifactPath)
	if err != nil {
		return nil, fmt.Errorf("artifact not found at %s: %w", opts.ArtifactPath, err)
	}

	// Calculate SHA256 of artifact
	artHash, err := hashFile(opts.ArtifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed computing artifact hash: %w", err)
	}

	// 2. Select and verify container runtime
	runtime := opts.Runtime
	if runtime == nil {
		podman := NewPodmanContainerRuntime()
		if podman.IsAvailable(ctx) {
			runtime = podman
		} else {
			docker := NewDockerContainerRuntime()
			if docker.IsAvailable(ctx) {
				runtime = docker
			} else {
				return nil, fmt.Errorf("no container runtime available; install podman or docker (run 'compat lab doctor' for details)")
			}
		}
	}

	// 3. Prepare targets and scenarios
	targets := opts.Targets
	if len(targets) == 0 {
		targets, _ = FilterTargets(nil)
	}

	scenarios := opts.Scenarios
	if len(scenarios) == 0 {
		scenarios = FilterScenarios(nil)
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = "compat-report"
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("failed creating output directory %s: %w", outDir, err)
	}
	logsDir := filepath.Join(outDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	screensDir := filepath.Join(outDir, "screenshots")
	_ = os.MkdirAll(screensDir, 0755)
	reproDir := filepath.Join(outDir, "repro")
	_ = os.MkdirAll(reproDir, 0755)

	// 4. Hard Rule 6: Never modify developer's original artifact; work on staged copy
	stageDir, err := os.MkdirTemp("", "compat-stage-*")
	if err != nil {
		return nil, fmt.Errorf("failed creating staging directory: %w", err)
	}
	defer os.RemoveAll(stageDir)

	artifactBase := filepath.Base(opts.ArtifactPath)
	stagedArtifactPath := filepath.Join(stageDir, artifactBase)
	if err := copyFile(opts.ArtifactPath, stagedArtifactPath); err != nil {
		return nil, fmt.Errorf("failed staging artifact copy: %w", err)
	}
	_ = os.Chmod(stagedArtifactPath, 0755)

	// 5. T0 Static ELF Scan
	staticScanner := NewStaticScanner()
	staticScan, _ := staticScanner.Scan(stagedArtifactPath, targets)

	var pkgFormat domain.PackageFormat
	if staticScan != nil {
		pkgFormat = domain.PackageFormat(staticScan.AppProfile.Format)
	}

	// 6. Initialize Evaluator and Redactor
	evaluator, err := NewEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed initializing evaluator: %w", err)
	}
	redactor := NewRedactor()

	// 7. Execute test matrix
	var results []TargetResult
	summary := LabSummary{}

	var targetIDs []string
	for _, t := range targets {
		targetIDs = append(targetIDs, t.ID)
	}
	var scenarioIDs []string
	for _, s := range scenarios {
		scenarioIDs = append(scenarioIDs, s.ID)
	}

	appProf := AppProfile{Framework: "generic"}
	if staticScan != nil {
		appProf = staticScan.AppProfile
	}

	for _, target := range targets {
		for _, scenario := range scenarios {
			summary.TotalScenarios++

			if scenario.ID == "static-scan" {
				res := evaluator.EvaluateStaticScan(target, staticScan)
				tallySummary(&summary, res.Verdict)
				results = append(results, res)
				continue
			}

			// T1 Container execution
			innerCmd := BuildScenarioCommand(artifactBase, scenario, pkgFormat)
			innerEnv := BuildScenarioEnv(scenario.ID)

			runOpts := ContainerRunOptions{
				Target:           target,
				Scenario:         scenario,
				ArtifactHostPath: stagedArtifactPath,
				ArtifactBase:     artifactBase,
				OutputDir:        outDir,
				Network:          opts.Network || scenario.Network,
				Timeout:          opts.Timeout,
				Command:          innerCmd,
				Env:              innerEnv,
			}

			runRes, runErr := runtime.Run(ctx, runOpts)
			if runErr != nil {
				res := TargetResult{
					TargetID:         target.ID,
					ScenarioID:       scenario.ID,
					Tier:             TierT1Container,
					Verdict:          VerdictInconclusive,
					Confidence:       ConfidenceHypothesis,
					Cause:            fmt.Sprintf("Container run invocation error: %v", runErr),
					DurationMs:       0,
					ExitCode:         128,
					UntestableReason: runErr.Error(),
				}
				tallySummary(&summary, res.Verdict)
				results = append(results, res)
				continue
			}

			// Redact log and persist to logs/<target-slug>-<scenario-id>.log
			redactedLog := redactor.Redact(runRes.CombinedLog)
			targetSlug := sanitizeSlug(target.ID)
			logRelPath := filepath.Join("logs", fmt.Sprintf("%s-%s.log", targetSlug, sanitizeSlug(scenario.ID)))
			logFullPath := filepath.Join(outDir, logRelPath)
			_ = os.WriteFile(logFullPath, []byte(redactedLog), 0644)

			res := evaluator.EvaluateContainerRun(target, scenario, runRes, appProf)
			res.RedactedLogRef = logRelPath
			if runRes.ReproScriptPath != "" {
				res.ReproScriptRef = filepath.Join("repro", filepath.Base(runRes.ReproScriptPath))
			}
			if runRes.ScreenshotPath != "" {
				res.ScreenshotRef = filepath.Join("screenshots", filepath.Base(runRes.ScreenshotPath))
			}

			tallySummary(&summary, res.Verdict)
			results = append(results, res)
		}
	}

	// 8. Assemble LabReport
	report := &LabReport{
		RunID:               fmt.Sprintf("run-%d", time.Now().Unix()),
		Timestamp:           time.Now().UTC(),
		ArtifactPath:        redactor.Redact(opts.ArtifactPath),
		ArtifactSHA256:      artHash,
		ArtifactSize:        artStat.Size(),
		RuntimeID:           runtime.ID(),
		IsEmulated:          func() bool { em, _ := CheckEmulation(rt.GOARCH, appProf.Architecture); return em }(),
		Targets:             targetIDs,
		Scenarios:           scenarioIDs,
		Results:             results,
		StaticScan:          staticScan,
		Summary:             summary,
		FidelityLimitations: T1FidelityLimitations,
	}

	// 9. Write context bundle
	writer := NewBundleWriter(redactor)
	if err := writer.WriteBundle(outDir, report); err != nil {
		return nil, fmt.Errorf("failed writing context bundle: %w", err)
	}

	return report, nil
}

func tallySummary(s *LabSummary, v LabVerdict) {
	switch v {
	case VerdictPass:
		s.Passed++
	case VerdictFail:
		s.Failed++
	case VerdictInconclusive:
		s.Inconclusive++
	case VerdictNotTestableHere:
		s.NotTestable++
	}
}

func hashFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
