# vdiff

A TUI for reviewing git diffs and exporting line-level comments as prompts for AI coding agents.

Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Terminal equivalent of [vdiff-electron](https://github.com/erikbelusic/vdiff-electron).

## Install

```bash
go install github.com/erikbelusic/vdiff-tui@latest
```

Or build from source:

```bash
git clone https://github.com/erikbelusic/vdiff-tui.git
cd vdiff-tui
go build -o vdiff .
```

## Usage

```bash
vdiff [path]    # Open TUI at given repo (default: current directory)
vdiff --help    # Show usage
vdiff --version # Show version
```

## Development

Requires Go 1.26+ (managed via [mise](https://mise.jdx.dev/)) and Git.

```bash
git clone https://github.com/erikbelusic/vdiff-tui.git
cd vdiff-tui
mise install          # installs Go 1.26 from mise.toml
go run .              # run directly
go run . /path/to/repo  # run against a specific repo
go build -o vdiff .   # build binary
```
