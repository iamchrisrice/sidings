package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/tty"
)

// ErrSettingsConflict is returned when .claude/settings.json has settings that
// would prevent sidings from working correctly. The error message has already
// been printed to stderr; callers should exit non-zero without printing more.
var ErrSettingsConflict = errors.New("settings conflict")

// ClaudeExecutor executes tasks via the Claude Code CLI.
// For non-exceptional tiers it points Claude Code at Ollama via environment variables.
type ClaudeExecutor struct {
	OllamaURL string // default: http://localhost:11434
}

// NewClaude creates a ClaudeExecutor with the given Ollama URL.
func NewClaude(ollamaURL string) Executor {
	return &ClaudeExecutor{OllamaURL: ollamaURL}
}

func (e *ClaudeExecutor) Execute(task pipe.Task, verbose bool) (Result, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Result{}, fmt.Errorf("getting working directory: %w", err)
	}

	if err := ensureClaudeSettings(dir, verbose); err != nil {
		if errors.Is(err, ErrSettingsConflict) {
			return Result{}, err
		}
		fmt.Fprintf(os.Stderr, "sidings: warning: could not configure .claude/settings.json: %v\n", err)
	}

	before, _ := gitModifiedFiles(dir)
	start := time.Now()

	args := buildArgs(task)
	cmd := exec.Command("claude", args...)
	cmd.Stdin = tty.Reader()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if env := buildEnv(task.Tier, e.ollamaURL()); env != nil {
		cmd.Env = env
	}

	if err := cmd.Run(); err != nil {
		return Result{DurationMS: time.Since(start).Milliseconds()}, fmt.Errorf("claude: %w", err)
	}

	duration := time.Since(start)

	after, _ := gitModifiedFiles(dir)
	filesWritten := diffFiles(before, after)

	if len(filesWritten) > 0 {
		fmt.Fprintf(os.Stderr, "✓ wrote %d files (%.1fs)\n", len(filesWritten), duration.Seconds())
	} else {
		fmt.Fprintf(os.Stderr, "✓ done (%.1fs)\n", duration.Seconds())
	}

	return Result{
		FilesWritten: filesWritten,
		DurationMS:   duration.Milliseconds(),
	}, nil
}

func (e *ClaudeExecutor) ollamaURL() string {
	if e.OllamaURL == "" {
		return "http://localhost:11434"
	}
	return e.OllamaURL
}

// buildArgs constructs the claude CLI argument list for the given task.
func buildArgs(task pipe.Task) []string {
	args := []string{"--dangerously-skip-permissions", "-p", task.Content}
	if task.Route != nil && task.Route.Model != "" {
		args = append(args, "--model", task.Route.Model)
	}
	return args
}

// buildEnv returns the environment to pass to the claude process.
// Returns nil for exceptional tier (use ambient environment).
// Returns an augmented env for local tiers that points Claude Code at Ollama.
func buildEnv(tier, ollamaURL string) []string {
	if tier == "exceptional" {
		return nil
	}
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	return append(os.Environ(),
		"ANTHROPIC_BASE_URL="+ollamaURL,
		"ANTHROPIC_AUTH_TOKEN=ollama",
	)
}

// gitModifiedFiles returns the set of files reported by git status --porcelain.
// Returns an empty map (not an error) if the directory is not a git repo.
func gitModifiedFiles(dir string) (map[string]struct{}, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return map[string]struct{}{}, nil
	}
	files := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path != "" {
			files[path] = struct{}{}
		}
	}
	return files, nil
}

// diffFiles returns a sorted slice of files present in after but not in before.
func diffFiles(before, after map[string]struct{}) []string {
	var result []string
	for f := range after {
		if _, existed := before[f]; !existed {
			result = append(result, f)
		}
	}
	sort.Strings(result)
	return result
}

// ensureClaudeSettings creates or updates .claude/settings.json in dir so that
// sandbox mode is enabled. It detects and rejects conflicting settings.
func ensureClaudeSettings(dir string, verbose bool) error {
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
			return fmt.Errorf("creating .claude directory: %w", err)
		}
		if err := os.WriteFile(settingsPath, []byte(defaultSettings()), 0644); err != nil {
			return fmt.Errorf("writing .claude/settings.json: %w", err)
		}
		if verbose {
			fmt.Fprintln(os.Stderr, "sidings: created .claude/settings.json (sandbox enabled)")
		}
		return nil
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("reading .claude/settings.json: %w", err)
	}

	var existing map[string]interface{}
	if err := json.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("parsing .claude/settings.json: %w", err)
	}

	if clashes := detectClashes(existing); len(clashes) > 0 {
		fmt.Fprintln(os.Stderr, "sidings: cannot proceed — .claude/settings.json has conflicting settings:")
		for _, clash := range clashes {
			fmt.Fprintf(os.Stderr, "  %s\n", clash)
		}
		fmt.Fprintln(os.Stderr, "sidings: resolve these settings manually and retry")
		return ErrSettingsConflict
	}

	merged, changed := mergeSettings(existing)
	if !changed {
		return nil
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising .claude/settings.json: %w", err)
	}

	if err := os.WriteFile(settingsPath, out, 0644); err != nil {
		return fmt.Errorf("writing .claude/settings.json: %w", err)
	}

	if verbose {
		fmt.Fprintln(os.Stderr, "sidings: updated .claude/settings.json (sandbox enabled)")
	}
	return nil
}

func defaultSettings() string {
	return `{
  "permissions": {
    "defaultMode": "acceptEdits"
  },
  "sandbox": {
    "enabled": true,
    "autoAllowBashIfSandboxed": true
  }
}
`
}

func detectClashes(settings map[string]interface{}) []string {
	var clashes []string

	if sandbox, ok := settings["sandbox"].(map[string]interface{}); ok {
		if enabled, ok := sandbox["enabled"].(bool); ok && !enabled {
			clashes = append(clashes, `"sandbox.enabled" is false — sidings requires sandbox to be enabled`)
		}
	}

	if perms, ok := settings["permissions"].(map[string]interface{}); ok {
		if mode, ok := perms["disableBypassPermissionsMode"].(string); ok && mode == "disable" {
			clashes = append(clashes, `"permissions.disableBypassPermissionsMode" is "disable" — this prevents --dangerously-skip-permissions from working`)
		}
	}

	return clashes
}

func mergeSettings(settings map[string]interface{}) (map[string]interface{}, bool) {
	changed := false

	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		sandbox = map[string]interface{}{}
		settings["sandbox"] = sandbox
	}
	if _, ok := sandbox["enabled"]; !ok {
		sandbox["enabled"] = true
		changed = true
	}
	if _, ok := sandbox["autoAllowBashIfSandboxed"]; !ok {
		sandbox["autoAllowBashIfSandboxed"] = true
		changed = true
	}

	perms, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		perms = map[string]interface{}{}
		settings["permissions"] = perms
	}
	if _, ok := perms["defaultMode"]; !ok {
		perms["defaultMode"] = "acceptEdits"
		changed = true
	}

	return settings, changed
}
