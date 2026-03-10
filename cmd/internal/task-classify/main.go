package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iamchrisrice/sidings/pkg/classifier"
	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// configFile mirrors ~/.sidings/classify.yaml
type configFile struct {
	OllamaURL string `yaml:"ollama_url"`
	Model     string `yaml:"model"`
}

func loadConfig() classifier.Config {
	cfg := classifier.DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(home + "/.sidings/classify.yaml")
	if err != nil {
		return cfg // file is optional
	}

	var f configFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return cfg
	}
	if f.OllamaURL != "" {
		cfg.OllamaURL = f.OllamaURL
	}
	if f.Model != "" {
		cfg.ClassifierModel = f.Model
	}
	return cfg
}

func main() {
	var verbose bool

	root := &cobra.Command{
		Use:          "task-classify [task]",
		Short:        "Classify a coding task into a routing tier",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var task *pipe.Task
			var err error

			if len(args) > 0 {
				task = pipe.NewTask(strings.Join(args, " "))
			} else {
				task, err = pipe.Read(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
			}

			cfg := loadConfig()
			c := classifier.New(cfg)
			result, err := c.Classify(task.Content)
			if err != nil {
				return err
			}
			task.Tier = result.Tier
			task.Method = result.Method

			if verbose {
				fmt.Fprintf(os.Stderr, "%s (%s)\n", result.Tier, result.Method)
			}

			telemetry.Emit(telemetry.Event{
				Tool:   "task-classify",
				TaskID: task.TaskID,
				Tier:   result.Tier,
				Method: result.Method,
			})

			return pipe.Write(os.Stdout, task)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "print classification result to stderr")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
