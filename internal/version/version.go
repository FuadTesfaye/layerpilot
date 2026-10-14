package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is populated at build time via -ldflags
	Version = "v0.1.0-dev"
	// GitCommit is populated at build time
	GitCommit = "unknown"
	// BuildDate is populated at build time
	BuildDate = "unknown"
	// DatabaseSchemaVersion is the current schema version
	DatabaseSchemaVersion = 1
	// CatalogueVersion is the compatibility matrix catalogue version
	CatalogueVersion = "2026.10"
)

// Info holds version and runtime details
type Info struct {
	Version               string `json:"version"`
	GitCommit             string `json:"git_commit"`
	BuildDate             string `json:"build_date"`
	GoVersion             string `json:"go_version"`
	Platform              string `json:"platform"`
	DatabaseSchemaVersion int    `json:"database_schema_version"`
	CatalogueVersion      string `json:"catalogue_version"`
}

// Get returns the current Version Info
func Get() Info {
	return Info{
		Version:               Version,
		GitCommit:             GitCommit,
		BuildDate:             BuildDate,
		GoVersion:             runtime.Version(),
		Platform:              fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		DatabaseSchemaVersion: DatabaseSchemaVersion,
		CatalogueVersion:      CatalogueVersion,
	}
}

// String returns a human-readable version string
func (i Info) String() string {
	return fmt.Sprintf("compat version %s (%s) built %s for %s (%s)",
		i.Version, i.GitCommit, i.BuildDate, i.Platform, i.GoVersion)
}
