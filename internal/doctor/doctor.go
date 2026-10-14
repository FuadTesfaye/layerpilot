package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/hostinfo"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// CheckStatus indicates individual check outcome
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult records a discrete diagnostic check
type CheckResult struct {
	Category   string      `json:"category"`
	Name       string      `json:"name"`
	Status     CheckStatus `json:"status"`
	Message    string      `json:"message"`
	Impact     string      `json:"impact,omitempty"`
	Suggestion string      `json:"suggestion,omitempty"`
}

// DoctorReport contains all findings
type DoctorReport struct {
	Healthy bool          `json:"healthy"`
	Checks  []CheckResult `json:"checks"`
}

// Doctor coordinates diagnostic evaluation
type Doctor struct {
	profiler *hostinfo.Profiler
	registry *engine.Registry
	runner   runner.Runner
}

// New creates a new Doctor instance
func New(p *hostinfo.Profiler, r *engine.Registry, run runner.Runner) *Doctor {
	return &Doctor{
		profiler: p,
		registry: r,
		runner:   run,
	}
}

// Diagnose evaluates host readiness, engine availability, and storage health
func (d *Doctor) Diagnose(ctx context.Context) *DoctorReport {
	report := &DoctorReport{
		Healthy: true,
		Checks:  []CheckResult{},
	}

	host, err := d.profiler.Current()
	if err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Category:   "Host",
			Name:       "Host Profiling",
			Status:     StatusFail,
			Message:    "Failed to profile host: " + err.Error(),
			Impact:     "Cannot evaluate compatibility matrix",
			Suggestion: "Check permissions to /etc/os-release and /proc",
		})
		report.Healthy = false
		return report
	}

	// 1. Host OS and Arch
	report.Checks = append(report.Checks, CheckResult{
		Category: "Host",
		Name:     "Operating System",
		Status:   StatusPass,
		Message:  fmt.Sprintf("%s (%s %s) on %s", host.OS, host.Distro, host.DistroVersion, host.Arch),
	})

	// 2. Display & Audio
	if host.DisplayServer != "headless" {
		report.Checks = append(report.Checks, CheckResult{
			Category: "Desktop",
			Name:     "Display Server",
			Status:   StatusPass,
			Message:  fmt.Sprintf("%s display server detected", host.DisplayServer),
		})
	} else {
		report.Checks = append(report.Checks, CheckResult{
			Category:   "Desktop",
			Name:       "Display Server",
			Status:     StatusWarn,
			Message:    "No active display server detected (headless session)",
			Impact:     "GUI applications will not be able to render a window",
			Suggestion: "Ensure WAYLAND_DISPLAY or DISPLAY is exported when launching GUI apps",
		})
	}

	// 3. User namespace / subuid mapping for rootless containers
	if host.OS == "linux" {
		if host.RootlessPodman {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Containers",
				Name:     "User Namespace SubUID/SubGID",
				Status:   StatusPass,
				Message:  "Rootless subuid/subgid allocations configured properly",
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Containers",
				Name:       "User Namespace SubUID/SubGID",
				Status:     StatusWarn,
				Message:    "Missing /etc/subuid or /etc/subgid entries for current user",
				Impact:     "Rootless container engine may require root daemon fallback",
				Suggestion: "Run: sudo usermod --add-subuids 100000-165535 --add-subgids 100000-165535 $USER",
			})
		}
	}

	// 4. Engine probes
	reports := d.registry.ProbeAll(ctx, host)
	for id, rep := range reports {
		if rep.Available {
			msg := fmt.Sprintf("Available (version: %s)", rep.Version)
			if rep.Rootless {
				msg += " [rootless enabled]"
			}
			report.Checks = append(report.Checks, CheckResult{
				Category: "Engine",
				Name:     rep.EngineName,
				Status:   StatusPass,
				Message:  msg,
			})
		} else {
			status := StatusWarn
			if id == "podman" && host.OS == "linux" {
				status = StatusFail
				report.Healthy = false
			}
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Engine",
				Name:       rep.EngineName,
				Status:     status,
				Message:    "Not available",
				Impact:     fmt.Sprintf("Cannot use %s compatibility engine", id),
				Suggestion: fmt.Sprintf("Check %s installation", id),
			})
		}
	}

	// 5. State storage directory permissions
	home := os.Getenv("HOME")
	stateDir := filepath.Join(home, ".local", "share", "compat")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Category:   "Storage",
			Name:       "State Directory",
			Status:     StatusFail,
			Message:    fmt.Sprintf("Cannot create state directory: %s", stateDir),
			Impact:     "Application registry and launcher state cannot be persisted",
			Suggestion: fmt.Sprintf("Fix permissions on %s", stateDir),
		})
		report.Healthy = false
	} else {
		report.Checks = append(report.Checks, CheckResult{
			Category: "Storage",
			Name:     "State Directory",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Directory %s is writable", stateDir),
		})
	}

	return report
}
