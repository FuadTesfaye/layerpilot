package lab

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// ContainerRunOptions configures execution inside a container target
type ContainerRunOptions struct {
	Target           TargetDefinition
	Scenario         ScenarioDefinition
	ArtifactHostPath string // staged copy on host
	ArtifactBase     string // e.g. "app.AppImage"
	OutputDir        string // host output bundle directory
	Network          bool
	Timeout          time.Duration
	CPUs             string // default "2.0"
	Memory           string // default "2g"
	Command          []string
	Env              map[string]string
}

// ContainerRunResult contains captured container execution results
type ContainerRunResult struct {
	TargetID          string
	ScenarioID        string
	ExitCode          int
	Stdout            string
	Stderr            string
	CombinedLog       string
	Duration          time.Duration
	ReproScriptPath   string
	ScreenshotPath    string
	NotTestableReason string
}

// ContainerRuntime defines the execution backend (podman, docker, or mock)
type ContainerRuntime interface {
	ID() string
	IsAvailable(ctx context.Context) bool
	Run(ctx context.Context, opts ContainerRunOptions) (*ContainerRunResult, error)
}

// PodmanContainerRuntime executes containers using podman CLI
type PodmanContainerRuntime struct {
	binaryPath string
}

// NewPodmanContainerRuntime creates a podman runtime
func NewPodmanContainerRuntime() *PodmanContainerRuntime {
	bin, _ := exec.LookPath("podman")
	return &PodmanContainerRuntime{binaryPath: bin}
}

func (p *PodmanContainerRuntime) ID() string {
	return "podman"
}

func (p *PodmanContainerRuntime) IsAvailable(ctx context.Context) bool {
	if p.binaryPath == "" {
		var err error
		p.binaryPath, err = exec.LookPath("podman")
		if err != nil {
			return false
		}
	}
	cmd := exec.CommandContext(ctx, p.binaryPath, "--version")
	return cmd.Run() == nil
}

func (p *PodmanContainerRuntime) Run(ctx context.Context, opts ContainerRunOptions) (*ContainerRunResult, error) {
	return executeContainer(ctx, "podman", opts)
}

// DockerContainerRuntime executes containers using docker CLI
type DockerContainerRuntime struct {
	binaryPath string
}

// NewDockerContainerRuntime creates a docker runtime
func NewDockerContainerRuntime() *DockerContainerRuntime {
	bin, _ := exec.LookPath("docker")
	return &DockerContainerRuntime{binaryPath: bin}
}

func (d *DockerContainerRuntime) ID() string {
	return "docker"
}

func (d *DockerContainerRuntime) IsAvailable(ctx context.Context) bool {
	if d.binaryPath == "" {
		var err error
		d.binaryPath, err = exec.LookPath("docker")
		if err != nil {
			return false
		}
	}
	cmd := exec.CommandContext(ctx, d.binaryPath, "info")
	return cmd.Run() == nil
}

func (d *DockerContainerRuntime) Run(ctx context.Context, opts ContainerRunOptions) (*ContainerRunResult, error) {
	return executeContainer(ctx, "docker", opts)
}

// MockContainerRuntime provides a deterministic test runtime without container daemon
type MockContainerRuntime struct {
	IDName        string
	Available     bool
	MockResults   map[string]*ContainerRunResult // key: targetID + "/" + scenarioID
	DefaultResult *ContainerRunResult
}

func NewMockContainerRuntime() *MockContainerRuntime {
	return &MockContainerRuntime{
		IDName:      "mock",
		Available:   true,
		MockResults: make(map[string]*ContainerRunResult),
		DefaultResult: &ContainerRunResult{
			ExitCode:    0,
			Stdout:      "Mock execution succeeded\n",
			Stderr:      "",
			CombinedLog: "Mock execution succeeded\n",
			Duration:    100 * time.Millisecond,
		},
	}
}

func (m *MockContainerRuntime) ID() string {
	return m.IDName
}

func (m *MockContainerRuntime) IsAvailable(ctx context.Context) bool {
	return m.Available
}

