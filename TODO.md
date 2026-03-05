# vdiff-tui — Implementation Plan

TUI equivalent of [vdiff-electron](../vdiff-electron), built with **Go + Bubble Tea**. Designed for reviewing git diffs and exporting line-level comments as prompts for AI coding agents.

## Tech Stack

- **Language**: Go 1.26
- **TUI Framework**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-architecture)
- **Styling**: [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **Clipboard**: [atotto/clipboard](https://github.com/atotto/clipboard)
- **Diff parsing**: Shell out to `git` CLI, parse unified diff output
- **Syntax highlighting**: [Chroma](https://github.com/alecthomas/chroma)

---

## Phase 1: Project Scaffolding & CLI

- [ ] Initialize Go module (`go mod init`)
- [ ] Set up `main.go` entry point with Bubble Tea program
- [ ] CLI argument parsing: `vdiff [path]` (default to cwd)
- [ ] `--help` / `-h` flag
- [ ] `--version` / `-v` flag
- [ ] Validate that target directory is a git repo (run `git rev-parse --git-dir`)
- [ ] Graceful error messages for non-git dirs, missing git binary, etc.
- [ ] Basic Bubble Tea model with Init/Update/View wired up
- [ ] Clean quit on `q` and `Ctrl+C`

## Phase 2: Git Integration

- [ ] Execute `git status --porcelain=v1 -uall` and parse output
- [ ] Map porcelain codes to file statuses: `A`, `M`, `D`, `R`, `?` (untracked → `A`)
- [ ] Execute `git diff` (unstaged), `git diff --cached` (staged), `git diff --no-index /dev/null <file>` (untracked)
- [ ] Parse unified diff output into structured types:
  - `DiffFile` — file path, status, hunks, add/delete counts
  - `Hunk` — header string, start lines, list of `DiffLine`
  - `DiffLine` — type (add/delete/context), old line num, new line num, content
- [ ] Execute `git rev-parse --abbrev-ref HEAD` for current branch name
- [ ] Handle edge cases: binary files, empty files, renamed files, permission changes

## Phase 3: Layout & Navigation

### Header Bar
- [ ] Render top bar with: app name, repo folder name, branch name
- [ ] Lip Gloss styling (dark theme matching GitHub Dark: bg `#0d1117`, text `#e6edf3`)

### File List (Left Pane)
- [ ] Render list of changed files with status indicator and +/- counts
- [ ] Highlight currently selected file
- [ ] `j`/`k` or `↑`/`↓` to move selection
- [ ] `Enter` to open file diff in main pane
- [ ] Comment count badge per file (e.g., `[2]` next to filename)

### Diff Viewer (Right Pane)
- [ ] Render selected file's diff with hunk headers
- [ ] Display old + new line numbers in gutter columns
- [ ] Color-coded lines: green additions, red deletions, default context
- [ ] Scroll with `j`/`k`, `↑`/`↓`, `PgUp`/`PgDn`
- [ ] Jump to top/bottom with `g`/`G`
- [ ] Jump between hunks with `n`/`N`
- [ ] Cursor/highlight bar on current line

### Pane Management
- [ ] `Tab` / `Shift+Tab` to switch focus between file list and diff viewer
- [ ] Visual border/highlight change to indicate active pane
- [ ] Responsive layout — panes resize with terminal window

## Phase 4: Syntax Highlighting

- [ ] Integrate Chroma for terminal-compatible syntax highlighting
- [ ] Map file extensions to Chroma lexers (same set as Electron version):
  - JS/TS: `.js`, `.jsx`, `.mjs`, `.cjs`, `.ts`, `.tsx`
  - Python: `.py`
  - Go: `.go`
  - Rust: `.rs`
  - Ruby: `.rb`, `Gemfile`, `Rakefile`
  - And others: Java, C, C++, C#, PHP, Swift, Kotlin, Bash, JSON, XML, CSS, SCSS, SQL, YAML, Markdown, Dockerfile, Makefile
- [ ] Apply highlighting to diff line content (preserving add/delete background colors)
- [ ] Fall back to plain text for unknown file types

## Phase 5: Line Selection & Commenting

### Line Selection
- [ ] `c` on a diff line to select it and open comment input
- [ ] `v` to enter visual/range mode, then `j`/`k` to extend selection, `c` to comment
- [ ] `Escape` to cancel selection
- [ ] Visual highlight on selected lines (distinct color, e.g., blue)

### Comment Input
- [ ] Inline text input area below selected line(s)
- [ ] Multi-line text entry with `Enter` for newlines
- [ ] `Ctrl+S` to save comment
- [ ] `Escape` to cancel and discard
- [ ] Show the selected code snippet above the input for context

### Comment Display
- [ ] Render saved comments inline below their associated diff lines
- [ ] Visual gutter indicator on commented lines (purple/magenta marker)
- [ ] Show comment text with line number reference
- [ ] `e` on a comment to edit it (re-opens input with existing text)
- [ ] `d` on a comment to delete it (with brief confirmation or undo flash)

### Comment Data Model
```go
type Comment struct {
    ID       int      `json:"id"`
    FilePath string   `json:"filePath"`
    LineIDs  []string `json:"lineIds"`   // "hunkIdx-lineIdx"
    LineNum  string   `json:"lineNum"`   // "42" or "40-45"
    Code     string   `json:"code"`      // selected line(s) content
    Text     string   `json:"text"`      // user's comment
}
```

## Phase 6: Comment Persistence

- [ ] Store comments in `~/.config/vdiff/comments.json`
- [ ] Format: `{ "/path/to/repo": [comments] }`
- [ ] Auto-save on every add/update/delete operation
- [ ] Load comments for active repo on startup
- [ ] Auto-prune: remove comments for files no longer showing in diff (already committed)
- [ ] Create config directory if it doesn't exist

## Phase 7: Prompt Export

### Export Panel
- [ ] `p` toggles bottom panel showing formatted prompt output
- [ ] Panel takes ~1/3 of screen height, pushes diff viewer up
- [ ] `Escape` closes panel

### Export Formats
- [ ] Standard format (with code snippets):
  ```
  Address the following feedback:

  - path/to/file.js:42
     Code: const x = 5;
     Comment: This should be const.

  - src/utils.js:10-12
     Code:
       function helper() {
         return value;
       }
     Comment: Refactor this
  ```
- [ ] Compact format (without code):
  ```
  Address the following feedback:

  - path/to/file.js:42
    - This should be const.
  ```
- [ ] `f` to toggle between formats
- [ ] `y` to copy to system clipboard (via atotto/clipboard)
- [ ] Flash "Copied!" message in status bar for ~2 seconds after copy

## Phase 8: Refresh & Status

- [ ] `r` to manually refresh file list and branch
- [ ] Auto-refresh on SIGCONT (when returning from Ctrl+Z / `fg`)
- [ ] Prune stale comments on refresh
- [ ] Status bar at bottom showing: mode indicator, key hints, flash messages

## Phase 9: Help & Polish

- [ ] `?` to show/hide keybinding help overlay
- [ ] Help overlay content:
  | Key | Action |
  |---|---|
  | `j`/`k`, `↑`/`↓` | Navigate |
  | `Tab` | Switch pane |
  | `Enter` | Select file |
  | `g`/`G` | Top / Bottom |
  | `n`/`N` | Next / Prev hunk |
  | `c` | Comment on line |
  | `v` | Visual select (range) |
  | `e` | Edit comment |
  | `d` | Delete comment |
  | `Ctrl+S` | Save comment |
  | `Escape` | Cancel / Close |
  | `p` | Toggle prompt panel |
  | `y` | Copy prompt to clipboard |
  | `f` | Toggle compact format |
  | `r` | Refresh |
  | `q` | Quit |
  | `?` | Help |
- [ ] `--compact` CLI flag to start in compact mode
- [ ] Graceful terminal resize handling
- [ ] Clean alternate screen buffer usage (enter on start, exit on quit)

---

## Layout Reference

```
┌─ vdiff ── repo: my-project ── branch: feature/xyz ──────────┐
│ Changed Files  │  Diff Viewer                                │
│                │                                             │
│ M src/app.js   │  @@ -10,6 +10,8 @@                        │
│ A src/new.js   │   10  10  context line                     │
│ D old.js       │   11      - removed line                   │
│                │       11  + added line                      │
│                │  ┃ 💬 "Fix this variable name"              │
│                │   12  12  context line                      │
│                │                                             │
├────────────────┴─────────────────────────────────────────────┤
│ [Prompt Output]  (toggle with 'p')                           │
│ Address the following feedback:                              │
│ - src/app.js:11                                              │
│   Code: + added line                                         │
│   Comment: Fix this variable name                            │
├──────────────────────────────────────────────────────────────┤
│ mode: NORMAL | Tab: switch pane | c: comment | ?: help       │
└──────────────────────────────────────────────────────────────┘
```
