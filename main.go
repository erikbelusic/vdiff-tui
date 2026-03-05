package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"os/signal"
	"syscall"
	"time"

	"github.com/atotto/clipboard"

	"github.com/erikbelusic/vdiff-tui/comments"
	"github.com/erikbelusic/vdiff-tui/diff"
	gitpkg "github.com/erikbelusic/vdiff-tui/git"
	"github.com/erikbelusic/vdiff-tui/highlight"
)

// version is set at build time via -ldflags.
var version = "dev"

// main is the entry point. It parses CLI args, validates the target
// directory is a git repo, and launches the Bubble Tea TUI.
func main() {
	repoPath, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if err := validateGitRepo(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		newModel(repoPath),
		tea.WithAltScreen(),
		tea.WithReportFocus(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running vdiff: %s\n", err)
		os.Exit(1)
	}
}

// startCompact is set by --compact flag during arg parsing.
var startCompact bool

// parseArgs processes CLI arguments and returns the resolved repo path.
// Handles --help, --version, --compact flags and an optional positional path argument.
func parseArgs() (string, error) {
	args := os.Args[1:]
	var positional string

	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		case "--version", "-v":
			fmt.Printf("vdiff %s\n", version)
			os.Exit(0)
		case "--compact":
			startCompact = true
		default:
			if positional == "" && arg != "" && arg[0] != '-' {
				positional = arg
			}
		}
	}

	if positional != "" {
		return filepath.Abs(positional)
	}

	return os.Getwd()
}

// printUsage writes the help text to stdout.
func printUsage() {
	fmt.Println(`Usage: vdiff [path]

A TUI for reviewing git diffs and exporting line-level comments
as prompts for AI coding agents.

Arguments:
  path    Path to a git repository (default: current directory)

Flags:
  -h, --help       Show this help message
  -v, --version    Show version
      --compact    Start in compact export mode`)
}

// validateGitRepo checks that the given path is a directory containing a git repository.
func validateGitRepo(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s is not a git repository", path)
	}

	return nil
}

// pane identifies which UI pane is currently focused.
type pane int

const (
	fileListPane pane = iota
	diffViewPane
)

// mode represents the current interaction mode.
type mode int

const (
	modeNormal  mode = iota
	modeVisual       // selecting a range of lines
	modeComment      // typing a comment
	modeEdit         // editing an existing comment
)


// model is the top-level Bubble Tea model holding all application state.
type model struct {
	repoPath string
	repoName string
	branch   string
	width    int
	height   int

	// File list state
	files         []gitpkg.ChangedFile
	fileIdx       int // currently selected file in file list
	filesErr      string

	// Diff viewer state
	diffFiles     []diff.File
	diffRaw       string
	diffScrollY   int // vertical scroll offset in diff view
	diffCursorY   int // cursor position (line index) in diff view
	diffLines     []diffLine // flattened lines for rendering
	diffErr       string
	highlighter   *highlight.Highlighter

	// Pane focus
	activePane pane

	// Mode and commenting state
	mode          mode
	commentStore  *comments.Store
	visualStart   int      // start of visual selection (diffLines index)
	commentInput  string   // text being typed
	editCommentID int      // ID of comment being edited

	// Prompt panel state
	showPrompt   bool
	compactMode  bool
	showHelp     bool
	flashMsg     string    // temporary status message (e.g., "Copied!")
	flashExpiry  time.Time // when the flash message should disappear
}

// diffLine is a flattened representation of a line in the diff viewer,
// which may be a hunk header or a diff content line.
type diffLine struct {
	isHunkHeader bool
	hunkIdx      int
	lineIdx      int    // index within hunk (for content lines)
	text         string // display text
	oldNum       int
	newNum       int
	lineType     diff.LineType
}

// newModel creates the initial model for the given repository path.
func newModel(repoPath string) model {
	return model{
		repoPath:     repoPath,
		repoName:     filepath.Base(repoPath),
		activePane:   fileListPane,
		mode:         modeNormal,
		commentStore: comments.NewStore(),
		compactMode:  startCompact,
	}
}

// --- Messages ---

// gitDataMsg carries the results of loading git data (branch, files).
type gitDataMsg struct {
	branch string
	files  []gitpkg.ChangedFile
	err    error
}

// fileDiffMsg carries the diff result for a selected file.
type fileDiffMsg struct {
	raw string
	err error
}

// clearFlashMsg signals that a flash message should be cleared.
type clearFlashMsg struct{}

// sigcontMsg signals that the process resumed from a suspend (SIGCONT).
type sigcontMsg struct{}

// --- Commands ---

