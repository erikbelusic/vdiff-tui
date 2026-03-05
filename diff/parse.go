package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// File represents a parsed diff for a single file.
type File struct {
	Path      string
	OldPath   string
	Hunks     []Hunk
	Binary    bool
	Additions int
	Deletions int
}

// Hunk represents a single diff hunk.
type Hunk struct {
	Header   string // the @@ line
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Line
}

// LineType indicates whether a line is context, addition, or deletion.
type LineType int

const (
	LineContext  LineType = iota
	LineAdd
	LineDelete
)

// Line represents a single line in a diff hunk.
type Line struct {
	Type   LineType
	OldNum int    // 0 for additions
	NewNum int    // 0 for deletions
	Text   string // line content without the +/-/space prefix
}

// Parse takes unified diff output and returns parsed diff files.
func Parse(raw string) []File {
	if raw == "" {
		return nil
	}

	var files []File
	chunks := splitFiles(raw)

	for _, chunk := range chunks {
		f := parseFile(chunk)
		if f != nil {
			files = append(files, *f)
		}
	}

	return files
}

func splitFiles(raw string) []string {
	lines := strings.Split(raw, "\n")
	var chunks []string
	var current []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "diff --no-index") {
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, "\n"))
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}

	return chunks
}

func parseFile(chunk string) *File {
	lines := strings.Split(chunk, "\n")
	f := &File{}

	i := 0
	for i < len(lines) {
		line := lines[i]

		if strings.HasPrefix(line, "diff --git") {
			// Extract path from "diff --git a/path b/path"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				f.Path = strings.TrimPrefix(parts[len(parts)-1], "b/")
			}
		} else if strings.HasPrefix(line, "diff --no-index") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				f.Path = strings.TrimPrefix(parts[len(parts)-1], "b/")
			}
		} else if strings.HasPrefix(line, "--- a/") {
			f.OldPath = strings.TrimPrefix(line, "--- a/")
		} else if strings.HasPrefix(line, "--- /dev/null") {
			f.OldPath = "/dev/null"
		} else if strings.HasPrefix(line, "+++ b/") {
			f.Path = strings.TrimPrefix(line, "+++ b/")
		} else if strings.HasPrefix(line, "Binary files") {
			f.Binary = true
		} else if strings.HasPrefix(line, "rename from ") {
			f.OldPath = strings.TrimPrefix(line, "rename from ")
		} else if strings.HasPrefix(line, "@@") {
			hunk, consumed := parseHunk(lines[i:])
			if hunk != nil {
				f.Hunks = append(f.Hunks, *hunk)
				f.Additions += hunk.countType(LineAdd)
				f.Deletions += hunk.countType(LineDelete)
			}
			i += consumed
			continue
		}

		i++
	}

	if f.Path == "" {
		return nil
	}

	return f
}

func parseHunk(lines []string) (*Hunk, int) {
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "@@") {
		return nil, 1
	}

	h := &Hunk{Header: lines[0]}
	h.OldStart, h.OldCount, h.NewStart, h.NewCount = parseHunkHeader(lines[0])

	oldNum := h.OldStart
	newNum := h.NewStart
	consumed := 1

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		consumed++

		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff ") {
			// Start of next hunk or file — back up
			consumed--
			break
		}

		if len(line) == 0 {
			// Could be an empty context line or end of input.
			// Check if there are more non-empty lines in this hunk.
			allEmpty := true
			for j := i + 1; j < len(lines); j++ {
				if lines[j] != "" {
					if strings.HasPrefix(lines[j], "@@") || strings.HasPrefix(lines[j], "diff ") {
						break
					}
					allEmpty = false
					break
				}
			}
			if allEmpty {
				// Trailing blank lines — stop
				break
			}
			// Genuine empty context line
			h.Lines = append(h.Lines, Line{
				Type:   LineContext,
				OldNum: oldNum,
				NewNum: newNum,
				Text:   "",
			})
			oldNum++
			newNum++
			continue
		}

		switch line[0] {
		case '+':
			h.Lines = append(h.Lines, Line{
				Type:   LineAdd,
				NewNum: newNum,
				Text:   line[1:],
			})
			newNum++
		case '-':
			h.Lines = append(h.Lines, Line{
				Type:   LineDelete,
				OldNum: oldNum,
				Text:   line[1:],
			})
			oldNum++
		case '\\':
			// "\ No newline at end of file" — skip
		default:
			// Context line (starts with space)
			text := line
			if len(text) > 0 && text[0] == ' ' {
				text = text[1:]
			}
			h.Lines = append(h.Lines, Line{
				Type:   LineContext,
				OldNum: oldNum,
				NewNum: newNum,
				Text:   text,
			})
			oldNum++
			newNum++
		}
	}

	return h, consumed
}

func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int) {
	// Format: @@ -oldStart,oldCount +newStart,newCount @@
	at := strings.Index(header, "@@")
	if at < 0 {
		return
	}
	rest := header[at+2:]
	end := strings.Index(rest, "@@")
	if end < 0 {
		rest = strings.TrimSpace(rest)
	} else {
		rest = strings.TrimSpace(rest[:end])
	}

	parts := strings.Fields(rest)
	if len(parts) >= 1 {
		oldStart, oldCount = parseRange(strings.TrimPrefix(parts[0], "-"))
	}
	if len(parts) >= 2 {
		newStart, newCount = parseRange(strings.TrimPrefix(parts[1], "+"))
	}
	return
}

func parseRange(s string) (start, count int) {
	if idx := strings.Index(s, ","); idx >= 0 {
		start, _ = strconv.Atoi(s[:idx])
		count, _ = strconv.Atoi(s[idx+1:])
	} else {
		start, _ = strconv.Atoi(s)
		count = 1
	}
	return
}

func (h *Hunk) countType(t LineType) int {
	n := 0
	for _, l := range h.Lines {
		if l.Type == t {
			n++
		}
	}
	return n
}

// LineID returns a unique identifier for a line within a diff, used for comment anchoring.
func LineID(hunkIdx, lineIdx int) string {
	return fmt.Sprintf("%d-%d", hunkIdx, lineIdx)
}
