// Package pipe defines the shared NDJSON types for inter-tool communication.
// Every tool in the sidings pipeline reads a Task from stdin, enriches it,
// and writes it to stdout as a single JSON line.
package pipe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
)

// Task is the core type flowing through the sidings pipeline.
type Task struct {
	TaskID       string   `json:"task_id"`
	Content      string   `json:"content"`
	Tier         string   `json:"tier,omitempty"`
	Route        *Route   `json:"route,omitempty"`
	Result       string   `json:"result,omitempty"`
	Status       string   `json:"status,omitempty"`
	Error        string   `json:"error,omitempty"`
	DurationMS   int64    `json:"duration_ms,omitempty"`
	FilesWritten []string `json:"files_written,omitempty"`
}

// Route describes the selected execution backend.
type Route struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
}

// NewTask creates a Task from plain text content with a fresh UUID.
func NewTask(content string) *Task {
	return &Task{
		TaskID:  uuid.New().String(),
		Content: content,
	}
}

// Read reads a single Task from r.
// Accepts either a JSON object or plain text. Plain text is wrapped
// automatically: content is set to the raw line and a task_id is generated.
func Read(r io.Reader) (*Task, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	line := scanner.Text()

	var t Task
	if err := json.Unmarshal([]byte(line), &t); err != nil {
		// Not valid JSON — treat as plain text content.
		return NewTask(line), nil
	}
	return &t, nil
}

// Write serialises t as a single NDJSON line (JSON + newline) to w.
func Write(w io.Writer, t *Task) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}