// loadGitData fetches the current branch and changed files list.
func loadGitData(repoPath string) tea.Cmd {
	return func() tea.Msg {
		branch, _ := gitpkg.CurrentBranch(repoPath)
		files, err := gitpkg.ChangedFiles(repoPath)
		return gitDataMsg{branch: branch, files: files, err: err}
	}
}

// waitForSIGCONT returns a command that waits for a SIGCONT signal (sent when
// the process resumes after Ctrl+Z / fg) and emits a sigcontMsg.
func waitForSIGCONT() tea.Cmd {
	return func() tea.Msg {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGCONT)
		<-sig
		signal.Stop(sig)
		return sigcontMsg{}
	}
}

// loadFileDiff fetches the diff for a specific file.
func loadFileDiff(repoPath string, file gitpkg.ChangedFile) tea.Cmd {
	return func() tea.Msg {
		raw, err := gitpkg.FileDiff(repoPath, file)
		return fileDiffMsg{raw: raw, err: err}
	}
}

// Init returns the initial command to load git data and saved comments on startup.
func (m model) Init() tea.Cmd {
	// Load saved comments (non-blocking, errors are silently ignored)
	_ = m.commentStore.Load(m.repoPath)
	return tea.Batch(loadGitData(m.repoPath), waitForSIGCONT())
}

// Update handles incoming messages (key presses, window resizes) and returns
// the updated model and any commands to execute.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case gitDataMsg:
		if msg.err != nil {
			m.filesErr = msg.err.Error()
		} else {
			m.files = msg.files
			m.branch = msg.branch
			m.filesErr = ""
			// Prune comments for files no longer in the diff
			var paths []string
			for _, f := range m.files {
				paths = append(paths, f.Path)
			}
			m.commentStore.PruneFiles(paths)
			_ = m.commentStore.Save(m.repoPath)
			// Auto-select first file if available
			if len(m.files) > 0 && m.fileIdx == 0 {
				return m, loadFileDiff(m.repoPath, m.files[0])
			}
		}
		return m, nil

	case clearFlashMsg:
		m.flashMsg = ""
		return m, nil

	case sigcontMsg:
		// Process resumed from suspend — refresh and listen again
		return m, tea.Batch(loadGitData(m.repoPath), waitForSIGCONT())

	case tea.FocusMsg:
		// Terminal window/tab regained focus — refresh
		return m, loadGitData(m.repoPath)

	case fileDiffMsg:
		if msg.err != nil {
			m.diffErr = msg.err.Error()
			m.diffFiles = nil
			m.diffLines = nil
			m.highlighter = nil
		} else {
			m.diffRaw = msg.raw
			m.diffFiles = diff.Parse(msg.raw)
			m.diffLines = flattenDiffLines(m.diffFiles)
			m.diffScrollY = 0
			m.diffCursorY = 0
			m.diffErr = ""
			// Create highlighter based on the selected file's path
			if m.fileIdx < len(m.files) {
				m.highlighter = highlight.New(m.files[m.fileIdx].Path)
			}
		}
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input based on the active pane and mode.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Comment/edit mode handles its own input
	if m.mode == modeComment || m.mode == modeEdit {
		return m.handleCommentInput(msg)
	}

	key := msg.String()

	// Visual mode
	if m.mode == modeVisual {
		return m.handleVisualKey(key)
	}

	// Global keybindings (normal mode)
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.showPrompt {
			m.showPrompt = false
			return m, nil
		}
		return m, tea.Quit
	case "tab", "shift+tab", "left", "right":
		m.activePane = togglePane(m.activePane)
		return m, nil
	case "r":
		return m, loadGitData(m.repoPath)
	case "p":
		m.showPrompt = !m.showPrompt
		return m, nil
	case "y":
		if m.showPrompt && m.commentStore.Count() > 0 {
			return m, m.copyPromptToClipboard()
		}
	case "f":
		if m.showPrompt {
			m.compactMode = !m.compactMode
			return m, nil
		}
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "escape":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
	}

	// Pane-specific keybindings
	switch m.activePane {
	case fileListPane:
		return m.handleFileListKey(key)
	case diffViewPane:
		return m.handleDiffViewKey(key)
	}

	return m, nil
}

// togglePane switches between the file list and diff viewer panes.
func togglePane(current pane) pane {
	if current == fileListPane {
		return diffViewPane
	}
	return fileListPane
}

