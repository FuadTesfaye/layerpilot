package patterns

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var embeddedPatterns embed.FS

type Severity string

const (
	SeverityFatal Severity = "fatal"
	SeverityNoise Severity = "noise"
)

// Pattern defines an observable failure or noise signature with associated remediation
type Pattern struct {
	ID          string      `yaml:"id" json:"id"`
	Severity    Severity    `yaml:"severity" json:"severity"`
	Cause       string      `yaml:"cause" json:"cause"`
	Detect      DetectRule  `yaml:"detect" json:"detect"`
	Fixes       []FixOption `yaml:"fixes" json:"fixes"`
	Verify      VerifyRule  `yaml:"verify" json:"verify"`
	ManualSteps []string    `yaml:"manual_steps" json:"manual_steps"`

	// compiledRegexes caches compiled regular expressions
	compiledRegexes []*regexp.Regexp
}

type DetectRule struct {
	AppType       string   `yaml:"app_type,omitempty" json:"app_type,omitempty"`             // e.g. "electron"
	SessionType   string   `yaml:"session_type,omitempty" json:"session_type,omitempty"`     // e.g. "wayland"
	LogRegexes    []string `yaml:"log_regexes" json:"log_regexes"`
	NoWindowCheck bool     `yaml:"no_window_check,omitempty" json:"no_window_check,omitempty"`
}

type FixOption struct {
	ID          string            `yaml:"id" json:"id"`
	Description string            `yaml:"description" json:"description"`
	Tier        int               `yaml:"tier" json:"tier"`
	Action      string            `yaml:"action" json:"action"` // e.g. "wrapper_flags", "package_install", etc.
	Flags       []string          `yaml:"flags,omitempty" json:"flags,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Command     []string          `yaml:"command,omitempty" json:"command,omitempty"`
}

type VerifyRule struct {
	MinUptimeSeconds int  `yaml:"min_uptime_seconds" json:"min_uptime_seconds"`
	RequireWindow    bool `yaml:"require_window" json:"require_window"`
}

// Compile prepares regular expressions for fast matching
func (p *Pattern) Compile() error {
	p.compiledRegexes = make([]*regexp.Regexp, 0, len(p.Detect.LogRegexes))
	for _, expr := range p.Detect.LogRegexes {
		re, err := regexp.Compile(expr)
		if err != nil {
			return fmt.Errorf("pattern '%s' invalid regex '%s': %w", p.ID, expr, err)
		}
		p.compiledRegexes = append(p.compiledRegexes, re)
	}
	return nil
}

// MatchesLine returns true if any compiled regex matches the given text line
func (p *Pattern) MatchesLine(line string) bool {
	for _, re := range p.compiledRegexes {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// LoadDefaultPatterns loads all embedded YAML patterns
func LoadDefaultPatterns() ([]Pattern, error) {
	return LoadPatternsFromFS(embeddedPatterns)
}

// LoadPatternsFromFS loads patterns from an arbitrary fs.FS
func LoadPatternsFromFS(filesystem fs.FS) ([]Pattern, error) {
	entries, err := fs.ReadDir(filesystem, ".")
	if err != nil {
		return nil, fmt.Errorf("failed reading patterns directory: %w", err)
	}

	var result []Pattern
	for _, entry := range entries {
		if entry.IsDir() || (!regexMatch(`\.ya?ml$`, entry.Name())) {
			continue
		}

		data, err := fs.ReadFile(filesystem, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed reading pattern file '%s': %w", entry.Name(), err)
		}

		var pat Pattern
		if err := yaml.Unmarshal(data, &pat); err != nil {
			return nil, fmt.Errorf("failed unmarshaling pattern file '%s': %w", entry.Name(), err)
		}

		if err := pat.Compile(); err != nil {
			return nil, err
		}

		result = append(result, pat)
	}

	return result, nil
}

func regexMatch(pattern, val string) bool {
	matched, _ := regexp.MatchString(pattern, val)
	return matched
}