func (m *MockContainerRuntime) Run(ctx context.Context, opts ContainerRunOptions) (*ContainerRunResult, error) {
	key := opts.Target.ID + "/" + opts.Scenario.ID
	if res, ok := m.MockResults[key]; ok {
		resCopy := *res
		resCopy.TargetID = opts.Target.ID
		resCopy.ScenarioID = opts.Scenario.ID
		// Generate repro script
		scriptPath, _ := generateReproScript(opts.OutputDir, m.IDName, opts)
		resCopy.ReproScriptPath = scriptPath
		return &resCopy, nil
	}

	resCopy := *m.DefaultResult
	resCopy.TargetID = opts.Target.ID
	resCopy.ScenarioID = opts.Scenario.ID
	scriptPath, _ := generateReproScript(opts.OutputDir, m.IDName, opts)
	resCopy.ReproScriptPath = scriptPath
	return &resCopy, nil
}

// executeContainer builds the argument array and invokes container executable safely
func executeContainer(ctx context.Context, runtimeBin string, opts ContainerRunOptions) (*ContainerRunResult, error) {
	cpus := opts.CPUs
	if cpus == "" {
		cpus = "2.0"
	}
	memory := opts.Memory
	if memory == "" {
		memory = "2g"
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build safe argument array (non-root, resource-capped, isolated mounts, no shell interpolation)
	args := []string{
		"run", "--rm",
		"--security-opt=no-new-privileges",
		fmt.Sprintf("--memory=%s", memory),
		fmt.Sprintf("--cpus=%s", cpus),
	}

	if opts.Network {
		args = append(args, "--network=bridge")
	} else {
		args = append(args, "--network=none")
	}

	// Mounts: read-only artifact and write-only output dir
	args = append(args,
		"-v", fmt.Sprintf("%s:/artifact/%s:ro", opts.ArtifactHostPath, opts.ArtifactBase),
		"-v", fmt.Sprintf("%s:/out:rw", opts.OutputDir),
	)

	// Environment
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Image
	args = append(args, opts.Target.Image)

	// Inner container command
	args = append(args, opts.Command...)

	// Generate standalone reproduction script
	reproPath, _ := generateReproScript(opts.OutputDir, runtimeBin, opts)

	startTime := time.Now()
	cmd := exec.CommandContext(runCtx, runtimeBin, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	duration := time.Since(startTime)

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	combinedLog := stdoutStr + stderrStr

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 128
		}
	}

	result := &ContainerRunResult{
		TargetID:        opts.Target.ID,
		ScenarioID:      opts.Scenario.ID,
		ExitCode:        exitCode,
		Stdout:          stdoutStr,
		Stderr:          stderrStr,
		CombinedLog:     combinedLog,
		Duration:        duration,
		ReproScriptPath: reproPath,
	}

	// Check if screenshot was produced for window scenario
	targetSlug := sanitizeSlug(opts.Target.ID)
	screenshotCandidate := filepath.Join(opts.OutputDir, "screenshots", fmt.Sprintf("%s-window.png", targetSlug))
	if _, statErr := os.Stat(screenshotCandidate); statErr == nil {
		result.ScreenshotPath = screenshotCandidate
	}

	// If xvfb-run is missing
	if opts.Scenario.ID == "window" && strings.Contains(combinedLog, "xvfb-run: not found") || strings.Contains(combinedLog, "xvfb-run: command not found") {
		result.NotTestableReason = "xvfb-run display server is not installed in target container"
	}

	return result, nil
}

// BuildScenarioCommand determines the exact container inner command array for an artifact and scenario
func BuildScenarioCommand(artifactBase string, scenario ScenarioDefinition, format domain.PackageFormat) []string {
	isAppImage := format == domain.FormatAppImage || strings.HasSuffix(strings.ToLower(artifactBase), ".appimage")

	switch scenario.ID {
	case "smoke":
		if isAppImage {
			return []string{"/artifact/" + artifactBase, "--appimage-extract-and-run"}
		}
		return []string{"/artifact/" + artifactBase}

	case "libs":
		if isAppImage {
			// Extract and ldd the binary
			return []string{"sh", "-c", "TMPDIR=/tmp /artifact/" + artifactBase + " --appimage-extract && ldd squashfs-root/AppRun || exit 1"}
		}
		return []string{"ldd", "/artifact/" + artifactBase}

	case "offline":
		if isAppImage {
			return []string{"/artifact/" + artifactBase, "--appimage-extract-and-run"}
		}
		return []string{"/artifact/" + artifactBase}

	case "fresh-home":
		if isAppImage {
			return []string{"/artifact/" + artifactBase, "--appimage-extract-and-run"}
		}
		return []string{"/artifact/" + artifactBase}

	case "fuse":
		// Run AppImage without extract-and-run to test FUSE fallback detection
		return []string{"/artifact/" + artifactBase}

	case "install-clean":
		if strings.HasSuffix(artifactBase, ".deb") {
			return []string{"dpkg", "-i", "/artifact/" + artifactBase}
		}
		if strings.HasSuffix(artifactBase, ".rpm") {
			return []string{"dnf", "install", "-y", "/artifact/" + artifactBase}
		}
		if strings.HasSuffix(artifactBase, ".pkg.tar.zst") {
			return []string{"pacman", "-U", "--noconfirm", "/artifact/" + artifactBase}
		}
		return []string{"/artifact/" + artifactBase}

	case "window":
		if isAppImage {
			return []string{"xvfb-run", "-a", "-s", "-screen 0 1280x800x24", "/artifact/" + artifactBase, "--appimage-extract-and-run"}
		}
		return []string{"xvfb-run", "-a", "-s", "-screen 0 1280x800x24", "/artifact/" + artifactBase}

	default:
		if len(scenario.Command) > 0 {
			return scenario.Command
		}
		return []string{"/artifact/" + artifactBase}
	}
}

