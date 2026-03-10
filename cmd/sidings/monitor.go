package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func monitorCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "monitor",
		Short:              "Live pipeline monitoring",
		Long:               "Connects to the sidings telemetry socket and displays live pipeline events.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := libexecPath("monitor")
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
