package lab

import (
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/patterns"
)

// LabTier defines the fidelity tier of a test execution
type LabTier string

const (
	TierT0Static    LabTier = "T0" // Static inspection only (no execution)
	TierT1Container LabTier = "T1" // Rootless container execution
	TierT2VM        LabTier = "T2" // Virtual Machine (Lima / QEMU)
	TierT3Remote    LabTier = "T3" // Bare-metal / remote host
)

// LabVerdict defines the final outcome of a target scenario test
type LabVerdict string

const (
	VerdictPass            LabVerdict = "pass"
	VerdictFail            LabVerdict = "fail"
	VerdictInconclusive    LabVerdict = "inconclusive"
	VerdictNotTestableHere LabVerdict = "not-testable-at-this-tier"
)

// ConfidenceLevel records the certainty of an identified cause
type ConfidenceLevel string

const (
	ConfidenceConfirmed  ConfidenceLevel = "confirmed"
	ConfidenceLikely     ConfidenceLevel = "likely"
	ConfidenceHypothesis ConfidenceLevel = "hypothesis"
)

// TargetDefinition defines a Linux target environment
type TargetDefinition struct {
	ID             string            `json:"id" yaml:"id"`                       // e.g. "fedora:42"
	DistroFamily   string            `json:"distro_family" yaml:"distro_family"` // "fedora", "debian", "arch", "suse"
	Image          string            `json:"image" yaml:"image"`                 // pinned container image e.g. "quay.io/fedora/fedora:42"
	ImageDigest    string            `json:"image_digest" yaml:"image_digest"`   // SHA256 of image
	GlibcVersion   string            `json:"glibc_version" yaml:"glibc_version"` // e.g. "2.41"
	PackageManager string            `json:"package_manager" yaml:"package_manager"`
	DefaultUser    string            `json:"default_user" yaml:"default_user"`     // non-root username e.g. "developer"
	Capabilities   []string          `json:"capabilities" yaml:"capabilities"`     // supported scenario IDs at T1
	KnownPackages  map[string]string `json:"known_packages" yaml:"known_packages"` // library name -> package name
}

// ScenarioDefinition defines a test scenario
type ScenarioDefinition struct {
	ID           string   `json:"id" yaml:"id"` // e.g. "smoke", "window"
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	RequiredTier LabTier  `json:"required_tier" yaml:"required_tier"` // minimum tier required
	TimeoutSec   int      `json:"timeout_sec" yaml:"timeout_sec"`
	Network      bool     `json:"network" yaml:"network"` // false by default
	Command      []string `json:"command,omitempty" yaml:"command,omitempty"`
}

// TargetResult records the execution outcome for one (target x scenario) pair
type TargetResult struct {
	TargetID         string               `json:"target_id"`
	ScenarioID       string               `json:"scenario_id"`
	Tier             LabTier              `json:"tier"`
	Verdict          LabVerdict           `json:"verdict"`
	Confidence       ConfidenceLevel      `json:"confidence,omitempty"`
	Cause            string               `json:"cause,omitempty"`
	MatchedPatternID string               `json:"matched_pattern_id,omitempty"`
	ProposedFixes    []patterns.FixOption `json:"proposed_fixes,omitempty"`
	EvidenceRefs     []string             `json:"evidence_refs,omitempty"`
	DurationMs       int64                `json:"duration_ms"`
	ExitCode         int                  `json:"exit_code"`
	RedactedLogRef   string               `json:"redacted_log_ref,omitempty"`
	ScreenshotRef    string               `json:"screenshot_ref,omitempty"`
	ReproScriptRef   string               `json:"repro_script_ref,omitempty"`
	UntestableReason string               `json:"untestable_reason,omitempty"`
}

// AppProfile details application framework and version properties
type AppProfile struct {
	Name            string   `json:"name"`
	Format          string   `json:"format"`
	Architecture    string   `json:"architecture"`
	Framework       string   `json:"framework"` // e.g. "electron", "qt", "gtk", "native"
	ElectronVersion string   `json:"electron_version,omitempty"`
	ChromiumVersion string   `json:"chromium_version,omitempty"`
	NodeVersion     string   `json:"node_version,omitempty"`
	NativeAddons    []string `json:"native_addons,omitempty"`
	Entrypoint      string   `json:"entrypoint,omitempty"`
}

// StaticScanResult details static ELF analysis against target definitions
type StaticScanResult struct {
	Architecture         string                       `json:"architecture"`
	ELFInterpreter       string                       `json:"elf_interpreter,omitempty"`
	RequiredGlibcVersion string                       `json:"required_glibc_version,omitempty"`
	GlibcCompatibility   map[string]bool              `json:"glibc_compatibility"` // targetID -> compatible
	NeededSharedLibs     []string                     `json:"needed_shared_libs"`
	MissingLibsByTarget  map[string][]string          `json:"missing_libs_by_target"`
	PackageSuggestions   map[string]map[string]string `json:"package_suggestions"` // targetID -> (lib -> pkg)
	AppProfile           AppProfile                   `json:"app_profile"`
}

// LabReport records the full matrix and context bundle metadata
type LabReport struct {
	RunID               string            `json:"run_id"`
	Timestamp           time.Time         `json:"timestamp"`
	ArtifactPath        string            `json:"artifact_path"`
	ArtifactSHA256      string            `json:"artifact_sha256"`
	ArtifactSize        int64             `json:"artifact_size"`
	RuntimeID           string            `json:"runtime_id"` // e.g. "podman", "docker"
	IsEmulated          bool              `json:"is_emulated"`
	Targets             []string          `json:"targets"`
	Scenarios           []string          `json:"scenarios"`
	Results             []TargetResult    `json:"results"`
	StaticScan          *StaticScanResult `json:"static_scan,omitempty"`
	Summary             LabSummary        `json:"summary"`
	FidelityLimitations []string          `json:"fidelity_limitations"`
}

// LabSummary tallies verdicts across all tests
type LabSummary struct {
	TotalScenarios int `json:"total_scenarios"`
	Passed         int `json:"passed"`
	Failed         int `json:"failed"`
	Inconclusive   int `json:"inconclusive"`
	NotTestable    int `json:"not_testable"`
}

// TargetCapabilityLimitations documents what cannot be detected at container tier T1
var T1FidelityLimitations = []string{
	"Native Wayland compositor protocols and XWayland window mapping cannot be fully verified inside headless containers without real DRM GPU display servers.",
	"SELinux enforcing AVC denials and AppArmor security profiles require kernel confinement not available in unprivileged rootless containers.",
	"Direct hardware-accelerated GPU render pipelines (Direct3D/Vulkan/VA-API) require host GPU passthrough or a T2 VM / T3 remote host.",
	"Kernel-level FUSE mount operations inside containers may trigger fallback extraction (--appimage-extract-and-run) rather than real host FUSE behavior.",
}
