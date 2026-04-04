package main

import (
	"fmt"
	"net/http"
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

func checkOllama(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func main() {
	var verbose bool
	var ollamaURL string

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
			if ollamaURL != "" {
				cfg.OllamaURL = ollamaURL
			}

			// Warn if Ollama is unreachable for local tiers.
			if task.Tier != "exceptional" && !checkOllama(cfg.OllamaURL) {
				fmt.Fprintf(os.Stderr, "sidings: warning: ollama not available at %s — local model tasks will fail\n", cfg.OllamaURL)
			}

			telemetry.Emit(telemetry.Event{
				Tool:   "task-dispatch",
				TaskID: task.TaskID,
				Tier:   task.Tier,
				Model:  task.Route.Model,
				Status: "running",
			})

			ex := executor.NewClaude(cfg.OllamaURL)
			result, err := ex.Execute(*task, verbose)
			if err != nil {
				task.Status = "failed"
				task.Error = err.Error()
				task.DurationMS = result.DurationMS
				telemetry.Emit(telemetry.Event{
					Tool:       "task-dispatch",
					TaskID:     task.TaskID,
					Status:     "failed",
					DurationMS: task.DurationMS,
				})
				_ = pipe.Write(os.Stdout, task)
				return err
			}

			task.Status = "complete"
			task.FilesWritten = result.FilesWritten
			task.DurationMS = result.DurationMS

			telemetry.Emit(telemetry.Event{
				Tool:       "task-dispatch",
				TaskID:     task.TaskID,
				Status:     "complete",
				DurationMS: task.DurationMS,
			})

			return pipe.Write(os.Stdout, task)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "show file writes and routing details")
	root.Flags().StringVar(&ollamaURL, "ollama-url", "", "override Ollama URL (default: from config or http://localhost:11434)")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
