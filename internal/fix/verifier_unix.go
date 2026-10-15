//go:build !windows

package fix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/engine/appimage"
)

// DefaultVerifier implements active process stability and window detection verification
type DefaultVerifier struct {
	MinUptime    time.Duration
	PollInterval time.Duration
}

// NewDefaultVerifier creates a new verifier with standard 10-second stability requirement
func NewDefaultVerifier() *DefaultVerifier {
	return &DefaultVerifier{
		MinUptime:    10 * time.Second,
		PollInterval: 500 * time.Millisecond,
	}
}

// HasWindowProbe checks if any supported window inspection tool is available in PATH
func HasWindowProbe() (bool, string) {
	if _, err := exec.LookPath("xprop"); err == nil {
		return true, "xprop"
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		return true, "xdotool"
	}
	if _, err := exec.LookPath("wmctrl"); err == nil {
		return true, "wmctrl"
	}
	if _, err := exec.LookPath("swaymsg"); err == nil && os.Getenv("SWAYSOCK") != "" {
		return true, "swaymsg"
	}
	if _, err := exec.LookPath("hyprctl"); err == nil && os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return true, "hyprctl"
	}
	return false, ""
}

// GetProcessTreePIDs collects all descendant process IDs of rootPid by inspecting /proc
func GetProcessTreePIDs(rootPid int) []int {
	pids := map[int]bool{rootPid: true}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{rootPid}
	}

	parentMap := make(map[int][]int)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		statBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		lastParen := bytes.LastIndexByte(statBytes, ')')
		if lastParen == -1 || lastParen+2 >= len(statBytes) {
			continue
		}
		fields := strings.Fields(string(statBytes[lastParen+2:]))
		if len(fields) >= 2 {
			ppid, err := strconv.Atoi(fields[1])
			if err == nil {
				parentMap[ppid] = append(parentMap[ppid], pid)
			}
		}
	}

	queue := []int{rootPid}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, child := range parentMap[curr] {
			if !pids[child] {
				pids[child] = true
				queue = append(queue, child)
			}
		}
	}

	res := make([]int, 0, len(pids))
	for p := range pids {
		res = append(res, p)
	}
	return res
}

func probeXprop(pids []int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xprop", "-root", "_NET_CLIENT_LIST")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	lines := string(out)
	hashIdx := strings.Index(lines, "#")
	if hashIdx == -1 {
		return false
	}
	rawIDs := strings.Split(lines[hashIdx+1:], ",")

	pidSet := make(map[int]bool)
	for _, p := range pids {
		pidSet[p] = true
	}

	for _, rawID := range rawIDs {
		winID := strings.TrimSpace(rawID)
		if winID == "" || winID == "0x0" {
			continue
		}

		pidCmd := exec.CommandContext(ctx, "xprop", "-id", winID, "_NET_WM_PID")
		pidOut, err := pidCmd.Output()
		if err != nil {
			continue
		}
		// Example: _NET_WM_PID(CARDINAL) = 12345
		if idx := strings.Index(string(pidOut), "="); idx != -1 {
			valStr := strings.TrimSpace(string(pidOut)[idx+1:])
			if val, err := strconv.Atoi(valStr); err == nil && pidSet[val] {
				return true
			}
		}
	}
	return false
}

func probeXdotool(pids []int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	for _, pid := range pids {
		cmd := exec.CommandContext(ctx, "xdotool", "search", "--pid", strconv.Itoa(pid))
		out, err := cmd.Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return true
		}
	}
	return false
}

func probeWmctrl(pids []int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wmctrl", "-l", "-p")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	pidSet := make(map[int]bool)
	for _, p := range pids {
		pidSet[p] = true
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if pidVal, err := strconv.Atoi(fields[2]); err == nil && pidSet[pidVal] {
				return true
			}
		}
	}
	return false
}

func probeCompositorIPC(pids []int) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	pidSet := make(map[int]bool)
	for _, p := range pids {
		pidSet[p] = true
	}

	if _, err := exec.LookPath("swaymsg"); err == nil && os.Getenv("SWAYSOCK") != "" {
		cmd := exec.CommandContext(ctx, "swaymsg", "-t", "get_tree")
		if out, err := cmd.Output(); err == nil {
			for _, p := range pids {
				if bytes.Contains(out, []byte(fmt.Sprintf(`"pid":%d`, p))) ||
					bytes.Contains(out, []byte(fmt.Sprintf(`"pid": %d`, p))) {
					return true, "swaymsg"
				}
			}
		}
	}

	if _, err := exec.LookPath("hyprctl"); err == nil && os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		cmd := exec.CommandContext(ctx, "hyprctl", "clients", "-j")
		if out, err := cmd.Output(); err == nil {
			var clients []struct {
				PID int `json:"pid"`
			}
			if err := json.Unmarshal(out, &clients); err == nil {
				for _, c := range clients {
					if pidSet[c.PID] {
						return true, "hyprctl"
					}
				}
			}
		}
	}

	return false, ""
}

// CheckWindow probes all available mechanisms for an active window belonging to the process tree
func CheckWindow(pids []int) (bool, string) {
	if _, err := exec.LookPath("xprop"); err == nil {
		if probeXprop(pids) {
			return true, "xprop"
		}
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		if probeXdotool(pids) {
			return true, "xdotool"
		}
	}
	if _, err := exec.LookPath("wmctrl"); err == nil {
		if probeWmctrl(pids) {
			return true, "wmctrl"
		}
	}
	if ok, probe := probeCompositorIPC(pids); ok {
		return true, probe
	}
	return false, ""
}

