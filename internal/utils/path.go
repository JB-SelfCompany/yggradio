package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ in file paths to home directory
// Handles:
//   - "~" → home directory
//   - "~/path" → home/path
//   - anything else → unchanged
func ExpandPath(path string) string {
	if len(path) == 0 {
		return path
	}

	// Only expand if path is exactly "~" or starts with "~/"
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path // Return original path if we can't get home dir
		}

		if path == "~" {
			return home
		}

		// Skip "~/" and join with home
		return filepath.Join(home, path[2:])
	}

	return path
}