// handleFileListKey processes keyboard input when the file list pane is focused.
func (m model) handleFileListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if m.fileIdx < len(m.files)-1 {
			m.fileIdx++
			return m, loadFileDiff(m.repoPath, m.files[m.fileIdx])
		}
	case "k", "up":
		if m.fileIdx > 0 {
			m.fileIdx--
			return m, loadFileDiff(m.repoPath, m.files[m.fileIdx])
		}
	case "enter":
		m.activePane = diffViewPane
		return m, nil
	case "g":
		if m.fileIdx != 0 {
			m.fileIdx = 0
			return m, loadFileDiff(m.repoPath, m.files[m.fileIdx])
		}
	case "G":
		last := len(m.files) - 1
		if last >= 0 && m.fileIdx != last {
			m.fileIdx = last
			return m, loadFileDiff(m.repoPath, m.files[m.fileIdx])
		}
	}
	return m, nil
}

// handleDiffViewKey processes keyboard input when the diff viewer pane is focused.
func (m model) handleDiffViewKey(key string) (tea.Model, tea.Cmd) {
	totalLines := len(m.diffLines)
	viewHeight := m.diffViewHeight()

	switch key {
	case "j", "down":
		if m.diffCursorY < totalLines-1 {
			m.diffCursorY++
			m.ensureCursorVisible(viewHeight)
		}
	case "k", "up":
		if m.diffCursorY > 0 {
			m.diffCursorY--
			m.ensureCursorVisible(viewHeight)
		}
	case "pgdown", "ctrl+d":
		m.diffCursorY += viewHeight
		if m.diffCursorY >= totalLines {
			m.diffCursorY = totalLines - 1
		}
		if m.diffCursorY < 0 {
			m.diffCursorY = 0
		}
		m.ensureCursorVisible(viewHeight)
	case "pgup", "ctrl+u":
		m.diffCursorY -= viewHeight
		if m.diffCursorY < 0 {
			m.diffCursorY = 0
		}
		m.ensureCursorVisible(viewHeight)
	case "g":
		m.diffCursorY = 0
		m.diffScrollY = 0
	case "G":
		if totalLines > 0 {
			m.diffCursorY = totalLines - 1
			m.ensureCursorVisible(viewHeight)
		}
	case "n":
		m.jumpToNextHunk(1)
		m.ensureCursorVisible(viewHeight)
	case "N":
		m.jumpToNextHunk(-1)
		m.ensureCursorVisible(viewHeight)
	case "c":
		// Comment on current line (single line)
		if m.diffCursorY < len(m.diffLines) && !m.diffLines[m.diffCursorY].isHunkHeader {
			m.visualStart = m.diffCursorY
			m.mode = modeComment
			m.commentInput = ""
		}
	case "v":
		// Enter visual selection mode
		if m.diffCursorY < len(m.diffLines) && !m.diffLines[m.diffCursorY].isHunkHeader {
			m.mode = modeVisual
			m.visualStart = m.diffCursorY
		}
	case "e":
		// Edit comment at current line
		if m.diffCursorY < len(m.diffLines) && m.fileIdx < len(m.files) {
			dl := m.diffLines[m.diffCursorY]
			if !dl.isHunkHeader {
				lineID := comments.LineID(dl.hunkIdx, dl.lineIdx)
				if c := m.commentStore.CommentAtLineID(m.files[m.fileIdx].Path, lineID); c != nil {
					m.mode = modeEdit
					m.commentInput = c.Text
					m.editCommentID = c.ID
					m.visualStart = m.diffCursorY
				}
			}
		}
	case "d":
		// Delete comment at current line
		if m.diffCursorY < len(m.diffLines) && m.fileIdx < len(m.files) {
			dl := m.diffLines[m.diffCursorY]
			if !dl.isHunkHeader {
				lineID := comments.LineID(dl.hunkIdx, dl.lineIdx)
				if c := m.commentStore.CommentAtLineID(m.files[m.fileIdx].Path, lineID); c != nil {
					m.commentStore.Delete(c.ID)
					_ = m.commentStore.Save(m.repoPath)
				}
			}
		}
	}
	return m, nil
}

// handleVisualKey processes keyboard input during visual (range) selection mode.
func (m model) handleVisualKey(key string) (tea.Model, tea.Cmd) {
	viewHeight := m.diffViewHeight()

	switch key {
	case "j", "down":
		if m.diffCursorY < len(m.diffLines)-1 {
			m.diffCursorY++
			// Skip hunk headers in visual mode
			for m.diffCursorY < len(m.diffLines) && m.diffLines[m.diffCursorY].isHunkHeader {
				m.diffCursorY++
			}
			m.ensureCursorVisible(viewHeight)
		}
	case "k", "up":
		if m.diffCursorY > 0 {
			m.diffCursorY--
			for m.diffCursorY > 0 && m.diffLines[m.diffCursorY].isHunkHeader {
				m.diffCursorY--
			}
			m.ensureCursorVisible(viewHeight)
		}
	case "c":
		// Confirm selection and open comment input
		m.mode = modeComment
		m.commentInput = ""
	case "escape":
		m.mode = modeNormal
	}

	return m, nil
}

