package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iamchrisrice/sidings/pkg/executor"
	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/iamchrisrice/sidings/pkg/tty"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type dispatchConfig struct {
	OllamaURL        string `yaml:"ollama_url"`
	SkipConfirmation bool   `yaml:"skip_confirmation"`
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

func isTTY() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func main() {
	var yes bool
	var dryRun bool

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

			if task.Route.Backend == "claude" && !yes && !cfg.SkipConfirmation {
				if !tty.Confirm("⚠️  routing to claude sonnet — continue?") {
					task.Status = "failed"
					task.Error = "aborted by user"
					_ = pipe.Write(os.Stdout, task)
					return fmt.Errorf("aborted by user")
				}
			}

			var backend executor.Executor
			switch task.Route.Backend {
			case "claude":
				backend = executor.NewClaude()
			default:
				backend = executor.NewOllama(executor.OllamaConfig{
					OllamaURL: cfg.OllamaURL,
					Yes:       yes || cfg.SkipConfirmation,
					DryRun:    dryRun,
					TTY:       isTTY(),
				})
			}

			result, err := backend.Execute(*task)
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

	root.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompts")
	root.Flags().BoolVar(&dryRun, "dry-run", false, "print built prompt to stderr, don't execute")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
