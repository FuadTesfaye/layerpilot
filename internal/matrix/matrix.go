package matrix

import (
	"fmt"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

// PolicyEngine provides deterministic compatibility evaluation
type PolicyEngine struct{}

// New creates a new PolicyEngine
func New() *PolicyEngine {
	return &PolicyEngine{}
}

// Evaluate determines the exact compatibility tier, recommended engine, reason codes, and limitations
func (m *PolicyEngine) Evaluate(
	pkg *domain.PackageInfo,
	host *domain.HostProfile,
	availableEngines []string,
) *domain.StrategyDecision {
	decision := &domain.StrategyDecision{
		ReasonCodes:     []string{},
		Limitations:     []string{},
		RequiredActions: []string{},
	}

	engineSet := make(map[string]bool)
	for _, e := range availableEngines {
		engineSet[strings.ToLower(e)] = true
	}

	// 1. Architecture compatibility check
	if pkg.Architecture != "" && pkg.Architecture != "all" && !archCompatible(pkg.Architecture, host.Arch) {
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonArchMismatch)
		decision.Limitations = append(decision.Limitations, fmt.Sprintf("Package architecture '%s' is incompatible with host architecture '%s'", pkg.Architecture, host.Arch))
		return decision
	}
	decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonArchMatch)

	// 2. Format and OS matrix evaluation
	switch host.OS {
	case "linux":
		m.evaluateLinuxHost(pkg, host, engineSet, decision)
	case "windows":
		m.evaluateWindowsHost(pkg, host, engineSet, decision)
	case "darwin":
		m.evaluateDarwinHost(pkg, host, engineSet, decision)
	default:
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSMismatch)
		decision.Limitations = append(decision.Limitations, fmt.Sprintf("Host OS '%s' is not supported by any known compatibility strategy", host.OS))
	}

	return decision
}