// handleCommentInput processes keyboard input while typing a comment.
func (m model) handleCommentInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+s":
		// Save comment
		if strings.TrimSpace(m.commentInput) != "" {
			m.saveComment()
		}
		m.mode = modeNormal
		m.commentInput = ""
	case "escape":
		m.mode = modeNormal
		m.commentInput = ""
	case "enter":
		m.commentInput += "\n"
	case "backspace":
		if len(m.commentInput) > 0 {
			m.commentInput = m.commentInput[:len(m.commentInput)-1]
		}
	case "ctrl+c":
		return m, tea.Quit
	default:
		// Only add printable characters
		if len(key) == 1 || (len(msg.Runes) > 0) {
			m.commentInput += string(msg.Runes)
		}
	}

	return m, nil
}

// saveComment creates or updates a comment from the current selection and input.
func (m *model) saveComment() {
	if m.fileIdx >= len(m.files) {
		return
	}
	filePath := m.files[m.fileIdx].Path

	// Determine selection range
	start, end := m.visualStart, m.diffCursorY
	if start > end {
		start, end = end, start
	}

	// Collect line IDs, line numbers, and code from selection
	var lineIDs []string
	var lineNums []int
	var codeLines []string

	for i := start; i <= end; i++ {
		if i >= len(m.diffLines) || m.diffLines[i].isHunkHeader {
			continue
		}
		dl := m.diffLines[i]
		lineIDs = append(lineIDs, comments.LineID(dl.hunkIdx, dl.lineIdx))
		if dl.newNum > 0 {
			lineNums = append(lineNums, dl.newNum)
		} else if dl.oldNum > 0 {
			lineNums = append(lineNums, dl.oldNum)
		}
		prefix := " "
		switch dl.lineType {
		case diff.LineAdd:
			prefix = "+"
		case diff.LineDelete:
			prefix = "-"
		}
		codeLines = append(codeLines, prefix+dl.text)
	}

	if len(lineIDs) == 0 {
		return
	}

	if m.mode == modeEdit {
		m.commentStore.Update(m.editCommentID, m.commentInput)
	} else {
		m.commentStore.Add(
			filePath,
			lineIDs,
			comments.FormatLineNum(lineNums),
			comments.CollectCode(codeLines),
			m.commentInput,
		)
	}

	// Auto-save to disk
	_ = m.commentStore.Save(m.repoPath)
}

// ensureCursorVisible adjusts scroll so the cursor is within the visible area.
func (m *model) ensureCursorVisible(viewHeight int) {
	if viewHeight <= 0 {
		return
	}
	if m.diffCursorY < m.diffScrollY {
		m.diffScrollY = m.diffCursorY
	}
	if m.diffCursorY >= m.diffScrollY+viewHeight {
		m.diffScrollY = m.diffCursorY - viewHeight + 1
	}
}

// jumpToNextHunk moves the cursor to the next (dir=1) or previous (dir=-1) hunk header.
func (m *model) jumpToNextHunk(dir int) {
	if len(m.diffLines) == 0 {
		return
	}
	i := m.diffCursorY + dir
	for i >= 0 && i < len(m.diffLines) {
		if m.diffLines[i].isHunkHeader {
			m.diffCursorY = i
			return
		}
		i += dir
	}
}

// diffViewHeight returns the number of visible lines in the diff viewer.
func (m model) diffViewHeight() int {
	// header (1) + status bar (1) + borders (2)
	h := m.height - 4
	if h < 1 {
		return 1
	}
	return h
}

// flattenDiffLines converts parsed diff files into a flat list of renderable lines.
func flattenDiffLines(files []diff.File) []diffLine {
	var lines []diffLine
	for _, f := range files {
		for hi, h := range f.Hunks {
			lines = append(lines, diffLine{
				isHunkHeader: true,
				hunkIdx:      hi,
				text:         h.Header,
			})
			for li, l := range h.Lines {
				lines = append(lines, diffLine{
					hunkIdx:  hi,
					lineIdx:  li,
					text:     l.Text,
					oldNum:   l.OldNum,
					newNum:   l.NewNum,
					lineType: l.Type,
				})
			}
		}
	}
	return lines
}

