package hostinfo

import (
	"runtime"
	"testing"
)

func TestProfilerCurrent(t *testing.T) {
	p := New()
	hp, err := p.Current()
	if err != nil {
		t.Fatalf("Current() failed: %v", err)
	}

	if hp.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, hp.OS)
	}
	if hp.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %s, got %s", runtime.GOARCH, hp.Arch)
	}
	if runtime.GOOS == "linux" {
		if hp.Distro == "" {
			t.Logf("Notice: Distro is empty (running in non-standard linux environment)")
		}
	}
}
