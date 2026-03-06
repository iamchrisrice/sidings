package pipe_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// --- Read ---

func TestReadWrapsPlainTextWithGeneratedTaskID(t *testing.T) {
	task, err := pipe.Read(strings.NewReader("rename this variable\n"))
	if err != nil {
		t.Fatal(err)
	}
	if task.Content != "rename this variable" {
		t.Errorf("content = %q, want %q", task.Content, "rename this variable")
	}
	if task.TaskID == "" {
		t.Error("task_id should be generated for plain text input")
	}
}

func TestReadParsesValidNDJSON(t *testing.T) {
	input := `{"task_id":"abc123","content":"fix the bug","tier":"simple"}` + "\n"
	task, err := pipe.Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != "abc123" {
		t.Errorf("task_id = %q, want %q", task.TaskID, "abc123")
	}
	if task.Content != "fix the bug" {
		t.Errorf("content = %q, want %q", task.Content, "fix the bug")
	}
	if task.Tier != "simple" {
		t.Errorf("tier = %q, want %q", task.Tier, "simple")
	}
}

func TestReadPreservesExistingTaskID(t *testing.T) {
	input := `{"task_id":"preserved-id","content":"some task"}` + "\n"
	task, err := pipe.Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != "preserved-id" {
		t.Errorf("task_id = %q, want preserved-id", task.TaskID)
	}
}

func TestReadGeneratesNewTaskIDWhenAbsent(t *testing.T) {
	// Plain text carries no task_id — Read must generate one.
	task, err := pipe.Read(strings.NewReader("some plain text task"))
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID == "" {
		t.Error("expected a generated task_id, got empty string")
	}
}

func TestReadReturnsEOFOnEmptyInput(t *testing.T) {
	_, err := pipe.Read(strings.NewReader(""))
	if err != io.EOF {
		t.Errorf("expected io.EOF on empty input, got %v", err)
	}
}

func TestReadPlainTextWithoutNewline(t *testing.T) {
	// Scanners should handle input that lacks a trailing newline.
	task, err := pipe.Read(strings.NewReader("no newline at end"))
	if err != nil {
		t.Fatal(err)
	}
	if task.Content != "no newline at end" {
		t.Errorf("content = %q", task.Content)
	}
}

// --- Write ---

func TestWriteProducesSingleLineOfValidJSON(t *testing.T) {
	task := &pipe.Task{TaskID: "abc123", Content: "fix the bug", Tier: "simple"}
	var buf bytes.Buffer
	if err := pipe.Write(&buf, task); err != nil {
		t.Fatal(err)
	}
	line := buf.String()

	if !strings.HasSuffix(line, "\n") {
		t.Error("Write output must end with a newline (NDJSON)")
	}
	body := strings.TrimRight(line, "\n")
	if strings.Contains(body, "\n") {
		t.Error("Write must produce exactly one line")
	}
	var got pipe.Task
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Write output is not valid JSON: %v", err)
	}
	if got.TaskID != task.TaskID {
		t.Errorf("task_id = %q, want %q", got.TaskID, task.TaskID)
	}
}

func TestWriteOutputIsReadableByRead(t *testing.T) {
	original := &pipe.Task{
		TaskID:  "round-trip-id",
		Content: "refactor the auth module",
		Tier:    "complex",
	}
	var buf bytes.Buffer
	if err := pipe.Write(&buf, original); err != nil {
		t.Fatal(err)
	}
	got, err := pipe.Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != original.TaskID {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, original.TaskID)
	}
	if got.Content != original.Content {
		t.Errorf("Content: got %q, want %q", got.Content, original.Content)
	}
	if got.Tier != original.Tier {
		t.Errorf("Tier: got %q, want %q", got.Tier, original.Tier)
	}
}

func TestWriteAndReadPreservesMethodAndMatched(t *testing.T) {
	original := &pipe.Task{
		TaskID:  "meta-id",
		Content: "refactor the auth module",
		Tier:    "complex",
		Method:  "heuristic",
		Matched: []string{"refactor"},
	}
	var buf bytes.Buffer
	if err := pipe.Write(&buf, original); err != nil {
		t.Fatal(err)
	}
	got, err := pipe.Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != original.Method {
		t.Errorf("Method: got %q, want %q", got.Method, original.Method)
	}
	if len(got.Matched) != len(original.Matched) || got.Matched[0] != original.Matched[0] {
		t.Errorf("Matched: got %v, want %v", got.Matched, original.Matched)
	}
}

func TestWriteOmitsZeroValueFields(t *testing.T) {
	task := &pipe.Task{TaskID: "x", Content: "hello"}
	var buf bytes.Buffer
	_ = pipe.Write(&buf, task)
	line := buf.String()
	for _, field := range []string{`"tier"`, `"route"`, `"result"`, `"status"`, `"error"`, `"duration_ms"`} {
		if strings.Contains(line, field) {
			t.Errorf("expected %s to be omitted (omitempty), got: %s", field, line)
		}
	}
}
