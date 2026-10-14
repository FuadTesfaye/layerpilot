# Contributing to `compat`

Thank you for contributing to the Universal App Compatibility Tool.

## Development Requirements
- Go 1.23 or newer
- Rootless Podman 4.5+ (for Linux container engine tests)
- `make`

## Development Workflow
1. Fork and clone the repository.
2. Build the project:
   ```bash
   make build
   ```
3. Run tests with race detection:
   ```bash
   make test
   ```
4. Verify code formatting:
   ```bash
   make lint
   ```

## Commit Guidelines
We adhere to Conventional Commits:
- `feat:` New features or engine adapters
- `fix:` Bug fixes or limitation updates
- `test:` Unit, contract, and integration tests
- `docs:` Documentation improvements
- `refactor:` Code cleanup without semantic behavior changes
