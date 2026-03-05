package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// model is the top-level Bubble Tea model holding all application state.
type model struct {
	repoPath string
	repoName string
	width    int
	height   int
}

// newModel creates the initial model for the given repository path.
func newModel(repoPath string) model {
	return model{
		repoPath: repoPath,
		repoName: filepath.Base(repoPath),
	}
}

// Init returns the initial command to run when the program starts.
func (m model) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages (key presses, window resizes) and returns
// the updated model and any commands to execute.
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

// View renders the entire TUI to a string for display.
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
