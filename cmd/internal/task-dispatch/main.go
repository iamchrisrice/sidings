package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iamchrisrice/sidings/pkg/executor"
	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type dispatchConfig struct {
	OllamaURL string `yaml:"ollama_url"`
}

func loadConfig() dispatchConfig {
	cfg := dispatchConfig{OllamaURL: "http://localhost:11434"}
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(home + "/.sidings/dispatch.yaml")
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}

func main() {
	var dryRun bool
	var verbose bool

	root := &cobra.Command{
		Use:          "task-dispatch",
		Short:        "Execute a classified and routed task",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := pipe.Read(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}

			if task.Route == nil {
				return fmt.Errorf("task has no route — run through task-route first")
			}

			cfg := loadConfig()
			start := time.Now()

			telemetry.Emit(telemetry.Event{
				Tool:    "task-dispatch",
				TaskID:  task.TaskID,
				Tier:    task.Tier,
				Backend: task.Route.Backend,
				Model:   task.Route.Model,
				Status:  "running",
			})

			var backend executor.Executor
			switch task.Route.Backend {
			case "claude":
				backend = executor.NewClaude()
			default:
				backend = executor.NewOllama(executor.OllamaConfig{
					OllamaURL: cfg.OllamaURL,
					DryRun:    dryRun,
				})
			}

			result, err := backend.Execute(*task, verbose)
			task.DurationMS = time.Since(start).Milliseconds()

			if err != nil {
				task.Status = "failed"
				task.Error = err.Error()
				telemetry.Emit(telemetry.Event{
					Tool:       "task-dispatch",
					TaskID:     task.TaskID,
					Backend:    task.Route.Backend,
					Model:      task.Route.Model,
					Status:     "failed",
					DurationMS: task.DurationMS,
				})
				_ = pipe.Write(os.Stdout, task)
				return err
			}

			task.Status = "complete"
			task.FilesWritten = result.FilesWritten
			if result.Output != "" {
				task.Result = result.Output
			}

			telemetry.Emit(telemetry.Event{
				Tool:       "task-dispatch",
				TaskID:     task.TaskID,
				Backend:    task.Route.Backend,
				Model:      task.Route.Model,
				Status:     "complete",
				DurationMS: task.DurationMS,
			})

			return pipe.Write(os.Stdout, task)
		},
	}

	root.Flags().BoolVar(&dryRun, "dry-run", false, "print built prompt to stderr, don't execute")
	root.Flags().BoolVar(&verbose, "verbose", false, "show routing, file writes, and token streaming")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
