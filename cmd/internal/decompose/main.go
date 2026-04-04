package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iamchrisrice/sidings/pkg/classifier"
	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/router"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type decomposeConfig struct {
	OllamaURL string `yaml:"ollama_url"`
}

func loadConfig() decomposeConfig {
	cfg := decomposeConfig{OllamaURL: "http://localhost:11434"}
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(home + "/.sidings/decompose.yaml")
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}

// gatherContext collects project context for the decomposition prompt.
func gatherContext() string {
	var sb strings.Builder

	// 1. git ls-files
	if out, err := exec.Command("git", "ls-files").Output(); err == nil {
		sb.WriteString("Tracked files:\n")
		sb.WriteString(string(out))
		sb.WriteString("\n")
	}

	// 2. Key files
	for _, name := range []string{"go.mod", "README.md"} {
		if data, err := os.ReadFile(name); err == nil {
			fmt.Fprintf(&sb, "--- %s ---\n%s\n", name, string(data))
		}
	}

	// 3. Up to 5 most recently modified .go files
	type goFile struct {
		path    string
		modTime time.Time
	}
	var goFiles []goFile
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.Contains(path, ".git/") {
			goFiles = append(goFiles, goFile{path: path, modTime: info.ModTime()})
		}
		return nil
	})
	sort.Slice(goFiles, func(i, j int) bool {
		return goFiles[i].modTime.After(goFiles[j].modTime)
	})
	limit := 5
	if len(goFiles) < limit {
		limit = len(goFiles)
	}
	for _, f := range goFiles[:limit] {
		if data, err := os.ReadFile(f.path); err == nil {
			fmt.Fprintf(&sb, "--- %s ---\n%s\n", f.path, string(data))
		}
	}

	return sb.String()
}

const promptTemplate = `You are decomposing a software engineering task into independent subtasks that can be executed in parallel by separate agents.

Project context:
<context>
%s
</context>

Parent task: %s

Rules:
- Each subtask must be self-contained and independently executable
- Subtasks must not depend on each other (no ordering required)
- Each subtask should be a single, specific code change
- Reference real filenames and packages from the project context
- Return ONLY a JSON array of strings, no other text
- 3 to 6 subtasks maximum

Example output:
["add User model to models/user.go", "add POST /login endpoint to handlers/auth.go", "add JWT validation middleware to middleware/auth.go"]`

func buildPrompt(content, ctx string) string {
	return fmt.Sprintf(promptTemplate, ctx, content)
}

// modelCaller is a function that calls the model with a prompt and returns the raw response.
// Swappable in tests to avoid requiring a real claude subprocess.
type modelCaller func(task *pipe.Task, prompt, ollamaURL string) (string, error)

// callClaude runs `claude --print` as a subprocess and returns the response text.
// For local tiers it points Claude Code at Ollama via environment variables.
func callClaude(task *pipe.Task, prompt, ollamaURL string) (string, error) {
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
	}
	if task.Route != nil && task.Route.Model != "" {
		args = append(args, "--model", task.Route.Model)
	}
	args = append(args, prompt)

	cmd := exec.Command("claude", args...)
	cmd.Stderr = os.Stderr

	if task.Tier != "exceptional" {
		url := ollamaURL
		if url == "" {
			url = "http://localhost:11434"
		}
		cmd.Env = append(os.Environ(),
			"ANTHROPIC_BASE_URL="+url,
			"ANTHROPIC_AUTH_TOKEN=ollama",
		)
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude: %w", err)
	}
	return string(out), nil
}

// parseSubtasks parses a JSON array of strings from the model response.
// Strips markdown code fences if present.
func parseSubtasks(response string) ([]string, error) {
	s := strings.TrimSpace(response)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	var subtasks []string
	if err := json.Unmarshal([]byte(s), &subtasks); err != nil {
		return nil, fmt.Errorf("invalid JSON from model: %w", err)
	}
	return subtasks, nil
}

func run(stdin io.Reader, stdout io.Writer, ollamaURL string, caller modelCaller) error {
	task, err := pipe.Read(stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	// Classify and route if not already done.
	if task.Tier == "" {
		cfg := classifier.DefaultConfig()
		cfg.OllamaURL = ollamaURL
		c := classifier.New(cfg)
		result, classErr := c.Classify(task.Content)
		if classErr != nil {
			return fmt.Errorf("classifying task: %w", classErr)
		}
		task.Tier = result.Tier
		task.Method = result.Method
	}
	if task.Route == nil {
		table := router.LoadConfig()
		r := router.New(table)
		decision, routeErr := r.Route(task.Tier)
		if routeErr != nil {
			return fmt.Errorf("routing task: %w", routeErr)
		}
		task.Route = &pipe.Route{Model: decision.Model}
	}

	ctx := gatherContext()
	prompt := buildPrompt(task.Content, ctx)

	response, err := caller(task, prompt, ollamaURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sidings: decompose failed: %v — emitting original task\n", err)
		return pipe.Write(stdout, task)
	}

	subtasks, parseErr := parseSubtasks(response)
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "sidings: %v — emitting original task\n", parseErr)
		return pipe.Write(stdout, task)
	}
	if len(subtasks) == 0 {
		fmt.Fprintf(os.Stderr, "sidings: model returned empty subtask list — emitting original task\n")
		return pipe.Write(stdout, task)
	}

	for _, content := range subtasks {
		subtask := &pipe.Task{
			TaskID:        uuid.New().String(),
			Content:       content,
			ParentTaskID:  task.TaskID,
			ParentContent: task.Content,
		}
		if err := pipe.Write(stdout, subtask); err != nil {
			return err
		}
	}

	telemetry.Emit(telemetry.Event{
		Tool:   "task-decompose",
		TaskID: task.TaskID,
		Tier:   task.Tier,
		Status: "complete",
	})

	return nil
}

func main() {
	var verbose bool
	var ollamaURL string

	root := &cobra.Command{
		Use:          "task-decompose",
		Short:        "Decompose a task into independent subtasks",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			if ollamaURL != "" {
				cfg.OllamaURL = ollamaURL
			}
			if verbose {
				fmt.Fprintln(os.Stderr, "sidings: decomposing task...")
			}
			return run(os.Stdin, os.Stdout, cfg.OllamaURL, callClaude)
		},
	}

	root.Flags().BoolVar(&verbose, "verbose", false, "print decomposition status to stderr")
	root.Flags().StringVar(&ollamaURL, "ollama-url", "", "override Ollama URL")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
