package main

import (
	"fmt"
	"os"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/router"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/spf13/cobra"
)

func main() {
	var verbose bool

	root := &cobra.Command{
		Use:          "task-route",
		Short:        "Add a routing decision to a classified task",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := pipe.Read(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}

			if task.Tier == "" {
				fmt.Fprintln(os.Stderr, "task-route: warning: task has no tier, defaulting to medium")
				task.Tier = "medium"
			}

			table := router.LoadConfig()
			r := router.New(table)

			d, err := r.Route(task.Tier)
			if err != nil {
				return err
			}

			// Warn on unknown tier (route fell back to medium).
			if _, known := table[task.Tier]; !known {
				fmt.Fprintf(os.Stderr, "task-route: warning: unknown tier %q, routing as medium\n", task.Tier)
			}

			if verbose {
				fmt.Fprintf(os.Stderr, "→ %s: %s %s\n", task.Tier, d.Backend, d.Model)
			}

			task.Route = &pipe.Route{
				Backend: d.Backend,
				Model:   d.Model,
			}

			telemetry.Emit(telemetry.Event{
				Tool:    "task-route",
				TaskID:  task.TaskID,
				Tier:    task.Tier,
				Backend: d.Backend,
				Model:   d.Model,
			})

			return pipe.Write(os.Stdout, task)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "log routing decision to stderr")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
