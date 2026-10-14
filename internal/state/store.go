package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	_ "modernc.org/sqlite"
)

// Store handles persistent SQLite storage for managed applications and resources
type Store struct {
	db *sql.DB
}

// Open initializes or opens the SQLite database at dbPath
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed to create state directory", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed to open sqlite database", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// OpenDefault opens the standard state store in ~/.local/share/compat/state.db
func OpenDefault() (*Store, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	dbPath := filepath.Join(home, ".local", "share", "compat", "state.db")
	return Open(dbPath)
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS applications (
			app_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			package_format TEXT NOT NULL,
			package_hash TEXT NOT NULL,
			package_arch TEXT NOT NULL,
			package_version TEXT NOT NULL,
			engine_id TEXT NOT NULL,
			engine_version TEXT NOT NULL,
			host_os TEXT NOT NULL,
			host_distro TEXT NOT NULL,
			status TEXT NOT NULL,
			launcher_path TEXT,
			binary_entrypoint TEXT,
			package_type TEXT,
			installed_at TIMESTAMP NOT NULL,
			last_launched_at TIMESTAMP,
			compatibility_profile_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS resources (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			type TEXT NOT NULL,
			path_or_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id TEXT NOT NULL,
			operation TEXT NOT NULL,
			details TEXT,
			timestamp TIMESTAMP NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return domain.WrapError(domain.ErrCodeDatabaseError, "migration query failed", err)
		}
	}
	return nil
}

func (s *Store) Save(app *domain.ApplicationRecord) error {
	query := `INSERT INTO applications (
		app_id, display_name, package_format, package_hash, package_arch,
		package_version, engine_id, engine_version, host_os, host_distro,
		status, launcher_path, binary_entrypoint, package_type, installed_at,
		last_launched_at, compatibility_profile_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(app_id) DO UPDATE SET
		status = excluded.status,
		launcher_path = excluded.launcher_path,
		binary_entrypoint = excluded.binary_entrypoint,
		last_launched_at = excluded.last_launched_at;`

	_, err := s.db.Exec(query,
		app.AppID, app.DisplayName, string(app.PackageFormat), app.PackageHash,
		app.PackageArch, app.PackageVersion, app.EngineID, app.EngineVersion,
		app.HostOS, app.HostDistro, string(app.Status), app.LauncherPath,
		app.BinaryEntrypoint, string(app.PackageType), app.InstalledAt,
		app.LastLaunchedAt, app.CompatibilityProfileID,
	)
	if err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed to save application record", err)
	}

	return nil
}

func (s *Store) Get(appID string) (*domain.ApplicationRecord, error) {
	query := `SELECT 
		app_id, display_name, package_format, package_hash, package_arch,
		package_version, engine_id, engine_version, host_os, host_distro,
		status, launcher_path, binary_entrypoint, package_type, installed_at,
		last_launched_at, compatibility_profile_id
	FROM applications WHERE app_id = ?;`

	row := s.db.QueryRow(query, appID)
	var app domain.ApplicationRecord
	var pkgFmt, status, pkgType string

	err := row.Scan(
		&app.AppID, &app.DisplayName, &pkgFmt, &app.PackageHash,
		&app.PackageArch, &app.PackageVersion, &app.EngineID, &app.EngineVersion,
		&app.HostOS, &app.HostDistro, &status, &app.LauncherPath,
		&app.BinaryEntrypoint, &pkgType, &app.InstalledAt,
		&app.LastLaunchedAt, &app.CompatibilityProfileID,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewError(
			domain.ErrCodeAppNotFound,
			fmt.Sprintf("application '%s' is not registered in state store", appID),
			"Use 'compat list' to see all installed applications",
			"Query failed",
		)
	}
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed to query application", err)
	}

	app.PackageFormat = domain.PackageFormat(pkgFmt)
	app.Status = domain.LifecycleState(status)
	app.PackageType = domain.PackageType(pkgType)

	// Fetch resources
	resList, err := s.GetResources(appID)
	if err == nil {
		app.Resources = resList
	}

	return &app, nil
}

func (s *Store) List() ([]domain.ApplicationRecord, error) {
	query := `SELECT 
		app_id, display_name, package_format, package_hash, package_arch,
		package_version, engine_id, engine_version, host_os, host_distro,
		status, launcher_path, binary_entrypoint, package_type, installed_at,
		last_launched_at, compatibility_profile_id
	FROM applications ORDER BY installed_at DESC;`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed to list applications", err)
	}
	defer rows.Close()

	var apps []domain.ApplicationRecord
	for rows.Next() {
		var app domain.ApplicationRecord
		var pkgFmt, status, pkgType string
		err := rows.Scan(
			&app.AppID, &app.DisplayName, &pkgFmt, &app.PackageHash,
			&app.PackageArch, &app.PackageVersion, &app.EngineID, &app.EngineVersion,
			&app.HostOS, &app.HostDistro, &status, &app.LauncherPath,
			&app.BinaryEntrypoint, &pkgType, &app.InstalledAt,
			&app.LastLaunchedAt, &app.CompatibilityProfileID,
		)
		if err != nil {
			return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed scanning application row", err)
		}
		app.PackageFormat = domain.PackageFormat(pkgFmt)
		app.Status = domain.LifecycleState(status)
		app.PackageType = domain.PackageType(pkgType)
		apps = append(apps, app)
	}

	return apps, nil
}

func (s *Store) UpdateStatus(appID string, status domain.LifecycleState) error {
	query := `UPDATE applications SET status = ? WHERE app_id = ?;`
	_, err := s.db.Exec(query, string(status), appID)
	if err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed updating application status", err)
	}
	return nil
}

func (s *Store) Delete(appID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed to start transaction", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM resources WHERE app_id = ?;", appID); err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed deleting application resources", err)
	}
	if _, err := tx.Exec("DELETE FROM applications WHERE app_id = ?;", appID); err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed deleting application record", err)
	}

	return tx.Commit()
}

func (s *Store) AddResource(res domain.ResourceOwnership) error {
	query := `INSERT INTO resources (id, app_id, type, path_or_id, created_at) VALUES (?, ?, ?, ?, ?);`
	_, err := s.db.Exec(query, res.ID, res.AppID, res.Type, res.PathOrID, res.CreatedAt)
	if err != nil {
		return domain.WrapError(domain.ErrCodeDatabaseError, "failed adding resource record", err)
	}
	return nil
}

func (s *Store) GetResources(appID string) ([]domain.ResourceOwnership, error) {
	query := `SELECT id, app_id, type, path_or_id, created_at FROM resources WHERE app_id = ?;`
	rows, err := s.db.Query(query, appID)
	if err != nil {
		return nil, domain.WrapError(domain.ErrCodeDatabaseError, "failed querying resources", err)
	}
	defer rows.Close()

	var resources []domain.ResourceOwnership
	for rows.Next() {
		var r domain.ResourceOwnership
		if err := rows.Scan(&r.ID, &r.AppID, &r.Type, &r.PathOrID, &r.CreatedAt); err != nil {
			return nil, err
		}
		resources = append(resources, r)
	}
	return resources, nil
}

func (s *Store) LogAudit(appID, operation, details string) error {
	query := `INSERT INTO audit_logs (app_id, operation, details, timestamp) VALUES (?, ?, ?, ?);`
	_, err := s.db.Exec(query, appID, operation, details, time.Now().UTC())
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
