# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build        # Build cloudlogin binary (CGO_ENABLED=1 required for Fyne)
make run          # Build and run
make test         # go test -v ./...
make deps         # go mod download && go mod tidy
make clean        # Remove binaries

# Single test
go test -v ./pkg/awsconfig/... -run TestFunctionName

# Release (creates GitHub Release via GoReleaser)
make release      # Requires GITHUB_TOKEN
make test-release # Local snapshot, no publish
```

CGO must be enabled (`CGO_ENABLED=1`) because the Fyne GUI framework requires it.

## Architecture

**Multi-mode tool**: One binary, three UI modes (GUI, CLI, TUI). All core logic lives in `pkg/`, shared by all modes.

### Entry Points
- `main.go` — dispatches to GUI (default), CLI (`--update-aws-config`, `--sanitize-aws-config`), or exits
- `cmd/awsconfig-tui/main.go` — standalone TUI binary built with Bubble Tea (MVU pattern)

### Core Package: `pkg/awsconfig/`
All AWS SSO logic lives here. New features should be added here first, then integrated into each mode:
- `sso_api.go` — AWS CLI wrapper: reads SSO cache tokens, enumerates accounts/roles, calls `aws` via `exec`
- `config.go` — reads/writes `~/.aws/config`; `UpdateFromSSO()` is the main entrypoint
- `helpers.go` — env filtering, sensitive token masking for logs
- `types.go` — shared structs; contains hardcoded SSO Start URL and region (`lynqtech.awsapps.com/start`, `eu-central-1`)

### GUI Layer (Fyne, root package)
- `gui.go` — main window with AWS/Kubernetes/Quick Actions tabs + log console
- `aws_profile.go` — AWS profile parsing and SSO login
- `kube_context.go` — Kubernetes context switching
- `sync_aws_kube.go` — cross-tool synchronization
- `shell.go` — shell command execution helpers

### Feature Integration Pattern
When adding a new feature:
1. Implement in `pkg/awsconfig/` with signature: `func FeatureName(logFn func(string)) error`
2. Add CLI flag case in `main.go`
3. Add GUI button/action in `gui.go`
4. Add TUI command in `cmd/awsconfig-tui/main.go`

## Code Conventions

**Logging**: Emoji-prefixed log messages via `logFn` callback (never `fmt.Println`):
- `🔍` searching/reading, `✅` success, `❌` error, `ℹ️` info, `⚠️` warning

**Sensitive data**: Access tokens must be masked in logs — use `maskSensitiveArgs()` from `helpers.go`.

**Config backup**: Always create `.bak` before writing `~/.aws/config`.

**File size**: Keep files under ~150 lines. Split into subfiles rather than growing existing ones.

## Dependencies

- `fyne.io/fyne/v2` — GUI (requires CGO + system graphics libs on Linux)
- `github.com/charmbracelet/bubbletea` — TUI
- AWS CLI and kubectl must be installed on the system (called via `exec`)

## Release Process

Git tag matching `v*` triggers GitHub Actions which runs GoReleaser in parallel for macOS and Linux (separate `.goreleaser.yaml` and `.goreleaser-linux.yaml`). Linux builds require additional system packages (`libgl1-mesa-dev`, etc.).
