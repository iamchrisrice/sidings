package executor

import (
	"fmt"
	"os"
	"os/exec"
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
	err := cmd.Run()

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
