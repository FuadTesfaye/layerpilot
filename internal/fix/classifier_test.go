package fix

import (
	"os"
	"strings"
	"testing"
)

func TestGoldenLog1NuzzCrashGpu(t *testing.T) {
	c, err := NewDefaultClassifier()
	if err != nil {
		t.Fatalf("failed loading default classifier: %v", err)
	}

	data, err := os.ReadFile("testdata/nuzz_crash_gpu.log")
	if err != nil {
		t.Fatalf("failed reading golden log: %v", err)
	}
	lines := strings.Split(string(data), "\n")

	ctx := ExecutionContext{
		AppType:        "electron",
		SessionType:    "wayland",
		WindowDetected: false,
		ProcessAlive:   false,
		UptimeSeconds:  5,
	}

	result := c.ClassifyLog(lines, ctx)
	if !result.HasFatal() {
		t.Errorf("expected fatal match in golden log 1, got none")
	}

	if len(result.FatalMatches) == 0 || result.FatalMatches[0].Pattern.ID != "electron-wayland-no-window" {
		t.Errorf("expected primary fatal pattern 'electron-wayland-no-window', got %v", result.FatalMatches)
	}

	if len(result.NoiseMatches) == 0 {
		t.Errorf("expected noise pattern matches in golden log 1, got none")
	}

	// Verify noise is never reported as primary cause
	primaryCause := result.PrimaryCause()
	if strings.Contains(strings.ToLower(primaryCause), "noise") || strings.Contains(strings.ToLower(primaryCause), "updater") {
		t.Errorf("noise was incorrectly reported as primary cause: %s", primaryCause)
	}
}

func TestGoldenLog2NuzzFallbackSoftware(t *testing.T) {
	c, err := NewDefaultClassifier()
	if err != nil {
		t.Fatalf("failed loading default classifier: %v", err)
	}

	data, err := os.ReadFile("testdata/nuzz_fallback_software.log")
	if err != nil {
		t.Fatalf("failed reading golden log: %v", err)
	}
	lines := strings.Split(string(data), "\n")

	ctx := ExecutionContext{
		AppType:        "electron",
		SessionType:    "wayland",
		WindowDetected: false,
		ProcessAlive:   true,
		UptimeSeconds:  12,
	}

	result := c.ClassifyLog(lines, ctx)
	if !result.HasFatal() {
		t.Errorf("expected fatal match in golden log 2, got none")
	}

	foundP1 := false
	for _, m := range result.FatalMatches {
		if m.Pattern.ID == "electron-wayland-no-window" {
			foundP1 = true
			break
		}
	}
	if !foundP1 {
		t.Errorf("expected pattern 'electron-wayland-no-window' in fatal matches")
	}

	if len(result.NoiseMatches) == 0 {
		t.Errorf("expected noise pattern matches in golden log 2")
	}
}

