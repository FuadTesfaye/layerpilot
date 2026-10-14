package domain

import "time"

// PackageFormat represents supported and unsupported package categories
type PackageFormat string

const (
	FormatDeb        PackageFormat = "deb"
	FormatRPM        PackageFormat = "rpm"
	FormatAppImage   PackageFormat = "appimage"
	FormatWindowsExe PackageFormat = "exe"
	FormatMacOSApp   PackageFormat = "app"
	FormatUnknown    PackageFormat = "unknown"
)

// PackageType indicates if an app is GUI, CLI, or unknown
type PackageType string

const (
	PackageTypeCLI     PackageType = "cli"
	PackageTypeGUI     PackageType = "gui"
	PackageTypeUnknown PackageType = "unknown"
)

// PackageInfo contains safe inspection metadata without executing scripts
type PackageInfo struct {
	Path          string            `json:"path"`
	Format        PackageFormat     `json:"format"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Architecture  string            `json:"architecture"`
	PackageType   PackageType       `json:"package_type"`
	SHA256        string            `json:"sha256"`
	SizeBytes     int64             `json:"size_bytes"`
	Description   string            `json:"description,omitempty"`
	Dependencies  []string          `json:"dependencies,omitempty"`
	BinaryTargets []string          `json:"binary_targets,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// HostProfile details the running machine's environment
type HostProfile struct {
	OS              string `json:"os"`                // e.g. "linux", "windows", "darwin"
	Distro          string `json:"distro"`            // e.g. "fedora", "ubuntu", "arch"
	DistroVersion   string `json:"distro_version"`    // e.g. "44", "24.04"
	Arch            string `json:"arch"`              // e.g. "amd64", "arm64"
	KernelVersion   string `json:"kernel_version"`    // e.g. "6.11.0"
	DisplayServer   string `json:"display_server"`    // "wayland", "x11", "headless"
	AudioServer     string `json:"audio_server"`      // "pipewire", "pulseaudio", "none"
	RootlessPodman  bool   `json:"rootless_podman"`   // true if rootless user namespace works
	FUSESupport     bool   `json:"fuse_support"`      // true if /dev/fuse or fusermount is available
	HasVirtualAlloc bool   `json:"has_virtual_alloc"` // true if subuid/subgid mapped
}

// SupportTier defines the 4-tier compatibility classification
type SupportTier string

const (
	Tier1Supported    SupportTier = "Tier 1 — Supported"
	Tier2Limited      SupportTier = "Tier 2 — Supported with limitations"
	Tier3Experimental SupportTier = "Tier 3 — Experimental"
	Tier4Unsupported  SupportTier = "Tier 4 — Unsupported"
)

// Reason codes for deterministic explanation
const (
	ReasonHostOSSupported          = "HOST_OS_SUPPORTED"
	ReasonHostOSMismatch           = "HOST_OS_MISMATCH"
	ReasonArchMatch                = "ARCHITECTURE_MATCH"
	ReasonArchMismatch             = "ARCHITECTURE_MISMATCH"
	ReasonEngineAvailable          = "ENGINE_AVAILABLE"
	ReasonEngineMissing            = "ENGINE_MISSING"
	ReasonGUIBridgeAvailable       = "GUI_BRIDGE_AVAILABLE"
	ReasonDirectExecutionFeasible  = "DIRECT_EXECUTION_FEASIBLE"
	ReasonPkgDepsUnknown           = "PACKAGE_DEPENDENCIES_UNKNOWN"
	ReasonAppProfileUnverified     = "APP_PROFILE_UNVERIFIED"
	ReasonKernelFeatureUnsupported = "KERNEL_FEATURE_UNSUPPORTED"
	ReasonPrivilegedActionRequired = "PRIVILEGED_ACTION_REQUIRED"
	ReasonFormatUnsupported        = "FORMAT_UNSUPPORTED"
)

// StrategyDecision records the outcome of the deterministic strategy planner
type StrategyDecision struct {
	EngineID        string      `json:"engine_id"`
	SupportTier     SupportTier `json:"support_tier"`
	ReasonCodes     []string    `json:"reason_codes"`
	Limitations     []string    `json:"limitations"`
	RequiredActions []string    `json:"required_actions"`
	EvidenceID      string      `json:"evidence_id,omitempty"`
}

// PlanStep represents a single discrete action in an installation plan
type PlanStep struct {
	Order       int    `json:"order"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HostImpact  string `json:"host_impact"`
}

// InstallPlan defines an explicit, previewable action plan before applying changes
type InstallPlan struct {
	PlanID               string           `json:"plan_id"`
	AppID                string           `json:"app_id"`
	Package              PackageInfo      `json:"package"`
	Decision             StrategyDecision `json:"decision"`
	Steps                []PlanStep       `json:"steps"`
	HostChanges          []string         `json:"host_changes"`
	PrivilegedOperations []string         `json:"privileged_operations"`
	RequiresApproval     bool             `json:"requires_approval"`
	BaseImage            string           `json:"base_image,omitempty"`
}

// LifecycleState tracks application progression and explicit failure modes
type LifecycleState string

const (
	StateDiscovered        LifecycleState = "Discovered"
	StatePlanned           LifecycleState = "Planned"
	StateInstalling        LifecycleState = "Installing"
	StateInstalled         LifecycleState = "Installed"
	StateRunning           LifecycleState = "Running"
	StateStopped           LifecycleState = "Stopped"
	StateUninstalling      LifecycleState = "Uninstalling"
	StateRemoved           LifecycleState = "Removed"
	StateInstallFailed     LifecycleState = "InstallFailed"
	StateLaunchFailed      LifecycleState = "LaunchFailed"
	StateRemovalIncomplete LifecycleState = "RemovalIncomplete"
)

// ResourceOwnership tracks exact filesystem, container, and launcher resources created
type ResourceOwnership struct {
	ID         string    `json:"id"`
	AppID      string    `json:"app_id"`
	Type       string    `json:"type"` // "container", "image", "directory", "desktop_file", "symlink"
	PathOrID   string    `json:"path_or_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// ApplicationRecord represents the persistent SQLite record for an installed application
type ApplicationRecord struct {
	AppID                  string              `json:"app_id"`
	DisplayName            string              `json:"display_name"`
	PackageFormat          PackageFormat       `json:"package_format"`
	PackageHash            string              `json:"package_hash"`
	PackageArch            string              `json:"package_arch"`
	PackageVersion         string              `json:"package_version"`
	EngineID               string              `json:"engine_id"`
	EngineVersion          string              `json:"engine_version"`
	HostOS                 string              `json:"host_os"`
	HostDistro             string              `json:"host_distro"`
	Status                 LifecycleState      `json:"status"`
	LauncherPath           string              `json:"launcher_path,omitempty"`
	BinaryEntrypoint       string              `json:"binary_entrypoint,omitempty"`
	PackageType            PackageType         `json:"package_type"`
	InstalledAt            time.Time           `json:"installed_at"`
	LastLaunchedAt         *time.Time          `json:"last_launched_at,omitempty"`
	CompatibilityProfileID string              `json:"compatibility_profile_id,omitempty"`
	Resources              []ResourceOwnership `json:"resources,omitempty"`
}

// CapabilityReport is returned by an Engine during health and capability probing
type CapabilityReport struct {
	EngineID                string            `json:"engine_id"`
	EngineName              string            `json:"engine_name"`
	Available               bool              `json:"available"`
	Version                 string            `json:"version"`
	SupportedFormats        []PackageFormat   `json:"supported_formats"`
	SupportedArchitectures  []string          `json:"supported_architectures"`
	SupportsGUI             bool              `json:"supports_gui"`
	Rootless                bool              `json:"rootless"`
	MissingPrerequisites    []string          `json:"missing_prerequisites,omitempty"`
	DiagnosticDetails       map[string]string `json:"diagnostic_details,omitempty"`
}
