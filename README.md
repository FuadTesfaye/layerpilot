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

### Install Strategy Ladder (Safest → Most Invasive)
`compat` picks the first viable, safest strategy for the package and host:
- **S1 — User-local, no root**: AppImages to `~/Applications` or tarballs (`.tar.gz`, `.tar.xz`, `.zip`) extracted to `~/.local/opt/<app>`, wrapper in `~/.local/share/compat/bin` and desktop entry.
- **S2 — User-local FUSE fallback**: If `libfuse2` is absent, automatically applies `--appimage-extract-and-run` or extracts permanently to avoid root dependencies.
- **S3 — Native package install**: For packages matching the host distro (`.rpm` on Fedora, `.deb` on Debian, `.pkg.tar.zst` on Arch, `.rpm` on openSUSE). Shows exact single command and requires approval (`pkexec dnf/apt/pacman/zypper`).
- **S4 — Desktop bundle install**: User-scope Flatpak (rootless) or Snap bundle (`pkexec snap install --dangerous`).
- **S5 — Container fallback**: Foreign-family packages (`.deb` on Fedora, `.rpm` on Debian) run inside managed, rootless Podman containers to prevent host library pollution.
- **S6 — Refuse with actionable explanation**: macOS `.dmg`/`.app`, kernel modules, or self-extracting `.run`/`.sh` scripts (Tier 3, header previewed, explicit confirmation required).

---

## 3. What is explicitly NOT supported?
1. **Running macOS binaries on Linux or Windows**: macOS binaries require Apple proprietary frameworks and Darwin kernel interfaces that are not supported.
2. **Arbitrary kernel-mode drivers**: Applications requiring custom host kernel modules, anti-cheat drivers, or low-level ring-0 hardware access.
3. **Unrestricted privilege escalation**: `compat` will never silently execute root host commands.

---

## 4. Prerequisites

### Linux Host
- Linux kernel 5.10+ (cgroups v2 recommended)
- Rootless Podman 4.5+ (optional, for foreign package S5 container isolation)
- `fuse3` / `libfuse2` (optional, for direct AppImage mounting; S2 extracts automatically if missing)

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
# 1. Inspect a package without making any system changes (static ELF scan, GLIBC versions, deps)
compat inspect ./example.AppImage

# 2. Check applicable strategies and support tier
compat support ./example.AppImage

# 3. Preview the deterministic execution plan
compat plan ./example.AppImage

# 4. Install the package (with explicit user approval)
compat install ./example.AppImage

# 5. List managed applications
compat list

# 6. Launch an installed application
compat launch <app-id> -- --flag value

# 7. One-shot execution in temporary sandbox without permanent install
compat run ./example.AppImage

# 8. Check application runtime status and logs
compat status <app-id>
compat logs <app-id>

# 9. Diagnose runtime crashes and apply verified remedies
compat fix <app-id>

# 10. Roll back an applied compatibility fix
compat undo <app-id>

