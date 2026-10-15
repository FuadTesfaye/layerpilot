package lab

// Standard built-in scenario definitions
func BuiltinScenarios() []ScenarioDefinition {
	return []ScenarioDefinition{
		{
			ID:           "static-scan",
			Name:         "Static Scan & Glibc Check",
			Description:  "Performs T0 static ELF scan, GLIBC symbol verification, and dependency prediction without executing code",
			RequiredTier: TierT0Static,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "smoke",
			Name:         "Smoke Launch Test",
			Description:  "Launches artifact as unprivileged user, verifies process remains alive for 10s, and captures exit code and stderr",
			RequiredTier: TierT1Container,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "libs",
			Name:         "Dynamic Loader & Library Audit",
			Description:  "Executes ldd and dynamic linker trace to verify all required runtime shared libraries resolve cleanly",
			RequiredTier: TierT1Container,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "offline",
			Name:         "Offline Resiliency Test",
			Description:  "Executes application with network disabled to verify updater or analytics failures are harmless noise rather than fatal crashes",
			RequiredTier: TierT1Container,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "fresh-home",
			Name:         "Clean Home Directory Test",
			Description:  "Executes in a pristine temporary user home directory to ensure application initializes required configs without crashing",
			RequiredTier: TierT1Container,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "window",
			Name:         "X11/Xvfb Window Mapping Test",
			Description:  "Launches under headless Xvfb virtual framebuffer and verifies window mapping with screenshot capture",
			RequiredTier: TierT1Container,
			TimeoutSec:   45,
			Network:      false,
		},
		{
			ID:           "fuse",
			Name:         "FUSE Mount Fallback Test",
			Description:  "Verifies AppImage behavior when FUSE is unavailable, testing --appimage-extract-and-run fallback",
			RequiredTier: TierT1Container,
			TimeoutSec:   30,
			Network:      false,
		},
		{
			ID:           "install-clean",
			Name:         "Clean Package Installation",
			Description:  "Tests package manager installation (.deb on Debian/Ubuntu, .rpm on Fedora) in pristine container",
			RequiredTier: TierT1Container,
			TimeoutSec:   60,
			Network:      false,
		},
	}
}

// FindScenario locates a scenario definition by ID
func FindScenario(scenarioID string) *ScenarioDefinition {
	for _, s := range BuiltinScenarios() {
		if s.ID == scenarioID {
			copyS := s
			return &copyS
		}
	}
	return nil
}

// FilterScenarios resolves a comma-separated list of scenario IDs or defaults
func FilterScenarios(scenarioIDs []string) []ScenarioDefinition {
	if len(scenarioIDs) == 0 {
		return []ScenarioDefinition{
			*FindScenario("static-scan"),
			*FindScenario("smoke"),
			*FindScenario("libs"),
			*FindScenario("offline"),
			*FindScenario("window"),
		}
	}

	var res []ScenarioDefinition
	for _, id := range scenarioIDs {
		if s := FindScenario(id); s != nil {
			res = append(res, *s)
		}
	}
	return res
}
