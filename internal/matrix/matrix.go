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
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonDirectExecutionFeasible, domain.ReasonEngineAvailable)
		if !host.FUSESupport {
			decision.Limitations = append(decision.Limitations, "FUSE (/dev/fuse) not detected; fallback extraction may be required")
		}
		if pkg.PackageType == domain.PackageTypeGUI && host.DisplayServer == "headless" {
			decision.SupportTier = domain.Tier2Limited
			decision.Limitations = append(decision.Limitations, "Host is headless; GUI display will be unavailable")
		}

	case domain.FormatDeb:
		// Debian package on Linux
		decision.EngineID = "podman"
		if !engines["podman"] {
			decision.SupportTier = domain.Tier4Unsupported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineMissing)
			decision.RequiredActions = append(decision.RequiredActions, "Install podman to run Debian packages in an isolated container")
			return
		}

		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineAvailable)
		if host.Distro == "fedora" || host.Distro == "rhel" || host.Distro == "arch" || host.Distro == "alpine" {
			// Foreign package on non-Debian host
			decision.SupportTier = domain.Tier1Supported
			decision.Limitations = append(decision.Limitations, "Running inside managed rootless container to prevent host library pollution")
		} else {
			decision.SupportTier = domain.Tier1Supported
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
		decision.EngineID = "podman"
		if !engines["podman"] {
			decision.SupportTier = domain.Tier4Unsupported
			decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineMissing)
			decision.RequiredActions = append(decision.RequiredActions, "Install podman to manage RPM container environments")
			return
		}
		decision.SupportTier = domain.Tier1Supported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonEngineAvailable)

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
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonHostOSMismatch, domain.ReasonFormatUnsupported)
		decision.Limitations = append(decision.Limitations, "macOS applications require Apple Darwin kernel and proprietary frameworks; execution on Linux is unsupported")

	default:
		decision.SupportTier = domain.Tier4Unsupported
		decision.ReasonCodes = append(decision.ReasonCodes, domain.ReasonFormatUnsupported)
		decision.Limitations = append(decision.Limitations, "Unrecognized package format cannot be executed safely")
	}
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