# 11. Cleanly uninstall and remove all owned resources
compat uninstall <app-id>
```

---

## 7. Auto-Fix Engine & Pattern Library

When an application fails to launch or hangs without rendering a window (e.g. Electron apps on Wayland), `compat` provides an automated diagnostic and remediation workflow:

1. **Failure Classification**: Evaluates runtime logs against embedded YAML diagnostic patterns while filtering harmless noise (updater 404s, DBus warnings).
2. **Ranked Remedies**: Presents least-invasive fixes first (e.g. wrapper flags like `--ozone-platform=x11`, permanent extraction, or container isolation).
3. **Active Verification**: Before declaring a fix successful, `compat` re-launches the application and verifies the process stays alive $\ge 10\text{s}$ AND active window mapping is confirmed via `_NET_CLIENT_LIST` or Wayland compositor IPC.
4. **Automatic Rollback**: If verification fails, changes are reverted to the pre-fix backup.
5. **Undo Anytime**: `compat undo <app-id>` restores previous launcher configuration tracked in SQLite.

### Diagnostic Patterns Included
- **P1 (Electron Wayland Blank Window)**: Detects GPU/Ozone bitmap crashes on Wayland; remediates with `--ozone-platform=x11` or `--disable-gpu`.
- **P2 (Missing FUSE)**: Detects `libfuse.so.2` missing; remediates with `--appimage-extract-and-run` or user-local extraction.
- **P3 (SELinux AVC Denial)**: Detects SELinux policy blocks; remediates with `restorecon -v` or `chcon -t bin_t` (never disables SELinux).
- **P4 (Older Host GLIBC)**: Detects `GLIBC_X.Y not found`; falls back to containerized execution matching binary glibc requirements.
- **P5 (Chrome Sandbox / Userns)**: Detects unprivileged user namespace blocks; remediates with sysctl elevation or rootless container keep-id.
- **P6 (Missing Shared Library)**: Detects `error while loading shared libraries: libX.so.Y`; maps to native package manager install command.

`compat doctor` checks host virtualization, user namespaces (`subuid`/`subgid`), display servers, container runtimes, and local storage health:

```bash
compat doctor
```

Deep engine diagnosis:
```bash
compat engine doctor podman
```

---

## 8. `compat lab`: Testing Linux Apps Without a Linux Machine

`compat lab` is built for developers on macOS, Windows, or Linux who build software targeting Linux. It tests artifacts (AppImage, native binary, `.deb`, `.rpm`, tarball) across pinned Linux distributions (Fedora, Ubuntu, Debian, Arch, openSUSE), evaluates honest multi-tier verdicts, and outputs an AI-agent context bundle (`AGENT_CONTEXT.md`) enabling autonomous fixing in a bounded loop.

### Testing Fidelity Tiers

| Tier | Environment | Capabilities | Limitations |
| :--- | :--- | :--- | :--- |
| **T0 (Static)** | Static ELF Scan | Glibc symbol check, dynamic dependency prediction, framework profile | No execution; cannot verify runtime behavior |
| **T1 (Container)** | Rootless Container (Podman / Docker) | Smoke tests, dynamic loader resolution (`ldd`), offline resiliency, headless Xvfb window mapping | Cannot verify native Wayland compositor protocols, SELinux enforcing denials, or host GPU rendering |
| **T2 (VM)** | Lima / WSL2 / Multipass | Kernel confinement, full init systems, virtualized graphics | Slower startup; requires virtualization (KVM / Hyper-V) |
| **T3 (Remote/Host)** | Real Bare-Metal / Remote Host | Native Wayland compositor mapping, physical GPU passthrough, host SELinux | Requires physical hardware |

> [!IMPORTANT]
> **Honest Fidelity Principle**: A T1 container pass **never** claims "works on Fedora desktop". When host-dependent conditions are met (e.g. Electron Wayland blank window P1), `compat lab` reports `not-testable-at-this-tier` instead of reporting a false pass.

### Usage

```bash
# 1. Check container runtimes, virtualization, and host resources
compat lab doctor

# 2. Run the test matrix across pinned distros
compat lab run ./dist/myapp.AppImage

# 3. View the human-readable summary
compat lab report compat-report/

# 4. Pipe structured AI brief into an autonomous coding agent
compat lab context compat-report/ | your-coding-agent

# 5. Verify an applied AI fix with zero regressions
compat lab verify-fix compat-report/ --scenario smoke --artifact ./dist/myapp-fixed.AppImage
```

---

## 9. Security & Isolation Boundaries

- **Zero Shell Interpolation**: Subprocesses are executed via argument arrays (`exec.CommandContext`). Shell string expansion (`sh -c`) is strictly forbidden.
- **Rootless by Default**: Containerized applications run in user namespaces without host root privileges.
- **Resource Ownership Tracking**: Every created directory, container, image, and desktop file is registered in SQLite. Cleanups touch only resources owned by `compat`.
- **Desktop Sockets**: For GUI integration, Wayland or X11 sockets are selectively mounted. Managed containers provide dependency and environment isolation, **not an impenetrable sandbox against hostile code**.

---

## 10. Tested Applications Catalogue

| Application | Package Format | Tested Host | Strategy | Outcome |
| :--- | :--- | :--- | :--- | :--- |
| **htop** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container | Verified |
| **neofetch** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container | Verified |
| **kcalc** | `.deb` (amd64) | Fedora 44 (x86_64) | Rootless Container (GUI) | Verified |

---

## 11. Contributing & Security Reporting

- View [CONTRIBUTING.md](CONTRIBUTING.md) for development workflows and testing requirements.
- View [SECURITY.md](SECURITY.md) for our vulnerability disclosure policy and threat model.
- License: [MIT](LICENSE)
