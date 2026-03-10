package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Route and execute coding tasks",
		Long:  "Route and execute coding tasks through the sidings pipeline.",
	}

	cmd.AddCommand(
		delegate("classify", "task-classify",
			"Classify a task by complexity tier",
			"Classifies a coding task as simple, medium, complex, or exceptional using a local LLM."),
		delegate("route", "task-route",
			"Route a classified task to the right model",
			"Selects the appropriate model for a classified task."),
		delegate("dispatch", "task-dispatch",
			"Execute a task against the routed model",
			"Executes a classified and routed task via Claude Code."),
		delegate("decompose", "task-decompose",
			"Break a large task into parallel subtasks",
			"Decomposes a large task into smaller parallel subtasks."),
		delegate("merge", "task-merge",
			"Combine results from parallel task execution",
			"Merges results from parallel task execution."),
	)

	return cmd
}

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
