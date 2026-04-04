package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:          "sidings",
		Long:         "sidings — intelligent LLM task routing for the terminal",
		SilenceUsage: true,
	}

	root.AddCommand(
		delegate("classify", "classify",
			"Classify a task by complexity tier",
			"Classifies a coding task as simple, medium, complex, or exceptional using a local LLM."),
		delegate("route", "route",
			"Route a classified task to the right model",
			"Selects the appropriate model for a classified task."),
		delegate("dispatch", "dispatch",
			"Execute a task against the routed model",
			"Executes a classified and routed task via Claude Code."),
		delegate("decompose", "decompose",
			"Break a large task into independent subtasks",
			"Decomposes a large task into 3–6 independent subtasks, each emitted as its own NDJSON line."),
		delegate("merge", "merge",
			"Merge parallel subtask results into a single summary",
			"Reads subtask result lines until EOF and emits one summary NDJSON line per parent task."),
	)
	root.AddCommand(monitorCmd())
	root.AddCommand(completionCmd(root))

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