func TestClassifierTableCases(t *testing.T) {
	c, err := NewDefaultClassifier()
	if err != nil {
		t.Fatalf("failed loading default classifier: %v", err)
	}

	tests := []struct {
		name          string
		logLines      []string
		ctx           ExecutionContext
		expectFatal   bool
		expectedFatal string
		expectNoise   bool
	}{
		{
			name: "clean successful launch",
			logLines: []string{
				"Application initialized",
				"Connected to server",
				"Window rendered successfully",
			},
			ctx: ExecutionContext{
				AppType:        "electron",
				SessionType:    "wayland",
				WindowDetected: true,
				ProcessAlive:   true,
				UptimeSeconds:  15,
			},
			expectFatal: false,
			expectNoise: false,
		},
		{
			name: "window detected prevents P1 false positive",
			logLines: []string{
				"XGetWindowAttributes failed for window 1",
			},
			ctx: ExecutionContext{
				AppType:        "electron",
				SessionType:    "wayland",
				WindowDetected: true, // Window WAS detected, so no failure!
				ProcessAlive:   true,
				UptimeSeconds:  20,
			},
			expectFatal: false,
			expectNoise: false,
		},
		{
			name: "session is X11 prevents P1 false positive",
			logLines: []string{
				"XGetWindowAttributes failed for window 1",
			},
			ctx: ExecutionContext{
				AppType:        "electron",
				SessionType:    "x11", // X11 session, not Wayland
				WindowDetected: false,
				ProcessAlive:   true,
				UptimeSeconds:  20,
			},
			expectFatal: false,
			expectNoise: false,
		},
		{
			name: "pure noise only does not trigger fatal",
			logLines: []string{
				"[electron-updater] Cannot find latest-linux.yml: HttpError: 404 Not Found",
				"Checking for update",
				"Update for version 1.0.0 is not available",
				"GPU at 0000:00:02.0 has active display audio device",
			},
			ctx: ExecutionContext{
				AppType:        "electron",
				SessionType:    "wayland",
				WindowDetected: true,
				ProcessAlive:   true,
				UptimeSeconds:  15,
			},
			expectFatal: false,
			expectNoise: true,
		},
		{
			name: "P2 missing FUSE detection",
			logLines: []string{
				"dlopen(): error opening libfuse.so.2",
				"Cannot mount AppImage, FUSE kernel module may be missing",
			},
			ctx: ExecutionContext{
				AppType:        "generic",
				SessionType:    "x11",
				WindowDetected: false,
				ProcessAlive:   false,
				UptimeSeconds:  1,
			},
			expectFatal:   true,
			expectedFatal: "missing-fuse",
			expectNoise:   false,
		},
		{
			name: "P3 SELinux AVC denial detection",
			logLines: []string{
				"type=AVC msg=audit(1697200000.123:456): avc:  denied  { execute } for  pid=1234 comm=\"my-app\" path=\"/home/user/my-app\"",
			},
			ctx: ExecutionContext{
				AppType:        "generic",
				SessionType:    "wayland",
				WindowDetected: false,
				ProcessAlive:   false,
				UptimeSeconds:  1,
			},
			expectFatal:   true,
			expectedFatal: "selinux-denial",
			expectNoise:   false,
		},
		{
			name: "P4 missing newer glibc symbol version",
			logLines: []string{
				"./app: /lib64/libc.so.6: version `GLIBC_2.38' not found (required by ./app)",
			},
			ctx: ExecutionContext{
				AppType:        "generic",
				SessionType:    "wayland",
				WindowDetected: false,
				ProcessAlive:   false,
				UptimeSeconds:  1,
			},
			expectFatal:   true,
			expectedFatal: "glibc-version-too-old",
			expectNoise:   false,
		},
		{
			name: "P5 Chrome sandbox userns failure",
			logLines: []string{
				"[1234:1234:1015/090000.123:FATAL:sandbox_linux.cc(374)] Check failed: No usable sandbox!",
				"The setuid sandbox is not running to provide IPC between the Zygote and the browser node.",
			},
			ctx: ExecutionContext{
				AppType:        "electron",
				SessionType:    "wayland",
				WindowDetected: false,
				ProcessAlive:   false,
				UptimeSeconds:  2,
			},
			expectFatal:   true,
			expectedFatal: "chrome-sandbox-userns",
			expectNoise:   false,
		},
		{
			name: "P6 missing shared library ld.so failure",
			logLines: []string{
				"./app: error while loading shared libraries: libasound.so.2: cannot open shared object file: No such file or directory",
			},
			ctx: ExecutionContext{
				AppType:        "generic",
				SessionType:    "wayland",
				WindowDetected: false,
				ProcessAlive:   false,
				UptimeSeconds:  1,
			},
			expectFatal:   true,
			expectedFatal: "missing-shared-library",
			expectNoise:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := c.ClassifyLog(tc.logLines, tc.ctx)
			if res.HasFatal() != tc.expectFatal {
				t.Errorf("expected HasFatal=%v, got %v", tc.expectFatal, res.HasFatal())
			}
			if tc.expectedFatal != "" {
				if len(res.FatalMatches) == 0 || res.FatalMatches[0].Pattern.ID != tc.expectedFatal {
					t.Errorf("expected fatal ID %s, got %v", tc.expectedFatal, res.FatalMatches)
				}
			}
			if (len(res.NoiseMatches) > 0) != tc.expectNoise {
				t.Errorf("expected noise matches=%v, got count %d", tc.expectNoise, len(res.NoiseMatches))
			}
		})
	}
}
