package lab

import (
	"os"
	"regexp"
	"strings"
)

// Redactor scrubs personal and sensitive information from execution logs and reports
type Redactor struct {
	homeDir  string
	username string
	hostname string
	rules    []redactRule
}

type redactRule struct {
	re          *regexp.Regexp
	replacement string
}

// NewRedactor creates a configured Redactor with current host environment facts
func NewRedactor() *Redactor {
	home := os.Getenv("HOME")
	user := os.Getenv("USER")
	host, _ := os.Hostname()

	return NewCustomRedactor(home, user, host)
}

// NewCustomRedactor initializes a redactor with explicit test values
func NewCustomRedactor(home, user, host string) *Redactor {
	r := &Redactor{
		homeDir:  strings.TrimSpace(home),
		username: strings.TrimSpace(user),
		hostname: strings.TrimSpace(host),
	}

	r.initRules()
	return r
}

func (r *Redactor) initRules() {
	var rules []redactRule

	// 1. Authorization headers
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)(authorization:\s*)(?:bearer\s+|basic\s+|token\s+)?([^\r\n\s]+)`),
		replacement: `${1}[REDACTED]`,
	})
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)(proxy-authorization:\s*)([^\r\n]+)`),
		replacement: `${1}[REDACTED]`,
	})
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)(set-cookie:\s*)([^\r\n]+)`),
		replacement: `${1}[COOKIE REDACTED]`,
	})
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)(cookie:\s*)([^\r\n]+)`),
		replacement: `${1}[COOKIE REDACTED]`,
	})

	// 2. Tokens and API keys
	// JWT Tokens (header.payload.signature)
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\bey[a-zA-Z0-9_-]{10,}\.ey[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\b`),
		replacement: `[JWT REDACTED]`,
	})
	// GitHub Tokens (ghp_, gho_, ghu_, ghs_, ghr_)
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
		replacement: `[GITHUB_TOKEN REDACTED]`,
	})
	// GitLab Personal Access Tokens (glpat-)
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
		replacement: `[GITLAB_TOKEN REDACTED]`,
	})
	// Slack Tokens (xoxb-, xoxp-, etc)
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\bxox[baprs]-[0-9a-zA-Z]{10,}\b`),
		replacement: `[SLACK_TOKEN REDACTED]`,
	})
	// AWS Access Key ID
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		replacement: `[AWS_KEY REDACTED]`,
	})
	// Generic Key/Token assignments in query parameters or config logs
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)\b(api[_-]?key|client[_-]?secret|password|secret|access[_-]?token|auth[_-]?token)(["'=:\s]+)([a-zA-Z0-9_.-]{8,})`),
		replacement: `${1}${2}[REDACTED]`,
	})
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`(?i)\b(token)(["'=:\s]+)([^\[\r\n\s"'][a-zA-Z0-9_.-]{7,})`),
		replacement: `${1}${2}[REDACTED]`,
	})

	// 3. Email addresses
	rules = append(rules, redactRule{
		re:          regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`),
		replacement: `[EMAIL REDACTED]`,
	})

	r.rules = rules
}

var ipRegex = regexp.MustCompile(`\b(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

// Redact applies all sanitization rules to the given input string
func (r *Redactor) Redact(text string) string {
	res := text

	// 1. Redact specific home directories if present
	if r.homeDir != "" && r.homeDir != "/" {
		res = strings.ReplaceAll(res, r.homeDir, "[HOME REDACTED]")
	}
	// Generic home paths (/home/user, /Users/user)
	genericHome := regexp.MustCompile(`/(?:home|Users)/[a-zA-Z0-9_.-]+`)
	res = genericHome.ReplaceAllString(res, "[HOME REDACTED]")

	// Windows profile paths (C:\Users\username)
	winHome := regexp.MustCompile(`[a-zA-Z]:\\Users\\[a-zA-Z0-9_.-]+`)
	res = winHome.ReplaceAllString(res, `[HOME REDACTED]`)

	// 2. Redact username if specified and non-trivial
	if r.username != "" && len(r.username) > 2 {
		res = strings.ReplaceAll(res, r.username, "[USER REDACTED]")
	}

	// 3. Redact hostname if specified and non-trivial
	if r.hostname != "" && len(r.hostname) > 2 && r.hostname != "localhost" {
		res = strings.ReplaceAll(res, r.hostname, "[HOSTNAME REDACTED]")
	}

	// 4. Apply regex rules
	for _, rule := range r.rules {
		res = rule.re.ReplaceAllString(res, rule.replacement)
	}

	// 5. Redact IPs while preserving localhost / 0.0.0.0
	res = ipRegex.ReplaceAllStringFunc(res, func(ip string) string {
		if ip == "127.0.0.1" || ip == "0.0.0.0" {
			return ip
		}
		return "[IP REDACTED]"
	})

	return res
}

// RedactLines applies Redact to each line in a slice
func (r *Redactor) RedactLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = r.Redact(l)
	}
	return out
}
