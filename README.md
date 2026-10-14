# Universal App Compatibility Tool (`compat`)

A single Go-based interface that detects an application package, evaluates the host environment, selects an appropriate supported execution strategy, manages the application lifecycle, and explains failures in actionable terms.

[![Go Report Card](https://goreportcard.com/badge/github.com/FuadTesfaye/layerpilot)](https://goreportcard.com/report/github.com/FuadTesfaye/layerpilot)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 1. What problem does this tool solve?
Users encounter fragmented tools, complex configuration steps, incompatible package formats, missing runtime dependencies, and cryptic failures when running software outside its native target environment.

`compat` unifies the workflow—**not by pretending every operating system is identical**, but by providing a single, safe, explainable lifecycle:
`inspect` ➔ `plan` ➔ `install` ➔ `launch` ➔ `diagnose` ➔ `uninstall`.

---

## 2. Honest Compatibility Matrix

Compatibility is evaluated across 4 explicit tiers:

| Host OS | Guest / Application Environment | Target Engine | Support Tier | Current Status |
| :--- | :--- | :--- | :--- | :--- |
| **Linux (Fedora, Debian, Ubuntu, etc.)** | Applications for another Linux distro (`.deb`, `.rpm`) | Rootless Podman Container | **Tier 1 — Supported** | Active |
| **Linux (All distributions)** | AppImage applications (`.AppImage`) | Direct Sandboxed Execution | **Tier 1 — Supported** | Active |
| **Linux** | Windows applications (`.exe`, `.msi`) | Wine prefix manager | **Tier 3 — Experimental** | Testing |
| **Linux / Non-Apple** | macOS applications (`.dmg`, `.pkg`, `.app`) | N/A | **Tier 4 — Unsupported** | Refused |
| **Windows** | Linux CLI / GUI applications | WSL2 / WSLg | **Tier 2 — Limited** | Architecture Ready |
| **macOS** | Linux CLI applications | Lima Linux VM | **Tier 2 — Limited** | Architecture Ready |

### Support Tier Definitions
- **Tier 1 — Supported**: The host, architecture, package format, engine, and critical workflow are verified through automated integration tests. Eligible for bug fixes.
- **Tier 2 — Supported with limitations**: The strategy is validated, but documented limitations exist (e.g. desktop socket bridging, performance, headless constraints).
- **Tier 3 — Experimental**: Best-effort only. Requires explicit user acknowledgement prior to execution.
- **Tier 4 — Unsupported**: The strategy planner explicitly refuses execution and explains why. No misleading force flags.

---

## 3. What is explicitly NOT supported?
1. **Running macOS binaries on Linux or Windows**: macOS binaries require Apple proprietary frameworks and Darwin kernel interfaces that are not supported.
2. **Arbitrary kernel-mode drivers**: Applications requiring custom host kernel modules, anti-cheat drivers, or low-level ring-0 hardware access.
3. **Unrestricted privilege escalation**: `compat` will never silently execute root host commands.

---

## 4. Prerequisites

### Linux Host
- Linux kernel 5.10+ (cgroups v2 recommended)
- Rootless Podman 4.5+ (already installed on modern Fedora, Ubuntu 22.04+, Debian 12+)
- `fuse3` / `libfuse2` (optional, for direct AppImage mounting)

---

## 5. Installation

### Option A: Automated One-Line Install (Linux & macOS)
```bash
curl -fsSL https://raw.githubusercontent.com/FuadTesfaye/layerpilot/main/install.sh | bash
```

### Option B: Via `go install`
```bash
go install github.com/FuadTesfaye/layerpilot/cmd/compat@latest
```

### Option C: Download Pre-built Binary
Download the pre-compiled binary for your OS and architecture from [GitHub Releases](https://github.com/FuadTesfaye/layerpilot/releases/latest).

### Option D: Build from Source
```bash
git clone https://github.com/FuadTesfaye/layerpilot.git
cd layerpilot
make build
make install
```

---

## 6. Quick Start & Workflow

```bash
# 1. Inspect a package without making any system changes
compat inspect ./example.deb

# 2. Check applicable strategies and support tier
compat support ./example.deb

# 3. Preview the deterministic execution plan
compat plan ./example.deb

# 4. Install the package (with explicit user approval)
compat install ./example.deb

# 5. List managed applications
compat list

# 6. Launch an installed application
compat launch <app-id> -- --flag value

# 7. Check application runtime status and logs
compat status <app-id>
compat logs <app-id>

# 8. Explain the compatibility strategy and limitations
compat explain <app-id>

# 9. Cleanly uninstall and remove all owned resources
compat uninstall <app-id>
```

---

## 7. Diagnostics (`compat doctor`)

`compat doctor` checks host virtualization, user namespaces (`subuid`/`subgid`), display servers, container runtimes, and local storage health:

```bash
compat doctor
```

Deep engine diagnosis:
```bash
compat engine doctor podman
```

---

## 8. Security & Isolation Boundaries

- **Zero Shell Interpolation**: Subprocesses are executed via argument arrays (`exec.CommandContext`). Shell string expansion (`sh -c`) is strictly forbidden.
- **Rootless by Default**: Containerized applications run in user namespaces without host root privileges.
- **Resource Ownership Tracking**: Every created directory, container, image, and desktop file is registered in SQLite. Cleanups touch only resources owned by `compat`.
- **Desktop Sockets**: For GUI integration, Wayland or X11 sockets are selectively mounted. Managed containers provide dependency and environment isolation, **not an impenetrable sandbox against hostile code**.

---

## 9. Tested Applications Catalogue

| Application | Package Format | Tested Host | Strategy | Outcome |
| :--- | :--- | :--- | :--- | :--- |
| **htop** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container | Verified |
| **neofetch** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container | Verified |
| **kcalc** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container (GUI) | Verified |

---

## 10. Contributing & Security Reporting

- View [CONTRIBUTING.md](CONTRIBUTING.md) for development workflows and testing requirements.
- View [SECURITY.md](SECURITY.md) for our vulnerability disclosure policy and threat model.
- License: [MIT](LICENSE)