func (m *PolicyEngine) evaluateLinuxHost(
	pkg *domain.PackageInfo,
	host *domain.HostProfile,
	engines map[string]bool,
	decision *domain.StrategyDecision,
) {
	decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSSupported)

	switch pkg.Format {
	case domain.FormatAppImage:
		decision.EngineID = "appimage"
		if host.FUSESupport && host.Libfuse2Available {
			decision.StrategyID = "S1"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible, domain.ReasonEngineAvailable)
		} else {
			decision.StrategyID = "S2"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible, domain.ReasonEngineAvailable)
			decision.Limitations = append(decision.Limitations, "Host missing FUSE or libfuse2; user-local extraction fallback will be applied")
		}
		if pkg.PackageType == domain.PackageTypeGUI && host.DisplayServer == "headless" {
			decision.SupportTier = domain.Tier2Limited
			decision.Limitations = append(decision.Limitations, "Host is headless; GUI display will be unavailable")
		}

	case domain.FormatTarball:
		decision.EngineID = "tarball"
		decision.StrategyID = "S1"
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)

	case domain.FormatDeb:
		if host.DistroFamily == "debian" && (containsString(host.PackageManagers, "apt") || containsString(host.PackageManagers, "dpkg")) {
			// S3: Native package install on Debian family
			decision.EngineID = "native"
			decision.StrategyID = "S3"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
			decision.SingleRootCommand = []string{"pkexec", "apt-get", "install", "-y", pkg.Path}
			decision.RequiredActions = append(decision.RequiredActions, "Requires single-command root authorization via pkexec/sudo to install package")
		} else {
			// S5: Container fallback on foreign host (e.g. Debian package on Fedora)
			decision.EngineID = "podman"
			decision.StrategyID = "S5"
			if !engines["podman"] {
				decision.SupportTier = domain.Tier4Unsupported
				decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineMissing)
				decision.RequiredActions = append(decision.RequiredActions, "Install podman to run Debian packages in an isolated container")
				return
			}
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineAvailable)
			decision.Limitations = append(decision.Limitations, "Foreign package format running inside managed rootless container to prevent host library pollution")
		}

		if pkg.PackageType == domain.PackageTypeGUI {
			if host.DisplayServer != "headless" {
				decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonGUIBridgeAvailable)
				decision.Limitations = append(decision.Limitations, fmt.Sprintf("Desktop integration active via %s socket bridge", host.DisplayServer))
			} else {
				decision.SupportTier = domain.Tier2Limited
				decision.Limitations = append(decision.Limitations, "Headless host: GUI output will not be displayed")
			}
		}

	case domain.FormatRPM:
		if host.DistroFamily == "fedora" && containsString(host.PackageManagers, "dnf") {
			// S3: Native package install on Fedora family
			decision.EngineID = "native"
			decision.StrategyID = "S3"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
			decision.SingleRootCommand = []string{"pkexec", "dnf", "install", "-y", pkg.Path}
			decision.RequiredActions = append(decision.RequiredActions, "Requires single-command root authorization via pkexec/sudo to install package")
		} else if host.DistroFamily == "suse" && containsString(host.PackageManagers, "zypper") {
			// S3: Native package install on openSUSE
			decision.EngineID = "native"
			decision.StrategyID = "S3"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
			decision.SingleRootCommand = []string{"pkexec", "zypper", "--non-interactive", "install", pkg.Path}
			decision.RequiredActions = append(decision.RequiredActions, "Requires single-command root authorization via pkexec/sudo to install package")
		} else {
			// S5: Container fallback on foreign host (e.g. RPM on Debian/Ubuntu)
			decision.EngineID = "podman"
			decision.StrategyID = "S5"
			if !engines["podman"] {
				decision.SupportTier = domain.Tier4Unsupported
				decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineMissing)
				decision.RequiredActions = append(decision.RequiredActions, "Install podman to manage RPM container environments")
				return
			}
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineAvailable)
			decision.Limitations = append(decision.Limitations, "Foreign package format running inside managed rootless container to prevent host library pollution")
		}

	case domain.FormatArchPkg:
		if host.DistroFamily == "arch" && containsString(host.PackageManagers, "pacman") {
			// S3: Native package install on Arch
			decision.EngineID = "native"
			decision.StrategyID = "S3"
			decision.SupportTier = domain.Tier1Supported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
			decision.SingleRootCommand = []string{"pkexec", "pacman", "-U", "--noconfirm", pkg.Path}
			decision.RequiredActions = append(decision.RequiredActions, "Requires single-command root authorization via pkexec/sudo to install package")
		} else {
			// S5: Container fallback on foreign host
			decision.EngineID = "podman"
			decision.StrategyID = "S5"
			if !engines["podman"] {
				decision.SupportTier = domain.Tier4Unsupported
				decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineMissing)
				decision.RequiredActions = append(decision.RequiredActions, "Install podman to run Arch packages in an isolated container")
				return
			}
			decision.SupportTier = domain.Tier1Supported
			decision.Limitations = append(decision.Limitations, "Foreign package format running inside managed rootless container")
		}

	case domain.FormatFlatpak:
		decision.EngineID = "bundle"
		decision.StrategyID = "S4"
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineAvailable)

	case domain.FormatSnap:
		decision.EngineID = "bundle"
		decision.StrategyID = "S4"
		decision.SupportTier = domain.Tier2Limited
		decision.SingleRootCommand = []string{"pkexec", "snap", "install", "--dangerous", pkg.Path}
		decision.RequiredActions = append(decision.RequiredActions, "Requires single-command root authorization via pkexec/sudo to install snap")

	case domain.FormatSelfExtract:
		decision.StrategyID = "S6"
		decision.SupportTier = domain.Tier3Experimental
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonPrivilegedActionRequired)
		decision.Limitations = append(decision.Limitations, "Self-extracting script installer; execution requires explicit approval and sandbox isolation")
		decision.RequiredActions = append(decision.RequiredActions, "Pass --yes and inspect script header before executing untrusted installer scripts")

	case domain.FormatWindowsExe:
		decision.EngineID = "wine"
		decision.SupportTier = domain.Tier3Experimental
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonAppProfileUnverified)
		decision.Limitations = append(decision.Limitations,
			"Untested Windows application; Wine translation is best-effort",
			"Anti-cheat systems, custom kernel drivers, and DX12 may not function",
		)
		decision.RequiredActions = append(decision.RequiredActions, "Explicit user confirmation required for Tier 3 Experimental execution")

	case domain.FormatMacOSApp:
		if pkg.Framework == "electron" {
			decision.StrategyID = "S_ELECTRON_EXTRACT"
			decision.SupportTier = domain.Tier2Limited
			decision.EngineID = "tarball"
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonArchMatch, domain.ReasonHostOSSupported)
			decision.Limitations = append(decision.Limitations,
				"Extracted cross-platform Electron payload from macOS package",
				"Native Mach-O Node addons may require Linux equivalent binaries",
			)
			decision.RequiredActions = append(decision.RequiredActions, "System Electron or Node runtime required for execution")
		} else {
			decision.StrategyID = "S6"
			decision.SupportTier = domain.Tier4Unsupported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSMismatch, domain.ReasonFormatUnsupported)
			decision.Limitations = append(decision.Limitations, "macOS native applications require Apple Darwin kernel and proprietary Cocoa/Metal frameworks; execution on Linux is unsupported")
		}

	default:
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonFormatUnsupported)
		decision.Limitations = append(decision.Limitations, "Unrecognized package format cannot be executed safely")
	}
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}

func (m *PolicyEngine) evaluateWindowsHost(
	pkg *domain.PackageInfo,
	host *domain.HostProfile,
	engines map[string]bool,
	decision *domain.StrategyDecision,
) {
	switch pkg.Format {
	case domain.FormatDeb, domain.FormatRPM, domain.FormatAppImage:
		decision.EngineID = "wsl2"
		decision.SupportTier = domain.Tier2Limited
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSSupported, domain.ReasonEngineAvailable)
		decision.Limitations = append(decision.Limitations, "Executed inside WSL2 Linux environment; requires WSL2 and WSLg for GUI apps")
	case domain.FormatWindowsExe:
		decision.EngineID = "native"
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
	default:
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonFormatUnsupported)
	}
}

func (m *PolicyEngine) evaluateDarwinHost(
	pkg *domain.PackageInfo,
	host *domain.HostProfile,
	engines map[string]bool,
	decision *domain.StrategyDecision,
) {
	switch pkg.Format {
	case domain.FormatDeb, domain.FormatRPM, domain.FormatAppImage:
		decision.EngineID = "lima"
		decision.SupportTier = domain.Tier2Limited
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSSupported, domain.ReasonEngineAvailable)
		decision.Limitations = append(decision.Limitations, "Executed inside Lima Linux virtual machine; GUI display requires XQuartz or VNC bridge")
	case domain.FormatMacOSApp:
		decision.EngineID = "native"
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible)
	default:
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonFormatUnsupported)
	}
}

func archCompatible(pkgArch, hostArch string) bool {
	if pkgArch == "all" || pkgArch == "noarch" {
		return true
	}
	if pkgArch == hostArch {
		return true
	}
	// 386 can run on amd64
	if pkgArch == "386" && hostArch == "amd64" {
		return true
	}
	return false
}
