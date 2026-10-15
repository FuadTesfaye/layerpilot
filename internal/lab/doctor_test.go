package lab

import (
	"context"
	"fmt"
	"testing"
)

type mockExecutor struct {
	paths   map[string]string
	outputs map[string]string
	errors  map[string]error
}

func (m *mockExecutor) LookPath(file string) (string, error) {
	if p, ok := m.paths[file]; ok {
		return p, nil
	}
	return "", fmt.Errorf("executable not found: %s", file)
}

func (m *mockExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmdKey := name
	for _, a := range args {
		cmdKey += " " + a
	}

	if err, ok := m.errors[cmdKey]; ok {
		return "", err
	}
	if out, ok := m.outputs[cmdKey]; ok {
		return out, nil
	}
	return "", nil
}

func TestDoctorLinuxWithPodman(t *testing.T) {
	mock := &mockExecutor{
		paths: map[string]string{
			"podman": "/usr/bin/podman",
		},
		outputs: map[string]string{
			"podman version --format {{.Version}}": "5.2.0",
		},
	}

	doc := NewCustomDoctor(mock, "linux", "amd64")
	rep := doc.Diagnose(context.Background())

	if rep.HostOS != "linux" || rep.HostArch != "amd64" {
		t.Fatalf("unexpected OS/arch: %s/%s", rep.HostOS, rep.HostArch)
	}

	if rep.RecommendedRuntime != RuntimePodman {
		t.Errorf("expected podman recommended on Linux, got %s", rep.RecommendedRuntime)
	}

	foundPodman := false
	for _, r := range rep.Runtimes {
		if r.ID == RuntimePodman && r.Available {
			foundPodman = true
			if r.Version != "5.2.0" {
				t.Errorf("expected version 5.2.0, got %s", r.Version)
			}
		}
	}
	if !foundPodman {
		t.Errorf("expected podman runtime to be available")
	}
}

func TestDoctorMacOSWithLimaAndDocker(t *testing.T) {
	mock := &mockExecutor{
		paths: map[string]string{
			"docker":  "/usr/local/bin/docker",
			"limactl": "/opt/homebrew/bin/limactl",
		},
		outputs: map[string]string{
			"docker version --format {{.Server.Version}}": "27.2.0",
			"limactl list --json":                         `[{"name":"default","status":"Running"}]`,
		},
	}

	doc := NewCustomDoctor(mock, "darwin", "arm64")
	rep := doc.Diagnose(context.Background())

	if rep.RecommendedRuntime != RuntimeDocker {
		t.Errorf("expected docker recommended when podman absent on macOS, got %s", rep.RecommendedRuntime)
	}

	foundLima := false
	for _, r := range rep.Runtimes {
		if r.ID == RuntimeLima && r.Available {
			foundLima = true
		}
	}
	if !foundLima {
		t.Errorf("expected lima runtime to be available on macOS")
	}
}

func TestDoctorWindowsWithWSL2(t *testing.T) {
	mock := &mockExecutor{
		paths: map[string]string{
			"wsl.exe": "C:\\Windows\\System32\\wsl.exe",
		},
		outputs: map[string]string{
			"wsl.exe -l -v": "  NAME      STATE           VERSION\n* Ubuntu    Running         2",
		},
	}

	doc := NewCustomDoctor(mock, "windows", "amd64")
	rep := doc.Diagnose(context.Background())

	if rep.RecommendedRuntime != RuntimeWSL2 {
		t.Errorf("expected wsl2 recommended on Windows, got %s", rep.RecommendedRuntime)
	}
}

func TestCheckEmulation(t *testing.T) {
	isEmu, warn := CheckEmulation("arm64", "x86_64")
	if !isEmu || warn == "" {
		t.Errorf("expected emulation warning for x86_64 on arm64 host")
	}

	isEmuNative, _ := CheckEmulation("amd64", "x86_64")
	if isEmuNative {
		t.Errorf("native amd64 should not be marked emulated")
	}
}
