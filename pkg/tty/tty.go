// Package tty provides terminal I/O helpers that work correctly inside pipelines.
package tty

import (
	"io"
	"os"
)

// Reader returns /dev/tty if available, falling back to os.Stdin.
// /dev/tty always refers to the controlling terminal regardless of how stdin
// is redirected — this is how git, ssh, and gpg handle interactive input
// inside pipelines.
func Reader() io.Reader {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		// /dev/tty unavailable (e.g. CI, non-interactive environment).
		// Fall back to os.Stdin — interactive prompts may not work
		// but the tool should still run.
		return os.Stdin
	}
	return tty
}
