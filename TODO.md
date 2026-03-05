# vdiff-tui — Feature Spec

TUI equivalent of [vdiff-electron](../vdiff-electron), designed for reviewing git diffs and exporting line-level comments as prompts for AI coding agents.

---

## Core Features

### 1. Repository Management
- [ ] Accept repo path as CLI argument, default to cwd
- [ ] Validate directory is a git repository on startup
- [ ] Display current branch name in header/status bar
- [ ] Auto-refresh file list when returning from background (SIGCONT / terminal focus)

### 2. Changed Files List (Sidebar)
- [ ] Show all uncommitted changes (staged + unstaged + untracked)
- [ ] File status indicators: `A` Added, `M` Modified, `D` Deleted, `R` Renamed
- [ ] Per-file addition/deletion counts (+N / -N)
- [ ] Keyboard navigation (j/k or arrows) to select file
- [ ] Visual highlight on selected file
- [ ] Comment count badge per file (files that have comments)

### 3. Diff Viewer (Main Pane)
- [ ] Unified diff display with hunk headers
- [ ] Line numbers (old + new) in gutters
- [ ] Color-coded lines: green for additions, red for deletions, default for context
- [ ] Syntax highlighting (via terminal ANSI colors)
- [ ] Scrollable with keyboard (j/k, page up/down, g/G for top/bottom)
- [ ] Hunk-to-hunk jumping (n/N or similar)

### 4. Line Selection & Commenting
- [ ] Select a single line (Enter or keybind on a diff line)
- [ ] Select a range of lines (visual select / shift+arrows / mark start + mark end)
- [ ] Open comment input (text area / inline editor) for selected lines
- [ ] Save comment: Ctrl+S or Enter (configurable)
- [ ] Cancel comment: Escape
- [ ] Visual indicator on commented lines (colored gutter mark or highlight)
- [ ] View existing comments inline below their associated lines
- [ ] Edit existing comment (select commented line, press edit key)
- [ ] Delete comment (keybind on comment)

### 5. Comment Persistence
- [ ] Store comments per-repository in `~/.config/vdiff/comments.json`
- [ ] Auto-save on every add/update/delete
- [ ] Auto-prune comments for files no longer in the diff (already committed)
- [ ] Load comments on startup for the active repo

### 6. Prompt Export (Comment Output)
- [ ] Toggle export panel (bottom pane or full-screen overlay)
- [ ] Standard format (with code snippets):
  ```
  Address the following feedback:

  - path/to/file.js:42
     Code: const x = 5;
     Comment: This should be const.
  ```
- [ ] Compact format (without code):
  ```
  Address the following feedback:

  - path/to/file.js:42
    - This should be const.
  ```
- [ ] Toggle between standard/compact with a keybind
- [ ] Copy to system clipboard
- [ ] Visual confirmation after copy ("Copied!" flash message)

### 7. Layout
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
└──────────────────────────────────────────────────────────────┘
```

### 8. Keybindings
| Action | Key |
|---|---|
| Navigate files | `Tab` / `Shift+Tab` to switch panes |
| Select file | `Enter` in file list |
| Scroll diff | `j`/`k`, arrows, `PgUp`/`PgDn` |
| Top/bottom of diff | `g` / `G` |
| Next/prev hunk | `n` / `N` |
| Select line for comment | `c` on a diff line |
| Start range select | `v` (visual mode, vim-style) |
| Save comment | `Ctrl+S` |
| Cancel/close | `Escape` |
| Toggle prompt panel | `p` |
| Copy prompt to clipboard | `y` (when panel open) |
| Toggle compact mode | `f` (format toggle) |
| Refresh files | `r` |
| Quit | `q` |
| Help overlay | `?` |

### 9. CLI Interface
- [ ] `vdiff [path]` — open TUI at given repo (default: cwd)
- [ ] `--help` / `-h` — usage info
- [ ] `--version` / `-v` — print version
- [ ] `--compact` — start in compact export mode

---

## Tech Stack Options

### Option A: Go + Bubble Tea (Recommended)
- **Language**: Go
- **TUI Framework**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-architecture TUI framework)
- **Styling**: [Lip Gloss](https://github.com/charmbracelet/lipgloss) (terminal CSS-like styling)
- **Diff parsing**: Shell out to `git` + parse unified diff
- **Syntax highlighting**: [Chroma](https://github.com/alecthomas/chroma) (Go port of Pygments)
- **Clipboard**: [atotto/clipboard](https://github.com/atotto/clipboard)
- **Pros**: Single binary, fast startup, excellent TUI ecosystem (Charm tools), cross-platform, battle-tested by tools like `lazygit`, `glow`, `soft-serve`
- **Cons**: More verbose than scripting languages

### Option B: Rust + Ratatui
- **Language**: Rust
- **TUI Framework**: [Ratatui](https://github.com/ratatui/ratatui) (immediate-mode TUI)
- **Diff parsing**: Shell out to `git` + parse unified diff
- **Syntax highlighting**: [syntect](https://github.com/trishume/syntect) or [tree-sitter](https://github.com/tree-sitter/tree-sitter)
- **Clipboard**: [arboard](https://github.com/1Password/arboard)
- **Pros**: Maximum performance, strong type safety, single binary, great for complex state
- **Cons**: Steeper learning curve, slower iteration, more boilerplate for UI layout

### Option C: Python + Textual
- **Language**: Python
- **TUI Framework**: [Textual](https://github.com/Textualize/textual) (modern Python TUI with CSS-like styling)
- **Diff parsing**: `subprocess` to `git` + parse
- **Syntax highlighting**: Built-in (uses Rich/Pygments)
- **Clipboard**: [pyperclip](https://github.com/asweigart/pyperclip)
- **Pros**: Fastest to build, great docs, CSS-like layout system, built-in syntax highlighting, hot reload during dev
- **Cons**: Requires Python runtime (not a single binary without extra tooling), slower startup

### Option D: TypeScript + Ink
- **Language**: TypeScript
- **TUI Framework**: [Ink](https://github.com/vadimdemedes/ink) (React for CLIs)
- **Diff parsing**: `child_process` to `git` (same approach as Electron version)
- **Syntax highlighting**: highlight.js or cli-highlight
- **Clipboard**: clipboardy
- **Pros**: Can reuse logic from vdiff-electron (diff parser, comment export, etc.), React mental model, fast iteration
- **Cons**: Requires Node.js runtime, Ink has limitations for complex layouts, less mature for full-screen TUIs
