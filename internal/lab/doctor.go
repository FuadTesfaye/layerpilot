package lab

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// RuntimeID identifies a container or VM backend
type RuntimeID string

const (
	RuntimePodman RuntimeID = "podman"
	RuntimeDocker RuntimeID = "docker"
	RuntimeLima   RuntimeID = "lima"
	RuntimeWSL2   RuntimeID = "wsl2"
	RuntimeNone   RuntimeID = "none"
)

// RuntimeStatus details availability and capabilities of a runtime
type RuntimeStatus struct {
	ID                  RuntimeID `json:"id"`
	Name                string    `json:"name"`
	Available           bool      `json:"available"`
	Version             string    `json:"version,omitempty"`
	IsRootless          bool      `json:"is_rootless"`
	VirtualizationReady bool      `json:"virtualization_ready"`
	Details             string    `json:"details,omitempty"`
	InstallInstructions string    `json:"install_instructions,omitempty"`
}

// LabDoctorReport summarizes host capabilities, virtualization, and runtime availability
type LabDoctorReport struct {
	HostOS             string          `json:"host_os"`
	HostArch           string          `json:"host_arch"`
	Virtualization     bool            `json:"virtualization"`
	FreeDiskBytes      uint64          `json:"free_disk_bytes"`
	FreeRAMBytes       uint64          `json:"free_ram_bytes"`
	RecommendedRuntime RuntimeID       `json:"recommended_runtime"`
	Runtimes           []RuntimeStatus `json:"runtimes"`
	EmulationWarning   string          `json:"emulation_warning,omitempty"`
}

// Executor abstracts command execution to enable clean unit testing with mocked outputs
type Executor interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	LookPath(file string) (string, error)
}

// OSExecutor executes real host commands
type OSExecutor struct{}

func (e *OSExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (e *OSExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Doctor inspects system runtimes and host resources
type Doctor struct {
	exec     Executor
	hostOS   string
	hostArch string
}

// NewDoctor creates a doctor initialized for the current host
func NewDoctor() *Doctor {
	return &Doctor{
		exec:     &OSExecutor{},
		hostOS:   runtime.GOOS,
		hostArch: runtime.GOARCH,
	}
}

// NewCustomDoctor allows injecting custom OS, arch, and mock executor
func NewCustomDoctor(exec Executor, hostOS, hostArch string) *Doctor {
	return &Doctor{
		exec:     exec,
		hostOS:   hostOS,
		hostArch: hostArch,
	}
}

// Diagnose evaluates all runtimes and system resources
func (d *Doctor) Diagnose(ctx context.Context) *LabDoctorReport {
	rep := &LabDoctorReport{
		HostOS:   d.hostOS,
		HostArch: d.hostArch,
		Runtimes: make([]RuntimeStatus, 0),
	}

	d.checkResources(rep)
	d.checkPodman(ctx, rep)
	d.checkDocker(ctx, rep)
	d.checkLima(ctx, rep)
	d.checkWSL2(ctx, rep)

	// Preference order: podman > docker > lima > wsl2
	rep.RecommendedRuntime = RuntimeNone
	for _, preferred := range []RuntimeID{RuntimePodman, RuntimeDocker, RuntimeLima, RuntimeWSL2} {
		for _, r := range rep.Runtimes {
			if r.ID == preferred && r.Available {
				rep.RecommendedRuntime = r.ID
				break
			}
		}
		if rep.RecommendedRuntime != RuntimeNone {
			break
		}
	}

	return rep
}

func (d *Doctor) checkResources(rep *LabDoctorReport) {
	// Disk space check on user home
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}

	rep.FreeDiskBytes = getFreeDiskSpace(home)

	// RAM check on Linux
	if d.hostOS == "linux" {
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemAvailable:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
							rep.FreeRAMBytes = kb * 1024
						}
					}
					break
				}
			}
		}
	}

	// Check KVM / virtualization on Linux
	if d.hostOS == "linux" {
		if _, err := os.Stat("/dev/kvm"); err == nil {
			rep.Virtualization = true
		}
	} else if d.hostOS == "darwin" || d.hostOS == "windows" {
		rep.Virtualization = true
	}
}

