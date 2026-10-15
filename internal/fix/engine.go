package fix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/patterns"
	"github.com/FuadTesfaye/layerpilot/internal/state"
)

// Diagnosis details detected failure patterns and candidate solutions
type Diagnosis struct {
	AppID         string               `json:"app_id"`
	Pattern       *patterns.Pattern    `json:"pattern,omitempty"`
	MatchedLines  []string             `json:"matched_lines,omitempty"`
	ProposedFixes []patterns.FixOption `json:"proposed_fixes,omitempty"`
	NoiseCount    int                  `json:"noise_count"`
	HasFatal      bool                 `json:"has_fatal"`
	LogPath       string               `json:"log_path,omitempty"`
}

// FixResult represents the outcome of attempting to apply a fix
type FixResult struct {
	Success            bool                `json:"success"`
	RolledBack         bool                `json:"rolled_back"`
	AppliedFix         *domain.AppliedFix  `json:"applied_fix,omitempty"`
	VerificationResult *VerificationResult `json:"verification_result,omitempty"`
	Message            string              `json:"message"`
}

// UndoResult represents the result of rolling back an applied fix
type UndoResult struct {
	Success      bool               `json:"success"`
	UndoneFix    *domain.AppliedFix `json:"undone_fix,omitempty"`
	RestoredPath string             `json:"restored_path"`
	Message      string             `json:"message"`
}

// Engine orchestrates diagnosis, fix application, active verification, and rollback
type Engine struct {
	classifier *Classifier
	store      *state.Store
	verifier   Verifier
	logsDir    string
	backupDir  string
}

// NewEngine creates a new Fix Engine
func NewEngine(store *state.Store, verifier Verifier) (*Engine, error) {
	cl, err := NewDefaultClassifier()
	if err != nil {
		return nil, err
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	logsDir := filepath.Join(home, ".local", "share", "compat", "logs")
	backupDir := filepath.Join(home, ".local", "share", "compat", "backups")

	_ = os.MkdirAll(logsDir, 0755)
	_ = os.MkdirAll(backupDir, 0755)

	if verifier == nil {
		verifier = NewDefaultVerifier()
	}

	return &Engine{
		classifier: cl,
		store:      store,
		verifier:   verifier,
		logsDir:    logsDir,
		backupDir:  backupDir,
	}, nil
}

// LogFilePath returns the expected log file path for an app
func (e *Engine) LogFilePath(appID string) string {
	return filepath.Join(e.logsDir, appID+".log")
}

// CurrentSessionType determines the active display server session
func CurrentSessionType() string {
	sess := os.Getenv("XDG_SESSION_TYPE")
	if sess != "" {
		return strings.ToLower(sess)
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return "headless"
}

// DetectAppType checks package metadata and directory layout for frameworks like electron
func DetectAppType(app *domain.ApplicationRecord) string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	appDir := filepath.Join(home, ".local", "share", "compat", "apps", app.AppID)
	// Check for electron markers
	if _, err := os.Stat(filepath.Join(appDir, "chrome-sandbox")); err == nil {
		return "electron"
	}
	if _, err := os.Stat(filepath.Join(appDir, "libffmpeg.so")); err == nil {
		return "electron"
	}
	if _, err := os.Stat(filepath.Join(appDir, "resources", "app.asar")); err == nil {
		return "electron"
	}
	if _, err := os.Stat(filepath.Join(appDir, "LICENSE.electron.txt")); err == nil {
		return "electron"
	}
	return string(app.PackageType)
}

// Diagnose inspects logs for known failure patterns
func (e *Engine) Diagnose(ctx context.Context, appID string) (*Diagnosis, error) {
	app, err := e.store.Get(appID)
	if err != nil {
		return nil, err
	}

	logPath := e.LogFilePath(appID)
	data, err := os.ReadFile(logPath)
	if err != nil {
		return &Diagnosis{
			AppID:   appID,
			LogPath: logPath,
		}, nil
	}

	lines := strings.Split(string(data), "\n")
	execCtx := ExecutionContext{
		AppType:        DetectAppType(app),
		SessionType:    CurrentSessionType(),
		WindowDetected: false,
		ProcessAlive:   false,
		UptimeSeconds:  0,
	}

	res := e.classifier.ClassifyLog(lines, execCtx)

	noiseCount := 0
	for _, n := range res.NoiseMatches {
		noiseCount += len(n.MatchedLines)
	}

	diag := &Diagnosis{
		AppID:      appID,
		LogPath:    logPath,
		NoiseCount: noiseCount,
	}

	if res.HasFatal() {
		primary := res.FatalMatches[0]
		diag.HasFatal = true
		patCopy := primary.Pattern
		diag.Pattern = &patCopy
		diag.MatchedLines = primary.MatchedLines
		diag.ProposedFixes = primary.Pattern.Fixes
	}

	return diag, nil
}

// ModifyWrapperContent injects or updates a compat fix block into a launcher wrapper
func ModifyWrapperContent(content string, fixOption patterns.FixOption) string {
	lines := strings.Split(content, "\n")

	// 1. Strip any existing compat fix block
	var cleaned []string
	inBlock := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "# COMPAT_FIX_BEGIN") {
			inBlock = true
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "# COMPAT_FIX_END") {
			inBlock = false
			continue
		}
		if !inBlock {
			cleaned = append(cleaned, l)
		}
	}

	// 2. Find position of the exec line
	execIdx := -1
	for i, l := range cleaned {
		if strings.HasPrefix(strings.TrimSpace(l), "exec ") {
			execIdx = i
			break
		}
	}

	var block []string
	block = append(block, fmt.Sprintf("# COMPAT_FIX_BEGIN: %s", fixOption.ID))
	for k, v := range fixOption.Env {
		block = append(block, fmt.Sprintf("export %s=%q", k, v))
	}
	if len(fixOption.Flags) > 0 {
		flagStr := strings.Join(fixOption.Flags, " ")
		block = append(block, fmt.Sprintf("FLAGS=%q", flagStr))
	}
	block = append(block, "# COMPAT_FIX_END")

	if execIdx != -1 {
		res := make([]string, 0, len(cleaned)+len(block))
		res = append(res, cleaned[:execIdx]...)
		res = append(res, block...)
		res = append(res, cleaned[execIdx:]...)
		return strings.Join(res, "\n")
	}

	res := append(cleaned, block...)
	return strings.Join(res, "\n")
}

