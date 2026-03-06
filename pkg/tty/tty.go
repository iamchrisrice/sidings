// Package tty provides terminal I/O helpers that work correctly inside pipelines.
package tty

import (
	"fmt"
	"io"
	"os"
	"strings"
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

// Confirm prints a prompt to stderr and reads a y/n response from the
// terminal. Returns true if the user responds with "y" or "yes".
// Always reads from /dev/tty so it works correctly inside pipelines.
func Confirm(prompt string) bool {
	return ConfirmFromReader(prompt, Reader())
}

// ConfirmFromReader is the testable core of Confirm.
// It prints to stderr and reads a response from r.
func ConfirmFromReader(prompt string, r io.Reader) bool {
	fmt.Fprintf(os.Stderr, "%s (y/n) ", prompt)
	var response string
	fmt.Fscan(r, &response) //nolint:errcheck
	resp := strings.ToLower(strings.TrimSpace(response))
	return resp == "y" || resp == "yes"
}
