# CLAUDE.md

## Project Overview

vdiff-tui is a terminal UI for reviewing git diffs and exporting line-level comments as prompts for AI coding agents. It's a Go + Bubble Tea rewrite of the Electron-based vdiff app.

## Tech Stack

- **Go 1.26** (managed via mise — see `mise.toml`)
- **Bubble Tea** — Elm-architecture TUI framework
- **Lip Gloss** — terminal styling
- **Chroma** — syntax highlighting (not yet integrated)
- **atotto/clipboard** — clipboard access (not yet integrated)

## Build & Run

```bash
go build -o vdiff .
./vdiff [path-to-repo]
```

Or directly:

```bash
go run . [path-to-repo]
```

## Project Structure

- `main.go` — entry point, CLI parsing, git validation, Bubble Tea model

## Implementation Plan

See `TODO.md` for the phased implementation plan. Currently on Phase 2.

## Project Structure

- `main.go` — entry point, CLI parsing, git validation, Bubble Tea model
- `git/` — git command execution (status, diff, branch)
- `diff/` — unified diff parser (File/Hunk/Line types)

## Conventions

- All exported and unexported functions must have a `//` doc comment directly above the declaration, starting with the function name
- All exported types and constants must have doc comments
- Packages must have a `// Package <name> ...` doc comment above the `package` declaration
- Shell out to `git` CLI for all git operations (no libgit2)
- GitHub Dark theme colors: bg `#0d1117`, secondary `#161b22`, text `#e6edf3`, muted `#8b949e`
- Commit messages should be concise and describe the "why"