// TerminateProcessTree cleanly terminates a process and all its children
func TerminateProcessTree(pid int) {
	pgid, err := syscall.Getpgid(pid)
	target := pid
	if err == nil && pgid > 0 {
		target = -pgid
	}

	_ = syscall.Kill(target, syscall.SIGTERM)

	// Wait up to 1.5 seconds for graceful exit
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still lingering
	_ = syscall.Kill(target, syscall.SIGKILL)
}

func (v *DefaultVerifier) Verify(ctx context.Context, app *domain.ApplicationRecord, launcherPath string) (*VerificationResult, error) {
	if launcherPath == "" {
		launcherPath = app.LauncherPath
	}

	// For CLI tools, test quick execution with --version or --help
	if app.PackageType == domain.PackageTypeCLI {
		var outBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, launcherPath, "--version")
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf
		err := cmd.Run()
		if err == nil {
			return &VerificationResult{
				Success:     true,
				ExitCode:    0,
				Message:     "CLI command executed successfully with exit code 0 on --version",
				CapturedLog: outBuf.String(),
			}, nil
		}

		// Fallback to --help
		outBuf.Reset()
		cmdHelp := exec.CommandContext(ctx, launcherPath, "--help")
		cmdHelp.Stdout = &outBuf
		cmdHelp.Stderr = &outBuf
		errHelp := cmdHelp.Run()
		if errHelp == nil {
			return &VerificationResult{
				Success:     true,
				ExitCode:    0,
				Message:     "CLI command executed successfully with exit code 0 on --help",
				CapturedLog: outBuf.String(),
			}, nil
		}

		exitCode := 1
		if exitErr, ok := errHelp.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &VerificationResult{
			Success:     false,
			ExitCode:    exitCode,
			Message:     fmt.Sprintf("CLI command failed with exit code %d", exitCode),
			CapturedLog: outBuf.String(),
		}, nil
	}

	// For GUI applications, perform active 10s monitoring with window detection
	hasProbe, probeName := HasWindowProbe()

	var logBuf bytes.Buffer
	cmd := exec.Command(launcherPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.MultiWriter(os.Stdout, &logBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &logBuf)

	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return &VerificationResult{
			Success:     false,
			ExitCode:    -1,
			Message:     fmt.Sprintf("Failed to start process: %v", err),
			CapturedLog: logBuf.String(),
		}, nil
	}

	pid := cmd.Process.Pid
	defer func() {
		TerminateProcessTree(pid)
		appimage.CleanupStaleElectronLocks(app.DisplayName)
		appimage.CleanupStaleElectronLocks(app.AppID)
	}()

	processExited := make(chan error, 1)
	go func() {
		processExited <- cmd.Wait()
	}()

	windowDetected := false
	windowProbeUsed := ""
	pollTicker := time.NewTicker(v.PollInterval)
	defer pollTicker.Stop()

	targetDuration := v.MinUptime
	timer := time.NewTimer(targetDuration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return &VerificationResult{
				Success:     false,
				Message:     "Verification timed out or was cancelled by user context",
				CapturedLog: logBuf.String(),
			}, ctx.Err()

		case err := <-processExited:
			elapsed := time.Since(startTime)
			exitCode := 0
			if err != nil {
				exitCode = 1
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}
			return &VerificationResult{
				Success:     false,
				Uptime:      elapsed,
				ExitCode:    exitCode,
				Message:     fmt.Sprintf("Application process exited prematurely after %v (exit code: %d)", elapsed.Round(100*time.Millisecond), exitCode),
				CapturedLog: logBuf.String(),
			}, nil

		case <-pollTicker.C:
			if !windowDetected {
				treePIDs := GetProcessTreePIDs(pid)
				if winFound, probe := CheckWindow(treePIDs); winFound {
					windowDetected = true
					windowProbeUsed = probe
				}
			}

		case <-timer.C:
			elapsed := time.Since(startTime)
			if windowDetected {
				return &VerificationResult{
					Success:         true,
					Uptime:          elapsed,
					WindowDetected:  true,
					WindowProbeUsed: windowProbeUsed,
					Message:         fmt.Sprintf("Application remained stable for %v and window was detected via %s", elapsed.Round(100*time.Millisecond), windowProbeUsed),
					CapturedLog:     logBuf.String(),
				}, nil
			}

			if hasProbe {
				return &VerificationResult{
					Success:         false,
					Uptime:          elapsed,
					WindowDetected:  false,
					WindowProbeUsed: probeName,
					Message:         fmt.Sprintf("Application process remained alive for %v but no window appeared", elapsed.Round(100*time.Millisecond)),
					CapturedLog:     logBuf.String(),
				}, nil
			}

			// Probes unavailable on host
			return &VerificationResult{
				Success:          true,
				Uptime:           elapsed,
				WindowDetected:   false,
				ProbeUnavailable: true,
				WindowProbeUsed:  "none",
				Message:          fmt.Sprintf("Window probes unavailable on host; application remained stable for %v without crashing", elapsed.Round(100*time.Millisecond)),
				CapturedLog:      logBuf.String(),
			}, nil
		}
	}
}
