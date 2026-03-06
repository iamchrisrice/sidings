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

// configFile mirrors ~/.sidings/task-classify.yaml
type configFile struct {
	OllamaURL       string `yaml:"ollama_url"`
	ClassifierModel string `yaml:"classifier_model"`
	LLMFallback     *bool  `yaml:"llm_fallback"`
}

func loadConfig() classifier.Config {
	cfg := classifier.DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(home + "/.sidings/task-classify.yaml")
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
	if f.ClassifierModel != "" {
		cfg.ClassifierModel = f.ClassifierModel
	}
	if f.LLMFallback != nil {
		cfg.LLMFallback = *f.LLMFallback
	}
	return cfg
}

func main() {
	var verbose bool
	var noLLM bool
	var forceTier string

	root := &cobra.Command{
		Use:          "task-classify [task]",
		Short:        "Classify a coding task into a routing tier",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read task from positional arg or stdin.
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

			// --tier bypasses classification entirely.
			if forceTier != "" {
				task.Tier = forceTier
				if verbose {
					fmt.Fprintf(os.Stderr, "%s (forced via --tier)\n", forceTier)
				}
				return pipe.Write(os.Stdout, task)
			}

			cfg := loadConfig()
			if noLLM {
				cfg.LLMFallback = false
			}

			c := classifier.New(cfg)
			result, err := c.Classify(task.Content)
			if err != nil {
				return err
			}
			task.Tier = result.Tier

			if verbose {
				printVerbose(result)
			}

			telemetry.Emit(telemetry.Event{
				Tool:    "task-classify",
				TaskID:  task.TaskID,
				Tier:    result.Tier,
				Method:  result.Method,
				Matched: result.Matched,
			})

			return pipe.Write(os.Stdout, task)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "print classification reasoning to stderr")
	root.Flags().BoolVar(&noLLM, "no-llm", false, "disable LLM fallback, use heuristic/length/default only")
	root.Flags().StringVar(&forceTier, "tier", "", "force a specific tier (simple|medium|complex|exceptional)")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// printVerbose writes a human-readable classification summary to stderr.
// stdout is kept clean for piping.
func printVerbose(result classifier.Result) {
	switch result.Method {
	case "heuristic":
		others := 0
		for name, s := range result.Scores {
			if name != result.Tier {
				others += s
			}
		}
		kwStrs := make([]string, len(result.Matched))
		for i, kw := range result.Matched {
			kwStrs[i] = `"` + kw + `"`
		}
		fmt.Fprintf(os.Stderr, "%s (heuristic: matched %s, score %s:%d others:%d)\n",
			result.Tier,
			strings.Join(kwStrs, ", "),
			result.Tier,
			len(result.Matched),
			others,
		)
	default:
		fmt.Fprintf(os.Stderr, "%s (%s)\n", result.Tier, result.Method)
	}
}
