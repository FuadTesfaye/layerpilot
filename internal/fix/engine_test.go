package fix

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/patterns"
	"github.com/FuadTesfaye/layerpilot/internal/state"
)

type mockVerifier struct {
	success bool
	message string
}

func (m *mockVerifier) Verify(ctx context.Context, app *domain.ApplicationRecord, launcherPath string) (*VerificationResult, error) {
	return &VerificationResult{
		Success:         m.success,
		Uptime:          10 * time.Second,
		WindowDetected:  m.success,
		WindowProbeUsed: "mock-probe",
		Message:         m.message,
	}, nil
}

func TestModifyWrapperContent(t *testing.T) {
	orig := `#!/bin/sh
APP_DIR="$HOME/.local/share/compat/apps/test-app"
BIN="$APP_DIR/app.AppImage"
FLAGS=""
if [ -n "$WAYLAND_DISPLAY" ]; then
    FLAGS="--ozone-platform-hint=auto"
fi

exec "$BIN" $FLAGS "$@"
`
	fix1 := patterns.FixOption{
		ID:          "electron-wayland-no-window-1",
		Description: "Run with X11 backend",
		Action:      "append_flags",
		Flags:       []string{"--ozone-platform=x11"},
		Env: map[string]string{
			"ELECTRON_OZONE_PLATFORM_HINT": "auto",
		},
	}

	modified := ModifyWrapperContent(orig, fix1)
	if !strings.Contains(modified, "# COMPAT_FIX_BEGIN: electron-wayland-no-window-1") {
		t.Fatalf("expected begin comment, got:\n%s", modified)
	}
	if !strings.Contains(modified, `FLAGS="--ozone-platform=x11"`) {
		t.Fatalf("expected FLAGS override, got:\n%s", modified)
	}
	if !strings.Contains(modified, `export ELECTRON_OZONE_PLATFORM_HINT="auto"`) {
		t.Fatalf("expected export env, got:\n%s", modified)
	}

	// Verify idempotency and clean replacement when applying fix2
	fix2 := patterns.FixOption{
		ID:          "electron-wayland-no-window-2",
		Description: "Run with X11 backend and disabled GPU",
		Action:      "append_flags",
		Flags:       []string{"--ozone-platform=x11", "--disable-gpu"},
	}

	modified2 := ModifyWrapperContent(modified, fix2)
	if strings.Contains(modified2, "electron-wayland-no-window-1") {
		t.Fatalf("expected fix1 to be stripped, got:\n%s", modified2)
	}
	if !strings.Contains(modified2, "# COMPAT_FIX_BEGIN: electron-wayland-no-window-2") {
		t.Fatalf("expected begin comment for fix2, got:\n%s", modified2)
	}
	if !strings.Contains(modified2, `FLAGS="--ozone-platform=x11 --disable-gpu"`) {
		t.Fatalf("expected updated FLAGS, got:\n%s", modified2)
	}
}

