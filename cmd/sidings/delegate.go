package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// delegate returns a cobra command that execs the named libexec binary,
// passing all args and stdio straight through.
func delegate(use, binary, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		// DisableFlagParsing passes all flags (--verbose, --dry-run, etc.)
		// through to the underlying binary unchanged.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := libexecPath(binary)
			if err != nil {
				return err
			}
			c := exec.Command(bin, args...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				return err
			}
			return nil
		},
	}
}
