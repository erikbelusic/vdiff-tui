// export.go formats comments into prompt text for AI coding agents.
package comments

import (
	"fmt"
	"strings"
)

// ExportStandard formats all comments in standard format with code snippets.
func ExportStandard(comments []Comment) string {
	if len(comments) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Address the following feedback:\n")

	for _, c := range comments {
		b.WriteString(fmt.Sprintf("\n- %s:%s\n", c.FilePath, c.LineNum))

		// Code snippet
		codeLines := strings.Split(c.Code, "\n")
		if len(codeLines) == 1 {
			b.WriteString(fmt.Sprintf("   Code: %s\n", codeLines[0]))
		} else {
			b.WriteString("   Code:\n")
			for _, line := range codeLines {
				b.WriteString(fmt.Sprintf("     %s\n", line))
			}
		}

		// Comment text
		commentLines := strings.Split(c.Text, "\n")
		if len(commentLines) == 1 {
			b.WriteString(fmt.Sprintf("   Comment: %s\n", commentLines[0]))
		} else {
			b.WriteString("   Comment:\n")
			for _, line := range commentLines {
				b.WriteString(fmt.Sprintf("     %s\n", line))
			}
		}
	}

	return b.String()
}

// ExportCompact formats all comments in compact format without code snippets.
func ExportCompact(comments []Comment) string {
	if len(comments) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Address the following feedback:\n")

	for _, c := range comments {
		b.WriteString(fmt.Sprintf("\n- %s:%s\n", c.FilePath, c.LineNum))
		for _, line := range strings.Split(c.Text, "\n") {
			b.WriteString(fmt.Sprintf("  - %s\n", line))
		}
	}

	return b.String()
}
