package lab

import (
	"fmt"
	"strings"
)

// DefaultTargets returns the standard pinned distro targets supported by compat lab
func DefaultTargets() []TargetDefinition {
	return []TargetDefinition{
		{
			ID:             "fedora:42",
			DistroFamily:   "fedora",
			Image:          "quay.io/fedora/fedora:42",
			ImageDigest:    "sha256:fedora42pinned",
			GlibcVersion:   "2.41",
			PackageManager: "dnf",
			DefaultUser:    "developer",
			Capabilities:   []string{"static-scan", "smoke", "libs", "offline", "fresh-home", "window"},
			KnownPackages: map[string]string{
				"libasound.so.2":     "alsa-lib",
				"libnss3.so":         "nss",
				"libatk-1.0.so.0":    "atk",
				"libgtk-3.so.0":      "gtk3",
				"libX11.so.6":        "libX11",
				"libXcomposite.so.1": "libXcomposite",
				"libXdamage.so.1":    "libXdamage",
				"libXrandr.so.2":     "libXrandr",
				"libgbm.so.1":        "mesa-libgbm",
				"libfuse.so.2":       "fuse-libs",
			},
		},
		{
			ID:             "ubuntu:24.04",
			DistroFamily:   "debian",
			Image:          "docker.io/library/ubuntu:24.04",
			ImageDigest:    "sha256:ubuntu2404pinned",
			GlibcVersion:   "2.39",
			PackageManager: "apt",
			DefaultUser:    "developer",
			Capabilities:   []string{"static-scan", "smoke", "libs", "offline", "fresh-home", "window"},
			KnownPackages: map[string]string{
				"libasound.so.2":     "libasound2t64",
				"libnss3.so":         "libnss3",
				"libatk-1.0.so.0":    "libatk1.0-0t64",
				"libgtk-3.so.0":      "libgtk-3-0t64",
				"libX11.so.6":        "libx11-6",
				"libXcomposite.so.1": "libxcomposite1",
				"libXdamage.so.1":    "libxdamage1",
				"libXrandr.so.2":     "libxrandr2",
				"libgbm.so.1":        "libgbm1",
				"libfuse.so.2":       "libfuse2t64",
			},
		},
		{
			ID:             "debian:12",
			DistroFamily:   "debian",
			Image:          "docker.io/library/debian:12-slim",
			ImageDigest:    "sha256:debian12pinned",
			GlibcVersion:   "2.36",
			PackageManager: "apt",
			DefaultUser:    "developer",
			Capabilities:   []string{"static-scan", "smoke", "libs", "offline", "fresh-home", "window"},
			KnownPackages: map[string]string{
				"libasound.so.2":     "libasound2",
				"libnss3.so":         "libnss3",
				"libatk-1.0.so.0":    "libatk1.0-0",
				"libgtk-3.so.0":      "libgtk-3-0",
				"libX11.so.6":        "libx11-6",
				"libXcomposite.so.1": "libxcomposite1",
				"libXdamage.so.1":    "libxdamage1",
				"libXrandr.so.2":     "libxrandr2",
				"libgbm.so.1":        "libgbm1",
				"libfuse.so.2":       "libfuse2",
			},
		},
		{
			ID:             "archlinux",
			DistroFamily:   "arch",
			Image:          "docker.io/library/archlinux:base-20241201.0.284345",
			ImageDigest:    "sha256:archpinned",
			GlibcVersion:   "2.41",
			PackageManager: "pacman",
			DefaultUser:    "developer",
			Capabilities:   []string{"static-scan", "smoke", "libs", "offline", "fresh-home", "window"},
			KnownPackages: map[string]string{
				"libasound.so.2":     "alsa-lib",
				"libnss3.so":         "nss",
				"libatk-1.0.so.0":    "atk",
				"libgtk-3.so.0":      "gtk3",
				"libX11.so.6":        "libx11",
				"libXcomposite.so.1": "libxcomposite",
				"libXdamage.so.1":    "libxdamage",
				"libXrandr.so.2":     "libxrandr",
				"libgbm.so.1":        "mesa",
				"libfuse.so.2":       "fuse2",
			},
		},
		{
			ID:             "opensuse:15.6",
			DistroFamily:   "suse",
			Image:          "docker.io/opensuse/leap:15.6",
			ImageDigest:    "sha256:opensuse156pinned",
			GlibcVersion:   "2.38",
			PackageManager: "zypper",
			DefaultUser:    "developer",
			Capabilities:   []string{"static-scan", "smoke", "libs", "offline", "fresh-home"},
			KnownPackages: map[string]string{
				"libasound.so.2":     "libasound2",
				"libnss3.so":         "mozilla-nss",
				"libatk-1.0.so.0":    "libatk-1_0-0",
				"libgtk-3.so.0":      "gtk3",
				"libX11.so.6":        "libX11-6",
				"libXcomposite.so.1": "libXcomposite1",
				"libXdamage.so.1":    "libXdamage1",
				"libXrandr.so.2":     "libXrandr2",
				"libgbm.so.1":        "libgbm1",
				"libfuse.so.2":       "libfuse2",
			},
		},
	}
}

// FindTarget retrieves a target definition by ID or prefix match
func FindTarget(targetID string) (*TargetDefinition, error) {
	for _, t := range DefaultTargets() {
		if strings.EqualFold(t.ID, targetID) || strings.HasPrefix(strings.ToLower(t.ID), strings.ToLower(targetID)) {
			copyT := t
			return &copyT, nil
		}
	}
	return nil, fmt.Errorf("target '%s' is not defined; available targets: fedora:42, ubuntu:24.04, debian:12, archlinux, opensuse:15.6", targetID)
}

// FilterTargets resolves a comma-separated list of target names or returns defaults
func FilterTargets(targetNames []string) ([]TargetDefinition, error) {
	if len(targetNames) == 0 {
		return DefaultTargets()[:4], nil // Default to fedora, ubuntu, debian, archlinux
	}

	var res []TargetDefinition
	for _, name := range targetNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		t, err := FindTarget(name)
		if err != nil {
			return nil, err
		}
		res = append(res, *t)
	}
	return res, nil
}
