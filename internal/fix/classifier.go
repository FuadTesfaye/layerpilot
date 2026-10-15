package fix

import (
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/patterns"
)

// ExecutionContext holds host and runtime facts captured during application launch
type ExecutionContext struct {
	AppType        string // e.g. "electron", "gtk", "qt", "generic"
	SessionType    string // e.g. "wayland", "x11"
	WindowDetected bool   // true if an active X11 or Wayland window was found
	ProcessAlive   bool   // true if the application process was still running
	UptimeSeconds  int    // seconds application stayed alive during test
}

// Match captures a pattern along with the specific log lines that matched it
type Match struct {
	Pattern      patterns.Pattern `json:"pattern"`
	MatchedLines []string         `json:"matched_lines"`
}

// ClassificationResult contains classified fatal failures and filtered noise
type ClassificationResult struct {
	FatalMatches []Match `json:"fatal_matches"`
	NoiseMatches []Match `json:"noise_matches"`
}

// HasFatal returns true if at least one fatal failure pattern was identified
func (r *ClassificationResult) HasFatal() bool {
	return len(r.FatalMatches) > 0
}

// PrimaryCause returns the plain-language cause of the highest-priority fatal pattern, or empty
func (r *ClassificationResult) PrimaryCause() string {
	if len(r.FatalMatches) == 0 {
		return ""
	}
	return r.FatalMatches[0].Pattern.Cause
}

// Classifier inspects execution logs and runtime context against pattern definitions
type Classifier struct {
	patterns []patterns.Pattern
}

// NewClassifier creates a classifier initialized with a slice of patterns
func NewClassifier(pats []patterns.Pattern) *Classifier {
	return &Classifier{patterns: pats}
}

// NewDefaultClassifier creates a classifier with default embedded YAML patterns
func NewDefaultClassifier() (*Classifier, error) {
	pats, err := patterns.LoadDefaultPatterns()
	if err != nil {
		return nil, err
	}
	return NewClassifier(pats), nil
}

// ClassifyLog lines against loaded patterns using runtime execution context
func (c *Classifier) ClassifyLog(logLines []string, ctx ExecutionContext) *ClassificationResult {
	result := &ClassificationResult{
		FatalMatches: make([]Match, 0),
		NoiseMatches: make([]Match, 0),
	}

	for _, pat := range c.patterns {
		// Verify environment context criteria
		if pat.Detect.AppType != "" && !strings.EqualFold(pat.Detect.AppType, ctx.AppType) {
			continue
		}
		if pat.Detect.SessionType != "" && !strings.EqualFold(pat.Detect.SessionType, ctx.SessionType) {
			continue
		}
		if pat.Detect.NoWindowCheck && ctx.WindowDetected {
			// If pattern specifically requires no window to have appeared, but a window DID appear, skip
			continue
		}

		var matchedLines []string
		for _, line := range logLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if pat.MatchesLine(line) {
				matchedLines = append(matchedLines, line)
			}
		}

		// If log regexes are specified, at least one must match
		if len(pat.Detect.LogRegexes) > 0 && len(matchedLines) == 0 {
			continue
		}
		// If no log regexes are specified, check context triggers (e.g. hung process with no window)
		contextTriggered := pat.Detect.NoWindowCheck && !ctx.WindowDetected && ctx.ProcessAlive && ctx.UptimeSeconds >= 5
		if len(pat.Detect.LogRegexes) == 0 && !contextTriggered {
			continue
		}

		m := Match{
			Pattern:      pat,
			MatchedLines: matchedLines,
		}
		if pat.Severity == patterns.SeverityFatal {
			result.FatalMatches = append(result.FatalMatches, m)
		} else if pat.Severity == patterns.SeverityNoise {
			result.NoiseMatches = append(result.NoiseMatches, m)
		}
	}

	return result
}