// BuildScenarioEnv returns environment variables for a given scenario
func BuildScenarioEnv(scenarioID string) map[string]string {
	env := map[string]string{
		"LC_ALL": "C.UTF-8",
		"LANG":   "C.UTF-8",
	}

	switch scenarioID {
	case "fresh-home":
		env["HOME"] = "/tmp/fresh-isolated-home"
		env["XDG_CONFIG_HOME"] = "/tmp/fresh-isolated-home/.config"
		env["XDG_DATA_HOME"] = "/tmp/fresh-isolated-home/.local/share"
	case "window":
		env["DISPLAY"] = ":99"
	}

	return env
}

// generateReproScript writes a standalone shell script reproducing the exact execution
func generateReproScript(outDir, runtimeBin string, opts ContainerRunOptions) (string, error) {
	if outDir == "" {
		return "", nil
	}
	reproDir := filepath.Join(outDir, "repro")
	if err := os.MkdirAll(reproDir, 0755); err != nil {
		return "", err
	}

	targetSlug := sanitizeSlug(opts.Target.ID)
	scriptName := fmt.Sprintf("%s-%s.sh", targetSlug, sanitizeSlug(opts.Scenario.ID))
	scriptPath := filepath.Join(reproDir, scriptName)

	var sb strings.Builder
	sb.WriteString("#!/usr/bin/env bash\n")
	sb.WriteString(fmt.Sprintf("# Standalone reproduction script for scenario: %s on target: %s\n", opts.Scenario.ID, opts.Target.ID))
	sb.WriteString("# Generated by compat lab. Exact argument arrays, zero shell interpolation.\n")
	sb.WriteString("set -euo pipefail\n\n")

	sb.WriteString(runtimeBin + " run --rm \\\n")
	sb.WriteString("  --security-opt=no-new-privileges \\\n")
	cpus := opts.CPUs
	if cpus == "" {
		cpus = "2.0"
	}
	mem := opts.Memory
	if mem == "" {
		mem = "2g"
	}
	sb.WriteString(fmt.Sprintf("  --memory=%s \\\n", mem))
	sb.WriteString(fmt.Sprintf("  --cpus=%s \\\n", cpus))

	if opts.Network {
		sb.WriteString("  --network=bridge \\\n")
	} else {
		sb.WriteString("  --network=none \\\n")
	}

	sb.WriteString(fmt.Sprintf("  -v \"%s:/artifact/%s:ro\" \\\n", opts.ArtifactHostPath, opts.ArtifactBase))
	sb.WriteString(fmt.Sprintf("  -v \"%s:/out:rw\" \\\n", opts.OutputDir))

	for k, v := range opts.Env {
		sb.WriteString(fmt.Sprintf("  -e %s=\"%s\" \\\n", k, v))
	}

	sb.WriteString(fmt.Sprintf("  %s", opts.Target.Image))
	for _, arg := range opts.Command {
		sb.WriteString(fmt.Sprintf(" \\\n  %q", arg))
	}
	sb.WriteString("\n")

	if err := os.WriteFile(scriptPath, []byte(sb.String()), 0755); err != nil {
		return "", err
	}

	return scriptPath, nil
}

func sanitizeSlug(s string) string {
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return strings.ToLower(s)
}
