package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erikbelusic/vdiff-tui/diff"
	gitpkg "github.com/erikbelusic/vdiff-tui/git"
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
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running vdiff: %s\n", err)
		os.Exit(1)
	}
}

// parseArgs processes CLI arguments and returns the resolved repo path.
// Handles --help, --version flags and an optional positional path argument.
func parseArgs() (string, error) {
	args := os.Args[1:]

	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		case "--version", "-v":
			fmt.Printf("vdiff %s\n", version)
			os.Exit(0)
		}
	}

	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return filepath.Abs(args[0])
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
  -v, --version    Show version`)
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

	// Pane focus
	activePane pane
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
		repoPath:   repoPath,
		repoName:   filepath.Base(repoPath),
		activePane: fileListPane,
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

// --- Commands ---

// loadGitData fetches the current branch and changed files list.
func loadGitData(repoPath string) tea.Cmd {
	return func() tea.Msg {
		branch, _ := gitpkg.CurrentBranch(repoPath)
		files, err := gitpkg.ChangedFiles(repoPath)
		return gitDataMsg{branch: branch, files: files, err: err}
	}
}

// loadFileDiff fetches the diff for a specific file.
func loadFileDiff(repoPath string, file gitpkg.ChangedFile) tea.Cmd {
	return func() tea.Msg {
		raw, err := gitpkg.FileDiff(repoPath, file)
		return fileDiffMsg{raw: raw, err: err}
	}
}

// Init returns the initial command to load git data on startup.
func (m model) Init() tea.Cmd {
	return loadGitData(m.repoPath)
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
			// Auto-select first file if available
			if len(m.files) > 0 && m.fileIdx == 0 {
				return m, loadFileDiff(m.repoPath, m.files[0])
			}
		}
		return m, nil

	case fileDiffMsg:
		if msg.err != nil {
			m.diffErr = msg.err.Error()
			m.diffFiles = nil
			m.diffLines = nil
		} else {
			m.diffRaw = msg.raw
			m.diffFiles = diff.Parse(msg.raw)
			m.diffLines = flattenDiffLines(m.diffFiles)
			m.diffScrollY = 0
			m.diffCursorY = 0
			m.diffErr = ""
		}
		return m, nil
	}

	return m, nil
}

// handleKey processes keyboard input based on the active pane and mode.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keybindings
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.activePane = togglePane(m.activePane)
		return m, nil
	case "r":
		return m, loadGitData(m.repoPath)
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
	}
	return m, nil
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
	colorCursorBg  = lipgloss.Color("#1f2937")
	colorHunkBg    = lipgloss.Color("#1c2333")
)

// View renders the entire TUI to a string for display.
func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.renderHeader()
	body := m.renderBody()
	status := m.renderStatus()

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

	entry := fmt.Sprintf(" %s %s%s", statusStyle.Render(f.Status), display, stats)

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

	// Render visible lines
	var lines []string
	for i := m.diffScrollY; i < m.diffScrollY+height && i < len(m.diffLines); i++ {
		dl := m.diffLines[i]
		isCursor := isActive && i == m.diffCursorY
		lines = append(lines, m.renderDiffLine(dl, width, isCursor))
	}

	// Pad remaining lines
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(""))
	}

	return strings.Join(lines, "\n")
}

// renderDiffLine renders a single line in the diff viewer.
func (m model) renderDiffLine(dl diffLine, width int, isCursor bool) string {
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
	gutter := gutterStyle.Render(oldNum) + gutterStyle.Render(newNum)

	// Prefix and content
	var prefix string
	var lineStyle lipgloss.Style

	switch dl.lineType {
	case diff.LineAdd:
		prefix = "+"
		lineStyle = lipgloss.NewStyle().Foreground(colorGreen)
		if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		} else {
			lineStyle = lineStyle.Background(colorAddBg)
		}
	case diff.LineDelete:
		prefix = "-"
		lineStyle = lipgloss.NewStyle().Foreground(colorRed)
		if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		} else {
			lineStyle = lineStyle.Background(colorDeleteBg)
		}
	default:
		prefix = " "
		lineStyle = lipgloss.NewStyle().Foreground(colorText)
		if isCursor {
			lineStyle = lineStyle.Background(colorCursorBg)
		}
	}

	// Truncate content to fit width
	contentWidth := width - gutterWidth*2 - 2 // 2 gutters + prefix + padding
	content := dl.text
	if contentWidth > 0 && len(content) > contentWidth {
		content = content[:contentWidth]
	}

	line := lineStyle.Width(width - gutterWidth*2).Render(prefix + content)

	return gutter + line
}

// renderStatus renders the bottom status bar with key hints.
func (m model) renderStatus() string {
	var hints string
	switch m.activePane {
	case fileListPane:
		hints = "j/k: navigate  ·  enter: view diff  ·  tab: switch pane  ·  r: refresh  ·  q: quit"
	case diffViewPane:
		hints = "j/k: scroll  ·  n/N: next/prev hunk  ·  g/G: top/bottom  ·  tab: switch pane  ·  q: quit"
	}

	style := lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorHeaderBg).
		Padding(0, 1).
		Width(m.width)

	return style.Render(hints)
}
