package prompt

import "strings"

// FileChange represents a file to be created or modified by the model's response.
type FileChange struct {
	Path    string
	Content string
}

// ParseFileChanges extracts <<<< path / >>>> file blocks from a model response.
// Returns an empty slice if no blocks are found. Never panics on malformed input.
func ParseFileChanges(response string) []FileChange {
	var changes []FileChange
	lines := strings.Split(response, "\n")

	var current *FileChange
	var contentLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "<<<<") {
			// Start of a new block — any open unclosed block is discarded.
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "<<<<"))
			if path != "" {
				current = &FileChange{Path: path}
				contentLines = nil
			}
			continue
		}

		if trimmed == ">>>>" {
			if current != nil {
				current.Content = strings.Join(contentLines, "\n")
				changes = append(changes, *current)
				current = nil
				contentLines = nil
			}
			continue
		}

		if current != nil {
			contentLines = append(contentLines, line)
		}
	}

	// Unclosed blocks are silently discarded.
	return changes
}
