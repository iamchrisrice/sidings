package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Route, execute, and coordinate coding tasks",
		Long: `Route, execute, and coordinate coding tasks through the sidings pipeline.

Single task:
  echo "fix the typo in README.md" | sidings task classify | sidings task route | sidings task dispatch

Parallel pipeline:
  echo "add authentication to the bookmarks API" \
    | sidings task decompose \
    | xargs -P 4 -I {} sh -c 'echo "{}" | sidings task classify | sidings task route | sidings task dispatch 2>/dev/null' \
    | sidings task merge`,
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
			"Break a large task into independent subtasks",
			`Decomposes a large task into 3–6 independent subtasks, each emitted as its own NDJSON line.

Gathers project context first (tracked files, go.mod, recent .go files) so subtasks
reference real filenames and packages from the current directory.

Each output line carries parent_task_id and parent_content so downstream tools have
full context when the subtasks are dispatched in parallel.`),
		delegate("merge", "task-merge",
			"Merge parallel subtask results into a single summary",
			`Reads subtask result lines until EOF and emits one summary NDJSON line per parent task.

Summary fields:
  status           complete (all succeeded) | partial (some failed) | failed (all failed)
  subtasks_total   count of subtask lines received
  subtasks_complete / subtasks_failed
  files_written    deduplicated union across all subtasks
  duration_ms      wall-clock elapsed from first to last result (not a sum)

Lines without a parent_task_id pass through unchanged, so merge is safe to add to
any pipeline regardless of whether decompose was used.`),
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
