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

func TestEvaluateStrategyLadder(t *testing.T) {
	pe := New()

	// S1: AppImage with FUSE and libfuse2
	appimagePkg := &domain.PackageInfo{
		Format:       domain.FormatAppImage,
		Architecture: "amd64",
		Path:         "/tmp/test.AppImage",
	}
	hostFUSE := &domain.HostProfile{
		OS:                "linux",
		Distro:            "fedora",
		Arch:              "amd64",
		FUSESupport:       true,
		Libfuse2Available: true,
	}
	decS1 := pe.Evaluate(appimagePkg, hostFUSE, []string{"appimage"})
	if decS1.StrategyID != "S1" || decS1.EngineID != "appimage" {
		t.Fatalf("expected S1/appimage, got %s/%s", decS1.StrategyID, decS1.EngineID)
	}

	// S2: AppImage missing libfuse2 fallback
	hostNoFuse := &domain.HostProfile{
		OS:                "linux",
		Distro:            "fedora",
		Arch:              "amd64",
		FUSESupport:       false,
		Libfuse2Available: false,
	}
	decS2 := pe.Evaluate(appimagePkg, hostNoFuse, []string{"appimage"})
	if decS2.StrategyID != "S2" {
		t.Fatalf("expected S2 fallback, got %s", decS2.StrategyID)
	}

	// S1: Tarball user-local
	tarballPkg := &domain.PackageInfo{
		Format:       domain.FormatTarball,
		Architecture: "amd64",
	}
	decTar := pe.Evaluate(tarballPkg, hostFUSE, []string{"tarball"})
	if decTar.StrategyID != "S1" || decTar.EngineID != "tarball" {
		t.Fatalf("expected S1/tarball, got %s/%s", decTar.StrategyID, decTar.EngineID)
	}

	// S3: Native RPM on Fedora
	rpmPkg := &domain.PackageInfo{
		Format:       domain.FormatRPM,
		Architecture: "amd64",
		Path:         "/tmp/pkg.rpm",
	}
	hostFedora := &domain.HostProfile{
		OS:              "linux",
		DistroFamily:    "fedora",
		Arch:            "amd64",
		PackageManagers: []string{"dnf", "rpm"},
	}
	decRpmFedora := pe.Evaluate(rpmPkg, hostFedora, []string{"native"})
	if decRpmFedora.StrategyID != "S3" || decRpmFedora.EngineID != "native" {
		t.Fatalf("expected S3/native for RPM on Fedora, got %s/%s", decRpmFedora.StrategyID, decRpmFedora.EngineID)
	}
	if len(decRpmFedora.SingleRootCommand) == 0 || decRpmFedora.SingleRootCommand[0] != "pkexec" {
		t.Fatalf("expected pkexec single root command, got %v", decRpmFedora.SingleRootCommand)
	}

	// S3: Native DEB on Debian
	debPkg := &domain.PackageInfo{
		Format:       domain.FormatDeb,
		Architecture: "amd64",
		Path:         "/tmp/pkg.deb",
	}
	hostDebian := &domain.HostProfile{
		OS:              "linux",
		DistroFamily:    "debian",
		Arch:            "amd64",
		PackageManagers: []string{"apt", "dpkg"},
	}
	decDebDebian := pe.Evaluate(debPkg, hostDebian, []string{"native"})
	if decDebDebian.StrategyID != "S3" || decDebDebian.EngineID != "native" {
		t.Fatalf("expected S3/native for DEB on Debian, got %s/%s", decDebDebian.StrategyID, decDebDebian.EngineID)
	}

	// S4: Flatpak bundle
	flatpakPkg := &domain.PackageInfo{
		Format:       domain.FormatFlatpak,
		Architecture: "amd64",
		Path:         "/tmp/app.flatpak",
	}
	decFlatpak := pe.Evaluate(flatpakPkg, hostFedora, []string{"bundle"})
	if decFlatpak.StrategyID != "S4" || decFlatpak.EngineID != "bundle" {
		t.Fatalf("expected S4/bundle for Flatpak, got %s/%s", decFlatpak.StrategyID, decFlatpak.EngineID)
	}

	// S5: Foreign package container fallback (DEB on Fedora)
	decDebFedora := pe.Evaluate(debPkg, hostFedora, []string{"podman"})
	if decDebFedora.StrategyID != "S5" || decDebFedora.EngineID != "podman" {
		t.Fatalf("expected S5/podman for DEB on Fedora, got %s/%s", decDebFedora.StrategyID, decDebFedora.EngineID)
	}

	// S6: Self-extract script refusal / experimental
	selfExtractPkg := &domain.PackageInfo{
		Format:       domain.FormatSelfExtract,
		Architecture: "amd64",
		Path:         "/tmp/installer.run",
	}
	decSelfExtract := pe.Evaluate(selfExtractPkg, hostFedora, []string{})
	if decSelfExtract.StrategyID != "S6" || decSelfExtract.SupportTier != domain.Tier3Experimental {
		t.Fatalf("expected S6 Tier 3 for self-extracting script, got %s / %s", decSelfExtract.StrategyID, decSelfExtract.SupportTier)
	}
}
