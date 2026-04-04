package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// mockCaller returns a modelCaller that always returns the given response and error.
func mockCaller(response string, err error) modelCaller {
	return func(task *pipe.Task, prompt, ollamaURL string) (string, error) {
		return response, err
	}
}

// parseLines parses each non-empty line as a pipe.Task.
func parseLines(t *testing.T, output string) []*pipe.Task {
	t.Helper()
	var tasks []*pipe.Task
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var task pipe.Task
		if err := json.Unmarshal([]byte(line), &task); err != nil {
			t.Fatalf("output line is not valid JSON: %v\nline: %s", err, line)
		}
		tasks = append(tasks, &task)
	}
	return tasks
}

func TestPlainTextInput(t *testing.T) {
	subtasksJSON := `["subtask one", "subtask two", "subtask three"]`
	var stdout bytes.Buffer
	if err := run(strings.NewReader("do something simple"), &stdout, "", mockCaller(subtasksJSON, nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 3 {
		t.Fatalf("expected 3 subtasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.ParentContent != "do something simple" {
			t.Errorf("parent_content mismatch: got %q", task.ParentContent)
		}
	}
}

func TestNDJSONWithTierRouteSkipsClassification(t *testing.T) {
	callerInvoked := false
	caller := func(task *pipe.Task, prompt, ollamaURL string) (string, error) {
		callerInvoked = true
		return `["subtask 1", "subtask 2", "subtask 3"]`, nil
	}

	input := `{"task_id":"abc123","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", caller); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !callerInvoked {
		t.Error("model caller was never invoked")
	}
	tasks := parseLines(t, stdout.String())
	if len(tasks) != 3 {
		t.Fatalf("expected 3 subtasks, got %d", len(tasks))
	}
}

func TestNDJSONWithoutTierCallsClassifier(t *testing.T) {
	// Provide a mock Ollama server so the classifier can run.
	classifierCalled := false
	classifierSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		classifierCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": "medium"})
	}))
	defer classifierSrv.Close()

	callerInvoked := false
	caller := func(task *pipe.Task, prompt, ollamaURL string) (string, error) {
		callerInvoked = true
		return `["subtask 1", "subtask 2", "subtask 3"]`, nil
	}

	input := `{"task_id":"abc123","content":"do something"}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, classifierSrv.URL, caller); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !classifierCalled {
		t.Error("classifier (Ollama) was not called for untiered task")
	}
	if !callerInvoked {
		t.Error("model caller was never invoked")
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 3 {
		t.Fatalf("expected 3 subtasks, got %d", len(tasks))
	}
}

func TestOutputLinesAreValidNDJSON(t *testing.T) {
	input := `{"task_id":"abc123","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller(`["subtask 1", "subtask 2", "subtask 3"]`, nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	for i, task := range tasks {
		if task.TaskID == "" {
			t.Errorf("line %d: task_id is empty", i)
		}
		if task.Content == "" {
			t.Errorf("line %d: content is empty", i)
		}
		if task.ParentTaskID == "" {
			t.Errorf("line %d: parent_task_id is empty", i)
		}
		if task.ParentContent == "" {
			t.Errorf("line %d: parent_content is empty", i)
		}
	}
}

func TestParentTaskIDMatchesInput(t *testing.T) {
	input := `{"task_id":"my-task-id","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller(`["subtask 1", "subtask 2"]`, nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	for _, task := range tasks {
		if task.ParentTaskID != "my-task-id" {
			t.Errorf("parent_task_id = %q, want %q", task.ParentTaskID, "my-task-id")
		}
	}
}

func TestThreeSubtasksEmitsThreeLines(t *testing.T) {
	input := `{"task_id":"abc123","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller(`["sub1", "sub2", "sub3"]`, nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 3 {
		t.Errorf("expected 3 output lines, got %d", len(tasks))
	}
}

func TestInvalidJSONFromModelEmitsOriginalTask(t *testing.T) {
	input := `{"task_id":"abc123","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller("not valid json at all", nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 1 {
		t.Fatalf("expected 1 line (original task), got %d", len(tasks))
	}
	if tasks[0].TaskID != "abc123" {
		t.Errorf("expected original task_id abc123, got %s", tasks[0].TaskID)
	}
}

func TestEmptyArrayFromModelEmitsOriginalTask(t *testing.T) {
	input := `{"task_id":"abc123","content":"do something","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller(`[]`, nil)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 1 {
		t.Fatalf("expected 1 line (original task), got %d", len(tasks))
	}
	if tasks[0].TaskID != "abc123" {
		t.Errorf("expected original task_id abc123, got %s", tasks[0].TaskID)
	}
}

func TestModelErrorEmitsOriginalTask(t *testing.T) {
	input := `{"task_id":"abc123","content":"do something","tier":"exceptional","route":{"model":""}}`
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout, "", mockCaller("", fmt.Errorf("claude subprocess failed"))); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	tasks := parseLines(t, stdout.String())
	if len(tasks) != 1 {
		t.Fatalf("expected 1 line (original task), got %d", len(tasks))
	}
	if tasks[0].TaskID != "abc123" {
		t.Errorf("expected original task_id abc123, got %s", tasks[0].TaskID)
	}
}

func TestMarkdownFencesAreStripped(t *testing.T) {
	fenced := "```json\n[\"sub1\", \"sub2\", \"sub3\"]\n```"
	subtasks, err := parseSubtasks(fenced)
	if err != nil {
		t.Fatalf("parseSubtasks returned error: %v", err)
	}
	if len(subtasks) != 3 {
		t.Errorf("expected 3 subtasks, got %d", len(subtasks))
	}
}
