package hostinfo

import (
	"bufio"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// Profiler gathers real-time host operating system and capability attributes
type Profiler struct{}

// New creates a new host profiler
func New() *Profiler {
	return &Profiler{}
}

// Current returns the evaluated HostProfile of the current system
func (p *Profiler) Current() (*domain.HostProfile, error) {
	hp := &domain.HostProfile{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		DisplayServer: "headless",
		SessionType:   "headless",
		AudioServer:   "none",
		DistroFamily:  "unknown",
		SELinuxMode:   "disabled",
		AppArmorState: "disabled",
	}

	if hp.OS == "linux" {
		p.profileLinux(hp)
	}

	return hp, nil
}

func (p *Profiler) profileLinux(hp *domain.HostProfile) {
	var id, idLike string
	// Parse /etc/os-release
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "ID=") {
				id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
				hp.Distro = id
			} else if strings.HasPrefix(line, "ID_LIKE=") {
				idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`)
			} else if strings.HasPrefix(line, "VERSION_ID=") {
				hp.DistroVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
			}
		}
	}

	// Classify Distro Family
	familyBlob := strings.ToLower(id + " " + idLike)
	if strings.Contains(familyBlob, "debian") || strings.Contains(familyBlob, "ubuntu") || strings.Contains(familyBlob, "mint") {
		hp.DistroFamily = "debian"
	} else if strings.Contains(familyBlob, "fedora") || strings.Contains(familyBlob, "rhel") || strings.Contains(familyBlob, "centos") || strings.Contains(familyBlob, "rocky") || strings.Contains(familyBlob, "alma") {
		hp.DistroFamily = "fedora"
	} else if strings.Contains(familyBlob, "arch") || strings.Contains(familyBlob, "manjaro") {
		hp.DistroFamily = "arch"
	} else if strings.Contains(familyBlob, "suse") || strings.Contains(familyBlob, "opensuse") {
		hp.DistroFamily = "suse"
	} else if id != "" {
		hp.DistroFamily = id
	}

	// Read Kernel Version
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		hp.KernelVersion = strings.TrimSpace(string(data))
	}

	// Detect Display Server & Session Type
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	x11Display := os.Getenv("DISPLAY")

	if sessionType == "wayland" || waylandDisplay != "" {
		hp.DisplayServer = "wayland"
		hp.SessionType = "wayland"
	} else if sessionType == "x11" || x11Display != "" {
		hp.DisplayServer = "x11"
		hp.SessionType = "x11"
	} else if sessionType != "" {
		hp.SessionType = sessionType
	}

	// Detect Compositor
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if desktop == "" {
		desktop = os.Getenv("DESKTOP_SESSION")
	}
	hp.Compositor = desktop

	// Detect GPU Driver
	hp.GPUDriver = detectGPUDriver()

	// Detect Audio Server
	if _, err := os.Stat(os.Getenv("XDG_RUNTIME_DIR") + "/pipewire-0"); err == nil {
		hp.AudioServer = "pipewire"
	} else if _, err := os.Stat(os.Getenv("XDG_RUNTIME_DIR") + "/pulse/native"); err == nil {
		hp.AudioServer = "pulseaudio"
	}

	// Detect SELinux Mode
	hp.SELinuxMode = detectSELinuxMode()

	// Detect AppArmor State
	hp.AppArmorState = detectAppArmorState()

	// Detect FUSE support (/dev/fuse)
	if _, err := os.Stat("/dev/fuse"); err == nil {
		hp.FUSESupport = true
	}

	// Detect libfuse.so.2 availability
	hp.Libfuse2Available = detectLibfuse2()

	// Detect user namespaces
	hp.UserNamespaces = detectUserNamespaces()

	// Detect noexec mounts
	hp.NoexecMounts = detectNoexecMounts()

	// Detect Container Tools
	if _, err := exec.LookPath("podman"); err == nil {
		hp.PodmanAvailable = true
	}
	if _, err := exec.LookPath("distrobox"); err == nil {
		hp.DistroboxAvailable = true
	}

	// Detect available package managers
	managers := []string{"dnf", "apt", "pacman", "zypper", "flatpak", "snap"}
	for _, m := range managers {
		if _, err := exec.LookPath(m); err == nil {
			hp.PackageManagers = append(hp.PackageManagers, m)
		}
	}

	// Detect subuid/subgid mapping for rootless containers
	currentUser, err := user.Current()
	if err == nil {
		hp.HasVirtualAlloc = hasSubIDMapping("/etc/subuid", currentUser.Username) &&
			hasSubIDMapping("/etc/subgid", currentUser.Username)
		hp.RootlessPodman = hp.HasVirtualAlloc && hp.PodmanAvailable
	}
}

func detectGPUDriver() string {
	// Inspect /sys/class/drm/card*/device/driver
	entries, err := filepath.Glob("/sys/class/drm/card*/device/driver")
	if err == nil && len(entries) > 0 {
		target, err := filepath.EvalSymlinks(entries[0])
		if err == nil {
			return filepath.Base(target)
		}
	}
	if _, err := os.Stat("/proc/driver/nvidia"); err == nil {
		return "nvidia"
	}
	return "generic"
}

func detectSELinuxMode() string {
	if data, err := os.ReadFile("/sys/fs/selinux/enforce"); err == nil {
		if strings.TrimSpace(string(data)) == "1" {
			return "enforcing"
		}
		return "permissive"
	}
	if f, err := os.Open("/etc/selinux/config"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "SELINUX=") {
				val := strings.ToLower(strings.Trim(strings.TrimPrefix(line, "SELINUX="), `"`))
				if val != "" {
					return val
				}
			}
		}
	}
	return "disabled"
}

func detectAppArmorState() string {
	if data, err := os.ReadFile("/sys/module/apparmor/parameters/enabled"); err == nil {
		if strings.TrimSpace(string(data)) == "Y" {
			return "enabled"
		}
	}
	return "disabled"
}

func detectLibfuse2() bool {
	candidates := []string{
		"/lib64/libfuse.so.2",
		"/usr/lib64/libfuse.so.2",
		"/usr/lib/x86_64-linux-gnu/libfuse.so.2",
		"/usr/lib/aarch64-linux-gnu/libfuse.so.2",
		"/usr/lib/libfuse.so.2",
		"/lib/libfuse.so.2",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

func detectUserNamespaces() bool {
	// If unprivileged_userns_clone exists and is 0, disabled
	if data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		return strings.TrimSpace(string(data)) != "0"
	}
	// Modern kernels default to enabled
	return true
}

func detectNoexecMounts() []string {
	var noexecList []string
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return noexecList
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			mountPoint := fields[1]
			options := strings.Split(fields[3], ",")
			for _, opt := range options {
				if opt == "noexec" {
					// Check relevant paths
					if mountPoint == "/tmp" || mountPoint == "/var/tmp" || mountPoint == "/home" || strings.HasPrefix(mountPoint, "/home/") {
						noexecList = append(noexecList, mountPoint)
					}
					break
				}
			}
		}
	}
	return noexecList
}

func hasSubIDMapping(subFile string, username string) bool {
	f, err := os.Open(subFile)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 3 && parts[0] == username {
			return true
		}
	}
	return false
}
