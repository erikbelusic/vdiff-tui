// Package comments manages line-level comments on diff files.
package comments

import (
	"fmt"
	"strings"
)

// Comment represents a user comment anchored to one or more diff lines.
type Comment struct {
	ID       int      `json:"id"`
	FilePath string   `json:"filePath"`
	LineIDs  []string `json:"lineIds"`  // "hunkIdx-lineIdx" identifiers
	LineNum  string   `json:"lineNum"`  // display format: "42" or "40-45"
	Code     string   `json:"code"`     // selected line(s) content
	Text     string   `json:"text"`     // the user's comment
}

// Store holds comments for the current repository.
type Store struct {
	comments  []Comment
	nextID    int
}

// NewStore creates an empty comment store.
func NewStore() *Store {
	return &Store{nextID: 1}
}

// Add creates a new comment and returns it.
func (s *Store) Add(filePath string, lineIDs []string, lineNum string, code string, text string) Comment {
	c := Comment{
		ID:       s.nextID,
		FilePath: filePath,
		LineIDs:  lineIDs,
		LineNum:  lineNum,
		Code:     code,
		Text:     text,
	}
	s.nextID++
	s.comments = append(s.comments, c)
	return c
}

// Update replaces the text of an existing comment by ID.
func (s *Store) Update(id int, text string) {
	for i := range s.comments {
		if s.comments[i].ID == id {
			s.comments[i].Text = text
			return
		}
	}
}

// Delete removes a comment by ID.
func (s *Store) Delete(id int) {
	for i := range s.comments {
		if s.comments[i].ID == id {
			s.comments = append(s.comments[:i], s.comments[i+1:]...)
			return
		}
	}
}

// ForFile returns all comments for a given file path.
func (s *Store) ForFile(filePath string) []Comment {
	var result []Comment
	for _, c := range s.comments {
		if c.FilePath == filePath {
			result = append(result, c)
		}
	}
	return result
}

// All returns all comments across all files.
func (s *Store) All() []Comment {
	return s.comments
}

// Count returns the total number of comments.
func (s *Store) Count() int {
	return len(s.comments)
}

// CountForFile returns the number of comments for a specific file.
func (s *Store) CountForFile(filePath string) int {
	n := 0
	for _, c := range s.comments {
		if c.FilePath == filePath {
			n++
		}
	}
	return n
}

// HasLineID returns true if any comment for the given file contains the specified line ID.
func (s *Store) HasLineID(filePath string, lineID string) bool {
	for _, c := range s.comments {
		if c.FilePath != filePath {
			continue
		}
		for _, lid := range c.LineIDs {
			if lid == lineID {
				return true
			}
		}
	}
	return false
}

// CommentAtLineID returns the comment anchored at a specific line ID for a file, if any.
func (s *Store) CommentAtLineID(filePath string, lineID string) *Comment {
	for i := range s.comments {
		c := &s.comments[i]
		if c.FilePath != filePath {
			continue
		}
		// Return comment if this lineID is the last in the range (comment renders below)
		if len(c.LineIDs) > 0 && c.LineIDs[len(c.LineIDs)-1] == lineID {
			return c
		}
	}
	return nil
}

// SetAll replaces all comments (used when loading from disk).
func (s *Store) SetAll(comments []Comment) {
	s.comments = comments
	s.nextID = 1
	for _, c := range comments {
		if c.ID >= s.nextID {
			s.nextID = c.ID + 1
		}
	}
}

// PruneFiles removes comments for files not in the provided list.
func (s *Store) PruneFiles(activePaths []string) {
	active := make(map[string]bool)
	for _, p := range activePaths {
		active[p] = true
	}
	var kept []Comment
	for _, c := range s.comments {
		if active[c.FilePath] {
			kept = append(kept, c)
		}
	}
	s.comments = kept
}

// LineID builds a line identifier string from hunk and line indices.
func LineID(hunkIdx, lineIdx int) string {
	return fmt.Sprintf("%d-%d", hunkIdx, lineIdx)
}

// FormatLineNum builds a display string for a line number or range.
func FormatLineNum(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	if len(nums) == 1 {
		return fmt.Sprintf("%d", nums[0])
	}
	return fmt.Sprintf("%d-%d", nums[0], nums[len(nums)-1])
}

// CollectCode gathers the code text from selected lines, joining with newlines.
func CollectCode(texts []string) string {
	return strings.Join(texts, "\n")
}