func (d *Doctor) checkPodman(ctx context.Context, rep *LabDoctorReport) {
	st := RuntimeStatus{
		ID:                  RuntimePodman,
		Name:                "Podman (Rootless)",
		InstallInstructions: d.podmanInstallInstructions(),
	}

	if _, err := d.exec.LookPath("podman"); err == nil {
		out, err := d.exec.Run(ctx, "podman", "version", "--format", "{{.Version}}")
		if err == nil {
			st.Available = true
			st.Version = out
			st.IsRootless = true
			st.Details = fmt.Sprintf("Podman CLI active (version %s)", out)

			// On mac/win, check podman machine
			if d.hostOS == "darwin" || d.hostOS == "windows" {
				mOut, mErr := d.exec.Run(ctx, "podman", "machine", "list", "--format", "{{.Running}}")
				if mErr == nil && strings.Contains(mOut, "true") {
					st.Details += "; podman machine is running"
				} else {
					st.Details += "; podman machine is NOT running (run 'podman machine start')"
				}
			}
		}
	}

	rep.Runtimes = append(rep.Runtimes, st)
}

func (d *Doctor) checkDocker(ctx context.Context, rep *LabDoctorReport) {
	st := RuntimeStatus{
		ID:                  RuntimeDocker,
		Name:                "Docker Engine / Desktop",
		InstallInstructions: d.dockerInstallInstructions(),
	}

	if _, err := d.exec.LookPath("docker"); err == nil {
		out, err := d.exec.Run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
		if err == nil {
			st.Available = true
			st.Version = out
			st.Details = fmt.Sprintf("Docker daemon reachable (version %s)", out)
		} else {
			st.Details = "Docker CLI installed, but Docker daemon is not running or accessible"
		}
	}

	rep.Runtimes = append(rep.Runtimes, st)
}

func (d *Doctor) checkLima(ctx context.Context, rep *LabDoctorReport) {
	if d.hostOS != "darwin" && d.hostOS != "linux" {
		return
	}

	st := RuntimeStatus{
		ID:                  RuntimeLima,
		Name:                "Lima Linux VM",
		InstallInstructions: "macOS: brew install lima && limactl start",
	}

	if _, err := d.exec.LookPath("limactl"); err == nil {
		out, err := d.exec.Run(ctx, "limactl", "list", "--json")
		if err == nil {
			st.Available = true
			st.VirtualizationReady = true
			st.Details = "Lima VM manager active"
			if strings.Contains(out, `"status":"Running"`) {
				st.Details += "; active Linux VM instance running"
			}
		}
	}

	rep.Runtimes = append(rep.Runtimes, st)
}

func (d *Doctor) checkWSL2(ctx context.Context, rep *LabDoctorReport) {
	if d.hostOS != "windows" {
		return
	}

	st := RuntimeStatus{
		ID:                  RuntimeWSL2,
		Name:                "Windows Subsystem for Linux (WSL2)",
		InstallInstructions: "PowerShell (Admin): wsl --install",
	}

	if _, err := d.exec.LookPath("wsl.exe"); err == nil {
		out, err := d.exec.Run(ctx, "wsl.exe", "-l", "-v")
		if err == nil {
			st.Available = true
			st.VirtualizationReady = true
			st.Details = "WSL2 active with installed Linux distros"
			if strings.Contains(out, "Running") {
				st.Details += "; active distro running"
			}
		}
	}

	rep.Runtimes = append(rep.Runtimes, st)
}

func (d *Doctor) podmanInstallInstructions() string {
	switch d.hostOS {
	case "darwin":
		return "macOS: brew install podman && podman machine init && podman machine start"
	case "windows":
		return "Windows: winget install RedHat.Podman-Desktop (or 'podman machine init' in PowerShell)"
	case "linux":
		return "Linux (Fedora/RHEL): sudo dnf install -y podman\nLinux (Ubuntu/Debian): sudo apt update && sudo apt install -y podman"
	default:
		return "Visit https://podman.io/getting-started/installation"
	}
}

func (d *Doctor) dockerInstallInstructions() string {
	switch d.hostOS {
	case "darwin":
		return "macOS: Install Docker Desktop (https://docs.docker.com/desktop/setup/install/mac-install/)"
	case "windows":
		return "Windows: Install Docker Desktop with WSL2 backend (https://docs.docker.com/desktop/setup/install/windows-install/)"
	case "linux":
		return "Linux: Follow Docker Engine installation guide (https://docs.docker.com/engine/install/)"
	default:
		return "Visit https://docs.docker.com/get-docker/"
	}
}

// CheckEmulation warns if an arm64 host is testing an amd64 artifact
func CheckEmulation(hostArch, artifactArch string) (bool, string) {
	if (hostArch == "arm64" || hostArch == "aarch64") && (artifactArch == "amd64" || artifactArch == "x86_64") {
		return true, "Artifact is x86_64 running under QEMU user emulation on an arm64 host. Performance may be slower and timing-sensitive scenarios may behave differently."
	}
	if (hostArch == "amd64" || hostArch == "x86_64") && (artifactArch == "arm64" || artifactArch == "aarch64") {
		return true, "Artifact is arm64 running under QEMU user emulation on an x86_64 host."
	}
	return false, ""
}
