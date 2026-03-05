// persistence.go handles saving and loading comments to/from disk.
package comments

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configDir returns the path to the vdiff config directory (~/.config/vdiff).
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "vdiff"), nil
}

// commentsFilePath returns the full path to the comments JSON file.
func commentsFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "comments.json"), nil
}

// Load reads comments for a specific repo from disk into the store.
func (s *Store) Load(repoPath string) error {
	filePath, err := commentsFilePath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no saved comments yet
		}
		return err
	}

	var allComments map[string][]Comment
	if err := json.Unmarshal(data, &allComments); err != nil {
		return err
	}

	if repoComments, ok := allComments[repoPath]; ok {
		s.SetAll(repoComments)
	}

	return nil
}

// Save writes the current comments for a specific repo to disk.
func (s *Store) Save(repoPath string) error {
	filePath, err := commentsFilePath()
	if err != nil {
		return err
	}

	// Read existing file to preserve comments for other repos
	var allComments map[string][]Comment

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		allComments = make(map[string][]Comment)
	} else {
		if err := json.Unmarshal(data, &allComments); err != nil {
			allComments = make(map[string][]Comment)
		}
	}

	// Update this repo's comments
	if len(s.comments) == 0 {
		delete(allComments, repoPath)
	} else {
		allComments[repoPath] = s.comments
	}

	// Ensure config directory exists
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write back
	out, err := json.MarshalIndent(allComments, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, out, 0644)
}
