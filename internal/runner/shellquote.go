package runner

import "strings"

// ShellQuote returns s as a single-quoted POSIX shell word, so values written
// into generated launcher scripts are never expanded or executed by the shell
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
