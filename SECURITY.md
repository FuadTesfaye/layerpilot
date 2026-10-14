# Security Policy

## Threat Model & Security Boundaries
`compat` manages and executes third-party applications across compatibility layers.

### Mandatory Security Principles
1. **No Silent Privilege Escalation**: Operations default to unprivileged, rootless user environments. Host administrator elevation is never requested silently.
2. **Deterministic Process Execution**: All subprocesses use parameter arrays (`exec.CommandContext`) with zero shell string interpolation.
3. **Transparent Isolation**: Containers and prefixes are isolation mechanisms for packaging and library dependencies. Applications sharing desktop display servers (X11/Wayland) or audio servers (PulseAudio/PipeWire) have access to those communication buses. `compat` does not claim military-grade malware containment.
4. **Input Sanitization**: Archive extraction incorporates path traversal checks (`../` escapes) and decompression size limits.

## Reporting a Vulnerability
If you discover a security vulnerability in `compat`, please report it responsibly by contacting:
**fuadtesfaye24@gmail.com**

Please provide:
- A description of the vulnerability and attack scenario.
- Steps to reproduce or proof-of-concept.
- Potential impact on host systems.

We will acknowledge receipt within 48 hours and work with you to release a patch.