// copyPromptToClipboard copies the formatted prompt to the system clipboard
// and shows a flash message.
func (m *model) copyPromptToClipboard() tea.Cmd {
	var text string
	if m.compactMode {
		text = comments.ExportCompact(m.commentStore.All())
	} else {
		text = comments.ExportStandard(m.commentStore.All())
	}

	if err := clipboard.WriteAll(text); err != nil {
		m.flashMsg = "Error: " + err.Error()
	} else {
		m.flashMsg = "Copied!"
	}
	m.flashExpiry = time.Now().Add(2 * time.Second)

	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return clearFlashMsg{}
	})
}

// truncateToWidth truncates a string (possibly containing ANSI sequences) to a
// given visible width. This is a simple approach that falls back to raw truncation
// if the string doesn't contain escape sequences.
func truncateToWidth(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	// For strings with ANSI codes, truncate rune by rune checking width
	result := []rune{}
	w := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			result = append(result, r)
			continue
		}
		if inEscape {
			result = append(result, r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		w++
		if w > maxWidth {
			break
		}
		result = append(result, r)
	}
	return string(result)
}

// --- Styles ---

var (
	// Colors matching GitHub Dark theme
	colorBg        = lipgloss.Color("#0d1117")
	colorHeaderBg  = lipgloss.Color("#161b22")
	colorBorder    = lipgloss.Color("#30363d")
	colorText      = lipgloss.Color("#e6edf3")
	colorMuted     = lipgloss.Color("#8b949e")
	colorAccent    = lipgloss.Color("#58a6ff")
	colorGreen     = lipgloss.Color("#3fb950")
	colorRed       = lipgloss.Color("#f85149")
	colorAddBg     = lipgloss.Color("#12261e")
	colorDeleteBg  = lipgloss.Color("#2d1316")
	colorCursorBg  = lipgloss.Color("#30415e")
	colorSelectBg  = lipgloss.Color("#1a3a5c")
	colorHunkBg    = lipgloss.Color("#1c2333")
	colorPurple    = lipgloss.Color("#8957e5")
)

// View renders the entire TUI to a string for display.
func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.showHelp {
		return m.renderHelp()
	}

	header := m.renderHeader()
	status := m.renderStatus()

	if m.showPrompt {
		// Split: top 2/3 for body, bottom 1/3 for prompt panel
		promptHeight := m.height / 3
		if promptHeight < 5 {
			promptHeight = 5
		}
		bodyHeight := m.height - 2 - promptHeight // minus header and status
		body := m.renderBodyWithHeight(bodyHeight)
		prompt := m.renderPromptPanel(promptHeight)
		return header + "\n" + body + "\n" + prompt + "\n" + status
	}

	body := m.renderBody()
	return header + "\n" + body + "\n" + status
}

// renderHeader renders the top bar with app name, repo, and branch.
func (m model) renderHeader() string {
	branchStr := ""
	if m.branch != "" {
		branchStr = fmt.Sprintf("  ·  %s", m.branch)
	}

	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorText).
		Background(colorHeaderBg).
		Padding(0, 1).
		Width(m.width)

	return style.Render(fmt.Sprintf("vdiff  ·  %s%s", m.repoName, branchStr))
}

// renderBody renders the main content area with file list and diff viewer side by side.
func (m model) renderBody() string {
	bodyHeight := m.height - 2 // minus header and status bar
	return m.renderBodyWithHeight(bodyHeight)
}

// renderBodyWithHeight renders the main content area at the specified height.
func (m model) renderBodyWithHeight(bodyHeight int) string {
	if bodyHeight < 1 {
		return ""
	}

	fileListWidth := m.fileListWidth()
	diffWidth := m.width - fileListWidth - 1 // -1 for the border

	fileList := m.renderFileList(fileListWidth, bodyHeight)
	diffView := m.renderDiffView(diffWidth, bodyHeight)

	// Vertical separator
	separator := lipgloss.NewStyle().
		Foreground(colorBorder).
		Height(bodyHeight).
		Render(strings.Repeat("│\n", bodyHeight-1) + "│")

	return lipgloss.JoinHorizontal(lipgloss.Top, fileList, separator, diffView)
}

