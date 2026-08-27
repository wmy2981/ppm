# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ppm (Portproxy Manager) — a k9s-style TUI for managing Windows `netsh interface portproxy` IPv4 rules. Built with Go and Charm's Bubble Tea framework. Requires administrator privileges (UAC elevation is automatic).

## Build Commands

```bash
# Local build
go build -o ppm.exe ./cmd/ppm

# Release build (injects version from VERSION file)
go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o ppm.exe ./cmd/ppm

# PowerShell build script
./scripts/build.ps1
```

No test suite exists (`_test.go` files are absent). No linter config is present.

## CLI Commands

ppm supports both TUI and CLI modes. Running `ppm` with no arguments launches the TUI; subcommands provide headless/scriptable access.
Positional listen args accept `:8080` (defaults to `0.0.0.0:8080`) or `ip:port`.
Flags (`--listen`, `--connect`, `--note`) can be used as alternatives to positional args.
Use `ppm <command> --help` for detailed usage.

## Critical Constraints

- **Windows-only**: Uses `shell32.dll`/`user32.dll` syscalls, `netsh`, and `netstat`. Will not compile on Linux/macOS.
- **Admin required**: In TUI mode, the app UAC-elevates on startup. In CLI mode, write commands require admin; `netsh` rule changes fail without admin.
- **GBK console decoding**: `netsh`/`netstat` output on Chinese Windows is GBK-encoded; the code in `internal/netsh` auto-detects and converts to UTF-8 via `golang.org/x/text`.
- **Plain text in table cells**: Do not use ANSI-styled text inside `bubbles/table` cells — it causes column misalignment. All cell values must remain plain strings (`internal/ui/model.go`).

## Architecture

```
cmd/ppm/main.go           — Entrypoint; routes to TUI or CLI based on args
internal/cli/              — urfave/cli app definition and command handlers (CLI mode)
internal/elevate/          — UAC self-elevation via ShellExecuteW
internal/netsh/            — netsh/netstat command wrappers; all commands have 10s timeouts
internal/store/            — %APPDATA%\ppm persistence (notes.json + backup imports)
internal/ui/model.go       — Bubble Tea TUI model (list, form, confirm views)
internal/ui/form.go        — Form, import, and delete-confirm sub-views
```

Key behaviors:
- **TUI ↔ CLI**: `ppm` (no args) launches TUI; `ppm <subcommand>` routes to CLI. `ppm tui` explicitly launches TUI.
- **Edit = delete + create**: `netsh` has no in-place portproxy update; editing deletes the old rule and creates a new one, with best-effort rollback on failure.
- **Import dedup**: Backup import skips rules whose `listenaddr:listenport` key already exists live.
- **Connectivity test**: Dials the target with a 3-second TCP timeout.
- **Note column width**: Computed dynamically from terminal width, clamped to [10, 40].

## Version Injection

`cmd/ppm/main.go` declares `version = "dev"` by default. The release build overrides it via `-ldflags "-X main.version=..."`. CI reads `VERSION` to create git tags and GitHub Releases.

## Commit Convention

Conventional Commits format: `type(scope): lowercase description` in English (e.g. `feat:`, `docs(readme):`).
