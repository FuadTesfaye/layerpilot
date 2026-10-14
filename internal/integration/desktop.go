package integration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/runner"
)

// DesktopIntegrator manages XDG standard .desktop launchers on Linux
type DesktopIntegrator struct {
	runner runner.Runner
}

// NewDesktopIntegrator creates a new DesktopIntegrator
func NewDesktopIntegrator(r runner.Runner) *DesktopIntegrator {
	return &DesktopIntegrator{runner: r}
}

// InstallLauncher creates a .desktop file in ~/.local/share/applications/
func (d *DesktopIntegrator) InstallLauncher(app *domain.ApplicationRecord) (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	appDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	launcherPath := filepath.Join(appDir, fmt.Sprintf("compat-%s.desktop", app.AppID))

	terminal := "false"
	if app.PackageType == domain.PackageTypeCLI {
		terminal = "true"
	}

	content := fmt.Sprintf(`[Desktop Entry]
Version=1.0
Type=Application
Name=%s
Comment=Managed by compat application manager
Exec=%s %%U
Terminal=%s
Categories=Utility;
`, app.DisplayName, app.LauncherPath, terminal)

	if err := os.WriteFile(launcherPath, []byte(content), 0644); err != nil {
		return "", err
	}

	// Update desktop database if available
	_, _ = d.runner.Run(nil, "update-desktop-database", appDir)

	return launcherPath, nil
}

// RemoveLauncher removes the .desktop file
func (d *DesktopIntegrator) RemoveLauncher(appID string) error {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	launcherPath := filepath.Join(home, ".local", "share", "applications", fmt.Sprintf("compat-%s.desktop", appID))
	_ = os.Remove(launcherPath)
	return nil
}