// renderPromptPanel renders the bottom panel showing the formatted prompt output.
func (m model) renderPromptPanel(height int) string {
	borderStyle := lipgloss.NewStyle().
		Foreground(colorBorder).
		Width(m.width)

	separator := borderStyle.Render(strings.Repeat("─", m.width))

	// Title bar
	titleParts := []string{
		lipgloss.NewStyle().Bold(true).Foreground(colorText).Render("Prompt Output"),
	}
	if m.compactMode {
		titleParts = append(titleParts,
			lipgloss.NewStyle().Foreground(colorMuted).Render(" [compact]"))
	}
	titleParts = append(titleParts,
		lipgloss.NewStyle().Foreground(colorMuted).Render("  ·  y: copy  ·  f: toggle format  ·  p: close"))

	title := lipgloss.NewStyle().
		Background(colorHeaderBg).
		Width(m.width).
		Padding(0, 1).
		Render(strings.Join(titleParts, ""))

	// Content
	var content string
	if m.commentStore.Count() == 0 {
		content = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1).
			Render("No comments yet. Press c on a diff line to add one.")
	} else {
		var exported string
		if m.compactMode {
			exported = comments.ExportCompact(m.commentStore.All())
		} else {
			exported = comments.ExportStandard(m.commentStore.All())
		}
		content = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(0, 1).
			Render(exported)
	}

	// Truncate content lines to fit panel height
	contentLines := strings.Split(content, "\n")
	availableHeight := height - 2 // minus separator and title
	if len(contentLines) > availableHeight {
		contentLines = contentLines[:availableHeight]
	}
	// Pad to fill height
	for len(contentLines) < availableHeight {
		contentLines = append(contentLines, "")
	}

	return separator + "\n" + title + "\n" + strings.Join(contentLines, "\n")
}

// fileListWidth returns the width allocated to the file list pane.
func (m model) fileListWidth() int {
	w := m.width / 4
	if w < 20 {
		w = 20
	}
	if w > 40 {
		w = 40
	}
	return w
}

// renderFileList renders the left pane showing changed files.
func (m model) renderFileList(width, height int) string {
	isActive := m.activePane == fileListPane

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorText).
		Padding(0, 1)
	if isActive {
		titleStyle = titleStyle.Foreground(colorAccent)
	}

	title := titleStyle.Render("Changed Files")
	lines := []string{title}

	if m.filesErr != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(colorRed).
			Padding(0, 1)
		lines = append(lines, errStyle.Render("Error: "+m.filesErr))
	} else if len(m.files) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)
		lines = append(lines, emptyStyle.Render("No changes"))
	} else {
		for i, f := range m.files {
			line := m.renderFileEntry(f, i == m.fileIdx, isActive, width)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")

	// Pad to fill height
	rendered := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Render(content)

	return rendered
}

// renderFileEntry renders a single file entry in the file list.
func (m model) renderFileEntry(f gitpkg.ChangedFile, selected, paneActive bool, width int) string {
	// Status indicator color
	var statusColor lipgloss.Color
	switch f.Status {
	case "A":
		statusColor = colorGreen
	case "D":
		statusColor = colorRed
	case "M":
		statusColor = colorAccent
	case "R":
		statusColor = lipgloss.Color("#d29922")
	default:
		statusColor = colorMuted
	}

	statusStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true)

	// Build the stats string
	stats := ""
	if f.Additions > 0 || f.Deletions > 0 {
		parts := []string{}
		if f.Additions > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render(fmt.Sprintf("+%d", f.Additions)))
		}
		if f.Deletions > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(colorRed).Render(fmt.Sprintf("-%d", f.Deletions)))
		}
		stats = " " + strings.Join(parts, " ")
	}

	// File name — truncate if needed
	name := filepath.Base(f.Path)
	dir := filepath.Dir(f.Path)
	display := name
	if dir != "." {
		display = dir + "/" + name
	}

	// Calculate available width for file path
	statusWidth := 4 // "M " prefix with padding
	statsWidth := lipgloss.Width(stats)
	available := width - statusWidth - statsWidth - 2 // padding
	if available > 0 && len(display) > available {
		display = "…" + display[len(display)-available+1:]
	}

	// Comment count badge
	commentBadge := ""
	commentCount := m.commentStore.CountForFile(f.Path)
	if commentCount > 0 {
		commentBadge = lipgloss.NewStyle().Foreground(colorPurple).Render(
			fmt.Sprintf(" [%d]", commentCount),
		)
	}

	entry := fmt.Sprintf(" %s %s%s%s", statusStyle.Render(f.Status), display, stats, commentBadge)

	style := lipgloss.NewStyle().Width(width)
	if selected && paneActive {
		style = style.Background(colorCursorBg).Foreground(colorText)
	} else if selected {
		style = style.Foreground(colorText)
	} else {
		style = style.Foreground(colorMuted)
	}

	return style.Render(entry)
}