func TestEngine_ApplyFix_And_Undo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compat-fix-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "state.db")
	store, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	launcherDir := filepath.Join(tmpDir, "bin")
	_ = os.MkdirAll(launcherDir, 0755)
	launcherPath := filepath.Join(launcherDir, "test-app")
	originalScript := "#!/bin/sh\nexec \"$BIN\" \"$@\"\n"
	if err := os.WriteFile(launcherPath, []byte(originalScript), 0755); err != nil {
		t.Fatal(err)
	}

	appRecord := &domain.ApplicationRecord{
		AppID:         "test-app",
		DisplayName:   "Test App",
		PackageFormat: domain.FormatAppImage,
		PackageType:   domain.PackageTypeGUI,
		Status:        domain.StateInstalled,
		LauncherPath:  launcherPath,
		InstalledAt:   time.Now().UTC(),
	}
	if err := store.Save(appRecord); err != nil {
		t.Fatal(err)
	}

	mockV := &mockVerifier{success: true, message: "mock verification passed"}
	engine, err := NewEngine(store, mockV)
	if err != nil {
		t.Fatal(err)
	}
	engine.backupDir = filepath.Join(tmpDir, "backups")
	engine.logsDir = filepath.Join(tmpDir, "logs")

	fixOpt := patterns.FixOption{
		ID:          "electron-wayland-no-window-1",
		Description: "Run with X11 backend",
		Action:      "append_flags",
		Flags:       []string{"--ozone-platform=x11"},
	}

	// 1. Apply Fix
	ctx := context.Background()
	res, err := engine.ApplyFix(ctx, "test-app", fixOpt)
	if err != nil {
		t.Fatalf("ApplyFix failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success true, got false: %s", res.Message)
	}

	// Verify wrapper was modified
	curScript, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(curScript), `FLAGS="--ozone-platform=x11"`) {
		t.Fatalf("expected launcher to be updated, got:\n%s", string(curScript))
	}

	// Verify recorded in store
	fixes, err := store.GetActiveFixes("test-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 {
		t.Fatalf("expected 1 active fix, got %d", len(fixes))
	}

	// 2. Undo Fix
	undoRes, err := engine.UndoFix(ctx, "test-app")
	if err != nil {
		t.Fatalf("UndoFix failed: %v", err)
	}
	if !undoRes.Success {
		t.Fatalf("expected undo success, got false: %s", undoRes.Message)
	}

	// Verify wrapper restored to original content
	restoredScript, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredScript) != originalScript {
		t.Fatalf("expected wrapper to be restored to original, got:\n%s", string(restoredScript))
	}

	// Verify active fixes is now 0
	activeFixesAfterUndo, err := store.GetActiveFixes("test-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(activeFixesAfterUndo) != 0 {
		t.Fatalf("expected 0 active fixes after undo, got %d", len(activeFixesAfterUndo))
	}
}

func TestEngine_ApplyFix_VerificationFailure_Rollback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compat-fix-fail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "state.db")
	store, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	launcherDir := filepath.Join(tmpDir, "bin")
	_ = os.MkdirAll(launcherDir, 0755)
	launcherPath := filepath.Join(launcherDir, "test-app")
	originalScript := "#!/bin/sh\nexec \"$BIN\" \"$@\"\n"
	if err := os.WriteFile(launcherPath, []byte(originalScript), 0755); err != nil {
		t.Fatal(err)
	}

	appRecord := &domain.ApplicationRecord{
		AppID:         "test-app",
		DisplayName:   "Test App",
		PackageFormat: domain.FormatAppImage,
		PackageType:   domain.PackageTypeGUI,
		Status:        domain.StateInstalled,
		LauncherPath:  launcherPath,
		InstalledAt:   time.Now().UTC(),
	}
	if err := store.Save(appRecord); err != nil {
		t.Fatal(err)
	}

	mockV := &mockVerifier{success: false, message: "process crashed after 2s"}
	engine, err := NewEngine(store, mockV)
	if err != nil {
		t.Fatal(err)
	}
	engine.backupDir = filepath.Join(tmpDir, "backups")
	engine.logsDir = filepath.Join(tmpDir, "logs")

	fixOpt := patterns.FixOption{
		ID:          "electron-wayland-no-window-1",
		Description: "Run with X11 backend",
		Action:      "append_flags",
		Flags:       []string{"--ozone-platform=x11"},
	}

	res, err := engine.ApplyFix(context.Background(), "test-app", fixOpt)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if res.Success {
		t.Fatalf("expected success false on verification failure")
	}
	if !res.RolledBack {
		t.Fatalf("expected rolled_back to be true on verification failure")
	}

	// Verify wrapper was restored to original
	curScript, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(curScript) != originalScript {
		t.Fatalf("expected wrapper to be restored immediately after failure, got:\n%s", string(curScript))
	}

	// Verify no active fix was saved
	fixes, err := store.GetActiveFixes("test-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 0 {
		t.Fatalf("expected 0 active fixes, got %d", len(fixes))
	}
}
