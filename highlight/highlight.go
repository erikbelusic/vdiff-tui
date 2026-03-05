// Package highlight provides syntax highlighting for diff lines using Chroma.
package highlight

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// style is the Chroma style used for highlighting, based on GitHub Dark.
var style = styles.Get("github-dark")

// Highlighter caches the lexer for a file so repeated calls are fast.
type Highlighter struct {
	lexer chroma.Lexer
}

// New creates a Highlighter for the given file path, detecting the language
// from the file extension or name. Returns nil if no lexer is found.
func New(filePath string) *Highlighter {
	lexer := detectLexer(filePath)
	if lexer == nil {
		return nil
	}
	return &Highlighter{lexer: lexer}
}

// Highlight applies syntax highlighting to a single line of code,
// returning a string with lipgloss ANSI color sequences.
func (h *Highlighter) Highlight(line string) string {
	if h == nil || line == "" {
		return line
	}

	iterator, err := h.lexer.Tokenise(nil, line)
	if err != nil {
		return line
	}

	var result strings.Builder
	for _, token := range iterator.Tokens() {
		entry := style.Get(token.Type)
		text := token.Value

		if entry.Colour.IsSet() {
			color := lipgloss.Color(entry.Colour.String())
			s := lipgloss.NewStyle().Foreground(color)
			if entry.Bold == chroma.Yes {
				s = s.Bold(true)
			}
			if entry.Italic == chroma.Yes {
				s = s.Italic(true)
			}
			result.WriteString(s.Render(text))
		} else {
			result.WriteString(text)
		}
	}

	return result.String()
}

// detectLexer finds the appropriate Chroma lexer for a file path.
func detectLexer(filePath string) chroma.Lexer {
	// Try by filename first (handles Dockerfile, Makefile, etc.)
	name := filepath.Base(filePath)
	if lexer := lexers.Get(name); lexer != nil {
		return chroma.Coalesce(lexer)
	}

	// Try by extension
	ext := filepath.Ext(filePath)
	if ext != "" {
		if lexer := lexers.Get(ext); lexer != nil {
			return chroma.Coalesce(lexer)
		}
	}

	// Chroma's own analysis as fallback
	if lexer := lexers.Match(filePath); lexer != nil {
		return chroma.Coalesce(lexer)
	}

	return nil
}
