package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/iamchrisrice/sidings/pkg/tty"
)

// ErrSettingsConflict is returned when .claude/settings.json has settings that
// would prevent sidings from working correctly. The error message has already
// been printed to stderr; callers should exit non-zero without printing more.
var ErrSettingsConflict = errors.New("settings conflict")

type claudeExecutor struct{}

// NewClaude creates an Executor that delegates to the Claude Code CLI.
// Claude Code handles its own file operations, confirmation prompts, and output.
// stdio passes through directly.
func NewClaude() Executor {
	return &claudeExecutor{}
}

func (e *claudeExecutor) Execute(task pipe.Task, verbose bool) (Result, error) {
	start := time.Now()

	dir, err := os.Getwd()
	if err != nil {
		return Result{}, fmt.Errorf("getting working directory: %w", err)
	}

	if err := ensureClaudeSettings(dir, verbose); err != nil {
		if errors.Is(err, ErrSettingsConflict) {
			// Message already printed to stderr by ensureClaudeSettings.
			return Result{}, err
		}
		// Non-conflict error — warn but continue.
		fmt.Fprintf(os.Stderr, "sidings: warning: could not configure .claude/settings.json: %v\n", err)
	}

	telemetry.Emit(telemetry.Event{
		Tool:    "task-dispatch",
		TaskID:  task.TaskID,
		Backend: "claude",
		Status:  "running",
	})

	cmd := exec.Command("claude", "--dangerously-skip-permissions", "-p", task.Content)
	cmd.Stdin = tty.Reader()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	status := "complete"
	if err != nil {
		status = "failed"
	}

	telemetry.Emit(telemetry.Event{
		Tool:       "task-dispatch",
		TaskID:     task.TaskID,
		Backend:    "claude",
		Status:     status,
		DurationMS: time.Since(start).Milliseconds(),
	})

	if err != nil {
		return Result{}, fmt.Errorf("claude: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ done (%.1fs)\n", time.Since(start).Seconds())
	return Result{}, nil
}

// ensureClaudeSettings creates or updates .claude/settings.json in dir so that
// sandbox mode is enabled. It detects and rejects conflicting settings.
func ensureClaudeSettings(dir string, verbose bool) error {
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	// Case 1: file missing — create with correct settings.
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

	// Case 2 & 3: file exists — read, check for clashes, merge.
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

	// No clashes — merge in sandbox settings if missing.
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

// detectClashes returns human-readable descriptions of any conflicting settings.
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

// mergeSettings adds sandbox settings if missing. Returns updated map and whether anything changed.
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
