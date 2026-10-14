package hostinfo

import (
	"bufio"
	"os"
	"os/user"
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
		AudioServer:   "none",
	}

	if hp.OS == "linux" {
		p.profileLinux(hp)
	}

	return hp, nil
}

func (p *Profiler) profileLinux(hp *domain.HostProfile) {
	// Parse /etc/os-release
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "ID=") {
				hp.Distro = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
			} else if strings.HasPrefix(line, "VERSION_ID=") {
				hp.DistroVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
			}
		}
	}

	// Read Kernel Version
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		hp.KernelVersion = strings.TrimSpace(string(data))
	}

	// Detect Display Server
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	x11Display := os.Getenv("DISPLAY")

	if sessionType == "wayland" || waylandDisplay != "" {
		hp.DisplayServer = "wayland"
	} else if sessionType == "x11" || x11Display != "" {
		hp.DisplayServer = "x11"
	}

	// Detect Audio Server
	if _, err := os.Stat(os.Getenv("XDG_RUNTIME_DIR") + "/pipewire-0"); err == nil {
		hp.AudioServer = "pipewire"
	} else if _, err := os.Stat(os.Getenv("XDG_RUNTIME_DIR") + "/pulse/native"); err == nil {
		hp.AudioServer = "pulseaudio"
	}

	// Detect FUSE support
	if _, err := os.Stat("/dev/fuse"); err == nil {
		hp.FUSESupport = true
	}

	// Detect subuid/subgid mapping for rootless containers
	currentUser, err := user.Current()
	if err == nil {
		hp.HasVirtualAlloc = hasSubIDMapping("/etc/subuid", currentUser.Username) &&
			hasSubIDMapping("/etc/subgid", currentUser.Username)
		hp.RootlessPodman = hp.HasVirtualAlloc
	}
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
