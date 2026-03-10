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

	root.AddCommand(taskCmd())
	root.AddCommand(monitorCmd())
	root.AddCommand(completionCmd(root))

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
