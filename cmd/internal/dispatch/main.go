package main

import (
	"fmt"
	"io"
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

func run(stdin io.Reader, stdout io.Writer, ex executor.Executor, ollamaAvailable func(string) bool, ollamaURL string, verbose bool) error {
	task, err := pipe.Read(stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	if task.Route == nil {
		return fmt.Errorf("task has no route — run through task-route first")
	}
	if task.Tier != "exceptional" && !ollamaAvailable(ollamaURL) {
		fmt.Fprintf(os.Stderr, "sidings: warning: ollama not available at %s — local model tasks will fail\n", ollamaURL)
	}
	telemetry.Emit(telemetry.Event{
		Tool:   "task-dispatch",
		TaskID: task.TaskID,
		Tier:   task.Tier,
		Model:  task.Route.Model,
		Status: "running",
	})
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
		_ = pipe.Write(stdout, task)
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
	return pipe.Write(stdout, task)
}

func main() {
	var verbose bool
	var ollamaURL string

	root := &cobra.Command{
		Use:          "task-dispatch",
		Short:        "Execute a classified and routed task",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			if ollamaURL != "" {
				cfg.OllamaURL = ollamaURL
			}
			return run(os.Stdin, os.Stdout, executor.NewClaude(cfg.OllamaURL), checkOllama, cfg.OllamaURL, verbose)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "show file writes and routing details")
	root.Flags().StringVar(&ollamaURL, "ollama-url", "", "override Ollama URL (default: from config or http://localhost:11434)")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
