// Package git provides functions for executing git commands and parsing their output.
package git

import (
	"os/exec"
	"strings"
)

// CurrentBranch returns the name of the checked-out branch.
func CurrentBranch(repoPath string) (string, error) {
	out, err := run(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ChangedFile represents a file with uncommitted changes.
type ChangedFile struct {
	Path      string
	Status    string // A, M, D, R
	OldPath   string // set for renames
	Additions int
	Deletions int
}

// ChangedFiles returns all uncommitted changes (staged + unstaged + untracked).
func ChangedFiles(repoPath string) ([]ChangedFile, error) {
	out, err := run(repoPath, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	var files []ChangedFile
	seen := make(map[string]bool)

	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}

		staged := line[0]
		unstaged := line[1]
		path := strings.TrimSpace(line[3:])

		// Handle renames: "R  old -> new"
		var oldPath string
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			oldPath = parts[0]
			path = parts[1]
		}

		if seen[path] {
			continue
		}
		seen[path] = true

		status := mapStatus(staged, unstaged)
		files = append(files, ChangedFile{
			Path:    path,
			Status:  status,
			OldPath: oldPath,
		})
	}

	// Get per-file stats
	if err := fillStats(repoPath, files); err != nil {
		// Non-fatal — we just won't have stats
		_ = err
	}

	return files, nil
}

// FileDiff returns the unified diff output for a single file.
// It tries unstaged changes first, then staged, handling added/deleted files specially.
func FileDiff(repoPath string, file ChangedFile) (string, error) {
	switch file.Status {
	case "A":
		// Untracked or newly staged file — diff against /dev/null
		out, _ := run(repoPath, "diff", "--cached", "--", file.Path)
		if out != "" {
			return out, nil
		}
		// Untracked file
		out, err := run(repoPath, "diff", "--no-index", "--", "/dev/null", file.Path)
		// diff --no-index returns exit 1 when files differ, which is expected
		if out != "" {
			return out, nil
		}
		return out, err

	case "D":
		out, err := run(repoPath, "diff", "--", file.Path)
		if out != "" {
			return out, nil
		}
		out, err = run(repoPath, "diff", "--cached", "--", file.Path)
		return out, err

	default:
		// Try unstaged first, then staged
		out, _ := run(repoPath, "diff", "--", file.Path)
		if out != "" {
			return out, nil
		}
		out, err := run(repoPath, "diff", "--cached", "--", file.Path)
		return out, err
	}
}

// mapStatus converts git porcelain status codes to a single-character status string.
func mapStatus(staged, unstaged byte) string {
	switch {
	case staged == 'R' || unstaged == 'R':
		return "R"
	case staged == 'A' || unstaged == '?':
		return "A"
	case staged == 'D' || unstaged == 'D':
		return "D"
	default:
		return "M"
	}
}

// fillStats populates the Additions and Deletions counts on each ChangedFile
// by running git diff --numstat for both staged and unstaged changes.
func fillStats(repoPath string, files []ChangedFile) error {
	// Staged stats
	stagedOut, _ := run(repoPath, "diff", "--cached", "--numstat")
	// Unstaged stats
	unstagedOut, _ := run(repoPath, "diff", "--numstat")

	stats := make(map[string][2]int) // path -> [additions, deletions]
	for _, out := range []string{stagedOut, unstagedOut} {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			// Binary files show "-" for stats
			if parts[0] == "-" {
				continue
			}
			add := atoi(parts[0])
			del := atoi(parts[1])
			path := parts[2]
			// For renames, numstat shows "old => new" or uses {old => new}
			if len(parts) > 3 {
				path = parts[len(parts)-1]
			}
			existing := stats[path]
			existing[0] += add
			existing[1] += del
			stats[path] = existing
		}
	}

	for i := range files {
		if s, ok := stats[files[i].Path]; ok {
			files[i].Additions = s[0]
			files[i].Deletions = s[1]
		}
	}

	return nil
}

// atoi parses a string to int, returning 0 for any non-numeric input.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// run executes a git command in the given repo directory and returns its stdout.
func run(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}
