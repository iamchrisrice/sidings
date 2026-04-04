package main

import (
	"fmt"
	"io"
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

func run(stdin io.Reader, stdout io.Writer, c classifier.Classifier, verbose bool) error {
	task, err := pipe.Read(stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
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
	return pipe.Write(stdout, task)
}

func main() {
	var verbose bool

	root := &cobra.Command{
		Use:          "task-classify [task]",
		Short:        "Classify a coding task into a routing tier",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var reader io.Reader = os.Stdin
			if len(args) > 0 {
				reader = strings.NewReader(strings.Join(args, " "))
			}
			cfg := loadConfig()
			return run(reader, os.Stdout, classifier.New(cfg), verbose)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "print classification result to stderr")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
