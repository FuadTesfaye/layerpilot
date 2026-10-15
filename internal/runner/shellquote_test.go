package runner

import (
	"os/exec"
	"testing"
)

func TestShellQuoteRoundTripsThroughShell(t *testing.T) {
	inputs := []string{
		"plain",
		"with space",
		"it's quoted",
		"$(touch pwned)",
		"`id`",
		"$HOME and ${PATH}",
		"line\nbreak",
		"",
	}
	for _, in := range inputs {
		out, err := exec.Command("sh", "-c", "printf '%s' "+ShellQuote(in)).Output()
		if err != nil {
			t.Fatalf("sh failed for %q: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("ShellQuote(%q) expanded to %q", in, out)
		}
	}
}
