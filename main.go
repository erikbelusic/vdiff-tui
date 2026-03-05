package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var version = "dev"

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

// --- Bubble Tea Model ---

type model struct {
	repoPath string
	repoName string
	width    int
	height   int
}

func newModel(repoPath string) model {
	return model{
		repoPath: repoPath,
		repoName: filepath.Base(repoPath),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#e6edf3")).
		Background(lipgloss.Color("#161b22")).
		Padding(0, 1).
		Width(m.width)

	header := headerStyle.Render(
		fmt.Sprintf("vdiff  ·  %s", m.repoName),
	)

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8b949e")).
		Padding(1, 2)

	body := bodyStyle.Render("No files to display. Press q to quit.")

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8b949e")).
		Background(lipgloss.Color("#161b22")).
		Padding(0, 1).
		Width(m.width)

	status := statusStyle.Render("q: quit  ·  ?: help")

	return header + "\n" + body + "\n" + status
}
