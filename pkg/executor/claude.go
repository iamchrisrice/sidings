package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/iamchrisrice/sidings/pkg/tty"
)

type claudeExecutor struct{}

// NewClaude creates an Executor that delegates to the Claude Code CLI.
// Claude Code handles its own file operations, confirmation prompts, and output.
// stdio passes through directly.
func NewClaude() Executor {
	return &claudeExecutor{}
}

func (e *claudeExecutor) Execute(task pipe.Task) (Result, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Result{}, fmt.Errorf("getting working directory: %w", err)
	}

	if err := ensureClaudePermissions(dir); err != nil {
		fmt.Fprintf(os.Stderr, "sidings: warning: could not create .claude/settings.json: %v\n", err)
	}

	start := time.Now()

	telemetry.Emit(telemetry.Event{
		Tool:    "task-dispatch",
		TaskID:  task.TaskID,
		Backend: "claude",
		Status:  "running",
	})

	cmd := exec.Command("claude", "-p", task.Content)
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
	return Result{}, nil
}

// ensureClaudePermissions creates .claude/settings.json in dir if it doesn't
// already exist, granting Claude Code file write access to the directory.
// This prevents Claude Code from prompting for permissions mid-pipeline.
func ensureClaudePermissions(dir string) error {
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	if _, err := os.Stat(settingsPath); err == nil {
		return nil // already exists, nothing to do
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	settings := "{\n  \"permissions\": {\n    \"defaultMode\": \"acceptEdits\"\n  }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
		return fmt.Errorf("writing .claude/settings.json: %w", err)
	}

	fmt.Fprintln(os.Stderr, "sidings: created .claude/settings.json to allow Claude Code file writes")
	return nil
}