// renderDiffView renders the right pane showing the diff for the selected file.
func (m model) renderDiffView(width, height int) string {
	isActive := m.activePane == diffViewPane

	if len(m.files) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(width).
			Height(height).
			Padding(1, 2)
		return emptyStyle.Render("No files to display")
	}

	if m.diffErr != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(colorRed).
			Width(width).
			Height(height).
			Padding(1, 2)
		return errStyle.Render("Error loading diff: " + m.diffErr)
	}

	if len(m.diffLines) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(width).
			Height(height).
			Padding(1, 2)
		return emptyStyle.Render("No diff available")
	}

	// Get current file path for comment lookups
	var currentFilePath string
	if m.fileIdx < len(m.files) {
		currentFilePath = m.files[m.fileIdx].Path
	}

	// Determine visual selection range
	selStart, selEnd := -1, -1
	if m.mode == modeVisual || m.mode == modeComment {
		selStart, selEnd = m.visualStart, m.diffCursorY
		if selStart > selEnd {
			selStart, selEnd = selEnd, selStart
		}
	}

	// Render visible lines with inline comments
	var lines []string
	for i := m.diffScrollY; i < m.diffScrollY+height && i < len(m.diffLines); i++ {
		dl := m.diffLines[i]
		isCursor := isActive && i == m.diffCursorY
		isSelected := i >= selStart && i <= selEnd
		isCommented := false
		if !dl.isHunkHeader && currentFilePath != "" {
			lineID := comments.LineID(dl.hunkIdx, dl.lineIdx)
			isCommented = m.commentStore.HasLineID(currentFilePath, lineID)
		}

		lines = append(lines, m.renderDiffLine(dl, width, isCursor, isSelected, isCommented))

		// Render inline comment below the last line of a comment's range
		if !dl.isHunkHeader && currentFilePath != "" {
			lineID := comments.LineID(dl.hunkIdx, dl.lineIdx)
			if c := m.commentStore.CommentAtLineID(currentFilePath, lineID); c != nil {
				commentLine := m.renderInlineComment(c, width)
				lines = append(lines, commentLine)
			}
		}

		// Render comment input below the selection end
		if (m.mode == modeComment || m.mode == modeEdit) && i == selEnd {
			inputLines := m.renderCommentInput(width)
			lines = append(lines, inputLines...)
		}
	}

	// Pad remaining lines
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(""))
	}

	// Truncate to fit height
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// renderDiffLine renders a single line in the diff viewer.
func (m model) renderDiffLine(dl diffLine, width int, isCursor, isSelected, isCommented bool) string {
	if dl.isHunkHeader {
		style := lipgloss.NewStyle().
			Foreground(colorAccent).
			Background(colorHunkBg).
			Bold(true).
			Width(width)
		if isCursor {
			style = style.Background(colorCursorBg)
		}
		return style.Render(" " + dl.text)
	}

	// Comment gutter marker
	gutterMarker := " "
	if isCommented {
		gutterMarker = lipgloss.NewStyle().Foreground(colorPurple).Render("┃")
	}

	// Line numbers
	gutterWidth := 5
	oldNum := "     "
	newNum := "     "
	if dl.oldNum > 0 {
		oldNum = fmt.Sprintf("%4d ", dl.oldNum)
	}
	if dl.newNum > 0 {
		newNum = fmt.Sprintf("%4d ", dl.newNum)
	}

	gutterStyle := lipgloss.NewStyle().Foreground(colorMuted)
	gutter := gutterMarker + gutterStyle.Render(oldNum) + gutterStyle.Render(newNum)

	// Prefix and content
	var prefix string
	var lineStyle lipgloss.Style

	switch dl.lineType {
	case diff.LineAdd:
		prefix = "+"
		lineStyle = lipgloss.NewStyle().Foreground(colorGreen)
		if isSelected {
			lineStyle = lineStyle.Background(colorSelectBg)
		} else if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		} else {
			lineStyle = lineStyle.Background(colorAddBg)
		}
	case diff.LineDelete:
		prefix = "-"
		lineStyle = lipgloss.NewStyle().Foreground(colorRed)
		if isSelected {
			lineStyle = lineStyle.Background(colorSelectBg)
		} else if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		} else {
			lineStyle = lineStyle.Background(colorDeleteBg)
		}
	default:
		prefix = " "
		lineStyle = lipgloss.NewStyle().Foreground(colorText)
		if isSelected {
			lineStyle = lineStyle.Background(colorSelectBg)
		} else if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		}
	}

	// Apply syntax highlighting to content, then truncate
	content := dl.text
	if m.highlighter != nil && dl.lineType == diff.LineContext {
		content = m.highlighter.Highlight(content)
	}

	contentWidth := width - gutterWidth*2 - 3 // 2 gutters + marker + prefix + padding
	if contentWidth > 0 && lipgloss.Width(content) > contentWidth {
		content = truncateToWidth(content, contentWidth)
	}

	line := lineStyle.Width(width - gutterWidth*2 - 1).Render(prefix + content)

	return gutter + line
}

