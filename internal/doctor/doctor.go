package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// 1. Host OS, Distro Family, and Architecture
	report.Checks = append(report.Checks, CheckResult{
		Category: "Host",
		Name:     "Operating System",
		Status:   StatusPass,
		Message:  fmt.Sprintf("%s (%s %s) [Family: %s] on %s", host.OS, host.Distro, host.DistroVersion, host.DistroFamily, host.Arch),
	})

	// 2. Available Package Managers
	if len(host.PackageManagers) > 0 {
		report.Checks = append(report.Checks, CheckResult{
			Category: "Host",
			Name:     "Package Managers",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Detected: %s", strings.Join(host.PackageManagers, ", ")),
		})
	} else {
		report.Checks = append(report.Checks, CheckResult{
			Category:   "Host",
			Name:       "Package Managers",
			Status:     StatusWarn,
			Message:    "No common package manager detected in PATH",
			Impact:     "Automatic system dependency resolution may be unavailable",
			Suggestion: "Ensure system package managers (dnf, apt, pacman) are in PATH",
		})
	}

	// 3. Desktop Session, Compositor, and GPU Driver
	if host.DisplayServer != "headless" {
		msg := fmt.Sprintf("%s session", host.DisplayServer)
		if host.Compositor != "" {
			msg += fmt.Sprintf(" (%s)", host.Compositor)
		}
		if host.GPUDriver != "" && host.GPUDriver != "generic" {
			msg += fmt.Sprintf(" [GPU: %s]", host.GPUDriver)
		}
		report.Checks = append(report.Checks, CheckResult{
			Category: "Desktop",
			Name:     "Display Server & GPU",
			Status:   StatusPass,
			Message:  msg,
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

	// 4. Security Modules: SELinux & AppArmor
	if host.OS == "linux" {
		if host.SELinuxMode != "disabled" {
			status := StatusPass
			if host.SELinuxMode == "enforcing" {
				report.Checks = append(report.Checks, CheckResult{
					Category: "Security",
					Name:     "SELinux",
					Status:   status,
					Message:  "SELinux is enforcing. compat respects standard confinement policies without disabling SELinux.",
				})
			} else {
				report.Checks = append(report.Checks, CheckResult{
					Category: "Security",
					Name:     "SELinux",
					Status:   StatusPass,
					Message:  fmt.Sprintf("SELinux is %s", host.SELinuxMode),
				})
			}
		}

		if host.AppArmorState != "disabled" {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Security",
				Name:     "AppArmor",
				Status:   StatusPass,
				Message:  fmt.Sprintf("AppArmor is %s", host.AppArmorState),
			})
		}
	}

	// 5. FUSE & libfuse2 Support
	if host.OS == "linux" {
		if host.FUSESupport {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Filesystem",
				Name:     "Kernel FUSE (/dev/fuse)",
				Status:   StatusPass,
				Message:  "Kernel FUSE device available for direct mounting",
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Filesystem",
				Name:       "Kernel FUSE (/dev/fuse)",
				Status:     StatusWarn,
				Message:    "/dev/fuse not accessible",
				Impact:     "AppImages will require extract-and-run fallback",
				Suggestion: "Ensure fuse module is loaded: sudo modprobe fuse",
			})
		}

		if host.Libfuse2Available {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Filesystem",
				Name:     "libfuse.so.2 Library",
				Status:   StatusPass,
				Message:  "libfuse2 runtime library installed (AppImage Type 1/2 compatible)",
			})
		} else {
			sugg := "Install libfuse2 using your package manager (e.g. 'sudo dnf install fuse-libs' or 'sudo apt install libfuse2')"
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Filesystem",
				Name:       "libfuse.so.2 Library",
				Status:     StatusWarn,
				Message:    "libfuse.so.2 not found in system library search paths",
				Impact:     "Older Type 2 AppImages will require automatic extraction to run",
				Suggestion: sugg,
			})
		}
	}

	// 6. User Namespaces & Noexec Mounts
	if host.OS == "linux" {
		if host.UserNamespaces {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Security",
				Name:     "User Namespaces",
				Status:   StatusPass,
				Message:  "Unprivileged user namespaces enabled (rootless sandboxing supported)",
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Security",
				Name:       "User Namespaces",
				Status:     StatusWarn,
				Message:    "Unprivileged user namespaces disabled",
				Impact:     "Rootless containers and sandboxed browser engines will fail",
				Suggestion: "Enable user namespaces: sudo sysctl -w kernel.unprivileged_userns_clone=1",
			})
		}

		if len(host.NoexecMounts) > 0 {
			report.Checks = append(report.Checks, CheckResult{
				Category:   "Filesystem",
				Name:       "Noexec Mounts",
				Status:     StatusWarn,
				Message:    fmt.Sprintf("Found noexec flag on: %s", strings.Join(host.NoexecMounts, ", ")),
				Impact:     "Executing downloaded binaries directly from these mount points will fail",
				Suggestion: "compat will stage executables in writable directories without noexec",
			})
		}
	}

	// 7. Containers: Podman & Distrobox
	if host.OS == "linux" {
		if host.PodmanAvailable {
			msg := "Podman installed"
			if host.RootlessPodman {
				msg += " with rootless subuid/subgid allocations"
			}
			report.Checks = append(report.Checks, CheckResult{
				Category: "Containers",
				Name:     "Podman",
				Status:   StatusPass,
				Message:  msg,
			})
		}
		if host.DistroboxAvailable {
			report.Checks = append(report.Checks, CheckResult{
				Category: "Containers",
				Name:     "Distrobox",
				Status:   StatusPass,
				Message:  "Distrobox available for guest container workflows",
			})
		}
	}

	// 8. Engine probes
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

	// 9. State storage directory permissions
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
