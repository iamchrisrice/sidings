package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// libexecCandidates returns the ordered list of paths to search for binary.
// Extracted for testability — libexecPath calls this with os.Args[0].
func libexecCandidates(binary, argv0 string) []string {
	return []string{
		filepath.Join(os.Getenv("SIDINGS_LIBEXEC"), binary),
		filepath.Join(os.Getenv("HOME"), ".local/libexec/sidings", binary),
		filepath.Join("/usr/local/libexec/sidings", binary),
		filepath.Join(filepath.Dir(argv0), binary),
	}
}

// libexecPath resolves the absolute path to a sidings libexec binary.
// Search order: SIDINGS_LIBEXEC env → ~/.local/libexec/sidings → /usr/local/libexec/sidings → same dir as sidings binary.
func libexecPath(binary string) (string, error) {
	for _, path := range libexecCandidates(binary, os.Args[0]) {
		if path == "" || path == binary {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("sidings component %q not found — try reinstalling sidings", binary)
}
