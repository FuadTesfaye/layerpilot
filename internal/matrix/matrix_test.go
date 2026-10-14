package matrix

import (
	"testing"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func TestEvaluateDebianOnFedora(t *testing.T) {
	pe := New()
	pkg := &domain.PackageInfo{
		Format:       domain.FormatDeb,
		Architecture: "amd64",
		PackageType:  domain.PackageTypeCLI,
	}
	host := &domain.HostProfile{
		OS:            "linux",
		Distro:        "fedora",
		Arch:          "amd64",
		DisplayServer: "wayland",
	}

	// 1. When podman is available
	dec := pe.Evaluate(pkg, host, []string{"podman"})
	if dec.SupportTier != domain.Tier1Supported {
		t.Fatalf("expected Tier 1, got %s", dec.SupportTier)
	}
	if dec.EngineID != "podman" {
		t.Fatalf("expected engine podman, got %s", dec.EngineID)
	}

	// 2. When podman is missing
	decMissing := pe.Evaluate(pkg, host, []string{})
	if decMissing.SupportTier != domain.Tier4Unsupported {
		t.Fatalf("expected Tier 4 when engine missing, got %s", decMissing.SupportTier)
	}
}

func TestEvaluateArchMismatch(t *testing.T) {
	pe := New()
	pkg := &domain.PackageInfo{
		Format:       domain.FormatDeb,
		Architecture: "arm64",
	}
	host := &domain.HostProfile{
		OS:   "linux",
		Arch: "amd64",
	}

	dec := pe.Evaluate(pkg, host, []string{"podman"})
	if dec.SupportTier != domain.Tier4Unsupported {
		t.Fatalf("expected Tier 4 on architecture mismatch, got %s", dec.SupportTier)
	}
}

func TestEvaluateMacOSRefusal(t *testing.T) {
	pe := New()
	pkg := &domain.PackageInfo{
		Format:       domain.FormatMacOSApp,
		Architecture: "amd64",
	}
	host := &domain.HostProfile{
		OS:   "linux",
		Arch: "amd64",
	}

	dec := pe.Evaluate(pkg, host, []string{"podman"})
	if dec.SupportTier != domain.Tier4Unsupported {
		t.Fatalf("expected Tier 4 for macOS on Linux, got %s", dec.SupportTier)
	}
}

func TestEvaluateWineExperimental(t *testing.T) {
	pe := New()
	pkg := &domain.PackageInfo{
		Format:       domain.FormatWindowsExe,
		Architecture: "amd64",
	}
	host := &domain.HostProfile{
		OS:   "linux",
		Arch: "amd64",
	}

	dec := pe.Evaluate(pkg, host, []string{"wine"})
	if dec.SupportTier != domain.Tier3Experimental {
		t.Fatalf("expected Tier 3 Experimental for Wine, got %s", dec.SupportTier)
	}
}