// ApplyFix backs up the wrapper, applies the selected fix, executes active verification, and rolls back if verification fails
func (e *Engine) ApplyFix(ctx context.Context, appID string, fixOption patterns.FixOption) (*FixResult, error) {
	app, err := e.store.Get(appID)
	if err != nil {
		return nil, err
	}

	launcherPath := app.LauncherPath
	if launcherPath == "" {
		launcherPath = filepath.Join(os.Getenv("HOME"), ".local", "share", "compat", "bin", appID)
	}

	origBytes, err := os.ReadFile(launcherPath)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "cannot read launcher script to apply fix", err)
	}

	// 1. Create durable backup
	appBackupDir := filepath.Join(e.backupDir, appID)
	if err := os.MkdirAll(appBackupDir, 0755); err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "failed creating backup directory", err)
	}

	timestamp := time.Now().UTC().Format("20060102_150405")
	backupPath := filepath.Join(appBackupDir, fmt.Sprintf("%s_%s.bak", timestamp, filepath.Base(launcherPath)))
	if err := os.WriteFile(backupPath, origBytes, 0755); err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "failed creating backup file", err)
	}

	// Record backup in resource registry
	_ = e.store.AddResource(domain.ResourceOwnership{
		ID:        fmt.Sprintf("backup-%s-%s", appID, timestamp),
		AppID:     appID,
		Type:      "backup_file",
		PathOrID:  backupPath,
		CreatedAt: time.Now().UTC(),
	})

	// 2. Modify launcher wrapper
	newContent := ModifyWrapperContent(string(origBytes), fixOption)
	if err := os.WriteFile(launcherPath, []byte(newContent), 0755); err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "failed updating launcher wrapper", err)
	}

	// 3. Active verification (10s stability + window detection)
	verResult, err := e.verifier.Verify(ctx, app, launcherPath)
	if err != nil || (verResult != nil && !verResult.Success) {
		// Verification failed: strictly rollback to backup
		_ = os.WriteFile(launcherPath, origBytes, 0755)
		msg := "Verification failed"
		if verResult != nil && verResult.Message != "" {
			msg = verResult.Message
		}
		if err != nil {
			msg = fmt.Sprintf("%s (%v)", msg, err)
		}

		_ = e.store.LogAudit(appID, "FIX_FAILED", fmt.Sprintf("Fix %s failed verification: %s. Changes rolled back.", fixOption.ID, msg))

		return &FixResult{
			Success:            false,
			RolledBack:         true,
			VerificationResult: verResult,
			Message:            msg,
		}, nil
	}

	// 4. Verification passed: record in SQLite
	flagsStr := strings.Join(fixOption.Flags, " ")
	appliedFix := &domain.AppliedFix{
		AppID:      appID,
		PatternID:  fixOption.ID,
		FixAction:  string(fixOption.Action),
		Details:    fmt.Sprintf("Flags: %s; Env: %v", flagsStr, fixOption.Env),
		BackupPath: backupPath,
		TargetPath: launcherPath,
		AppliedAt:  time.Now().UTC(),
		Status:     "applied",
	}

	if err := e.store.RecordFix(appliedFix); err != nil {
		return nil, err
	}

	_ = e.store.LogAudit(appID, "FIX_APPLIED", fmt.Sprintf("Applied and verified fix %s: %s", fixOption.ID, fixOption.Description))

	return &FixResult{
		Success:            true,
		AppliedFix:         appliedFix,
		VerificationResult: verResult,
		Message:            fmt.Sprintf("Fix '%s' applied and verified successfully: %s", fixOption.Description, verResult.Message),
	}, nil
}

// UndoFix rolls back the most recent applied fix for an app
func (e *Engine) UndoFix(ctx context.Context, appID string) (*UndoResult, error) {
	fix, err := e.store.GetLatestActiveFix(appID)
	if err != nil {
		return nil, err
	}

	if fix.BackupPath == "" || fix.TargetPath == "" {
		return nil, domain.NewError(domain.ErrCodeStateCorrupt, "fix record lacks backup or target path", "", "")
	}

	backupData, err := os.ReadFile(fix.BackupPath)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "cannot read backup file: "+fix.BackupPath, err)
	}

	if err := os.WriteFile(fix.TargetPath, backupData, 0755); err != nil {
		return nil, domain.WrapError(domain.ErrCodeRunnerFailed, "failed restoring backup to: "+fix.TargetPath, err)
	}

	if err := e.store.MarkFixRolledBack(fix.ID); err != nil {
		return nil, err
	}

	_ = e.store.LogAudit(appID, "FIX_ROLLBACK", fmt.Sprintf("Rolled back fix %s (ID: %d) from backup %s", fix.PatternID, fix.ID, fix.BackupPath))

	return &UndoResult{
		Success:      true,
		UndoneFix:    fix,
		RestoredPath: fix.TargetPath,
		Message:      fmt.Sprintf("Successfully restored %s from backup %s", fix.TargetPath, fix.BackupPath),
	}, nil
}