// renderInlineComment renders a saved comment below its associated diff line.
func (m model) renderInlineComment(c *comments.Comment, width int) string {
	style := lipgloss.NewStyle().
		Foreground(colorPurple).
		Width(width).
		PaddingLeft(12) // align with code content

	text := fmt.Sprintf("💬 %s", c.Text)
	// Handle multiline comments
	text = strings.ReplaceAll(text, "\n", "\n"+strings.Repeat(" ", 12)+"   ")
	return style.Render(text)
}

// renderCommentInput renders the text input area for typing a comment.
func (m model) renderCommentInput(width int) []string {
	var lines []string

	borderStyle := lipgloss.NewStyle().
		Foreground(colorPurple).
		Width(width).
		PaddingLeft(12)

	label := "Comment (Ctrl+S to save, Esc to cancel):"
	if m.mode == modeEdit {
		label = "Edit comment (Ctrl+S to save, Esc to cancel):"
	}
	lines = append(lines, borderStyle.Render(label))

	inputStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Background(lipgloss.Color("#1c1c2e")).
		Width(width - 14).
		PaddingLeft(1)

	// Show input text with cursor
	displayText := m.commentInput + "█"
	for _, inputLine := range strings.Split(displayText, "\n") {
		lines = append(lines, lipgloss.NewStyle().PaddingLeft(12).Render(
			inputStyle.Render(inputLine),
		))
	}

	return lines
}

// renderHelp renders a full-screen keybinding help overlay.
func (m model) renderHelp() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		Render("Keybindings")

	subtitle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render("Press ? or Esc to close")

	keyStyle := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Width(16)

	descStyle := lipgloss.NewStyle().
		Foreground(colorText)

	bindings := []struct{ key, desc string }{
		{"j / k / ↑ / ↓", "Navigate up/down"},
		{"← / → / Tab", "Switch pane"},
		{"Enter", "Open file diff"},
		{"g / G", "Top / Bottom"},
		{"PgUp / PgDn", "Page up / down"},
		{"n / N", "Next / Previous hunk"},
		{"", ""},
		{"c", "Comment on current line"},
		{"v", "Visual select (range)"},
		{"e", "Edit comment at cursor"},
		{"d", "Delete comment at cursor"},
		{"Ctrl+S", "Save comment"},
		{"Escape", "Cancel / Close"},
		{"", ""},
		{"p", "Toggle prompt panel"},
		{"y", "Copy prompt to clipboard"},
		{"f", "Toggle compact format"},
		{"", ""},
		{"r", "Refresh file list"},
		{"q", "Quit"},
		{"?", "Toggle this help"},
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+title)
	lines = append(lines, "  "+subtitle)
	lines = append(lines, "")

	for _, b := range bindings {
		if b.key == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, "  "+keyStyle.Render(b.key)+descStyle.Render(b.desc))
	}

	content := strings.Join(lines, "\n")

	// Center vertically
	contentHeight := len(lines)
	topPad := (m.height - contentHeight) / 2
	if topPad < 0 {
		topPad = 0
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		PaddingTop(topPad).
		Render(content)
}

// renderStatus renders the bottom status bar with mode indicator and key hints.
func (m model) renderStatus() string {
	var modeStr, hints string

	switch m.mode {
	case modeVisual:
		modeStr = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("VISUAL")
		hints = "j/k: extend selection  ·  c: comment  ·  esc: cancel"
	case modeComment:
		modeStr = lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render("COMMENT")
		hints = "type comment  ·  ctrl+s: save  ·  esc: cancel"
	case modeEdit:
		modeStr = lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render("EDIT")
		hints = "edit comment  ·  ctrl+s: save  ·  esc: cancel"
	default:
		modeStr = lipgloss.NewStyle().Foreground(colorMuted).Render("NORMAL")
		switch m.activePane {
		case fileListPane:
			hints = "j/k: navigate  ·  enter: view diff  ·  tab: switch pane  ·  r: refresh  ·  q: quit"
		case diffViewPane:
			hints = "j/k: scroll  ·  c: comment  ·  v: select range  ·  e/d: edit/delete  ·  n/N: hunks  ·  q: quit"
		}
	}

	// Comment count
	countStr := ""
	if m.commentStore.Count() > 0 {
		countStr = lipgloss.NewStyle().Foreground(colorPurple).Render(
			fmt.Sprintf("  [%d comments]", m.commentStore.Count()),
		)
	}

	// Flash message
	flashStr := ""
	if m.flashMsg != "" {
		flashStr = "  " + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(m.flashMsg)
	}

	style := lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorHeaderBg).
		Padding(0, 1).
		Width(m.width)

	return style.Render(modeStr + "  " + hints + countStr + flashStr)
}
