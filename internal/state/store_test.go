package state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func TestStateStoreLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	app := &domain.ApplicationRecord{
		AppID:          "htop-12345678",
		DisplayName:    "htop",
		PackageFormat:  domain.FormatDeb,
		PackageHash:    "1234567890abcdef",
		PackageArch:    "amd64",
		PackageVersion: "3.2.2-1",
		EngineID:       "podman",
		EngineVersion:  "5.8.7",
		HostOS:         "linux",
		HostDistro:     "fedora",
		Status:         domain.StateInstalled,
		InstalledAt:    time.Now().UTC(),
		PackageType:    domain.PackageTypeCLI,
	}

	// 1. Save
	if err := store.Save(app); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 2. Add resource
	res := domain.ResourceOwnership{
		ID:        "res-1",
		AppID:     app.AppID,
		Type:      "container",
		PathOrID:  "compat-app-htop-12345678",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.AddResource(res); err != nil {
		t.Fatalf("AddResource failed: %v", err)
	}

	// 3. Get
	fetched, err := store.Get(app.AppID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if fetched.DisplayName != "htop" {
		t.Errorf("expected displayName 'htop', got '%s'", fetched.DisplayName)
	}
	if len(fetched.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(fetched.Resources))
	}

	// 4. Update status
	if err := store.UpdateStatus(app.AppID, domain.StateRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	fetchedUpdated, err := store.Get(app.AppID)
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if fetchedUpdated.Status != domain.StateRunning {
		t.Errorf("expected status Running, got %s", fetchedUpdated.Status)
	}

	// 5. List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 listed app, got %d", len(list))
	}

	// 6. Delete
	if err := store.Delete(app.AppID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.Get(app.AppID)
	if err == nil {
		t.Fatalf("expected error after delete, got nil")
	}
}
