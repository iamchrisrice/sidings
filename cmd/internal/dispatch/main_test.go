package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/executor"
	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// mockExecutor is an in-process executor for testing.
type mockExecutor struct {
	result executor.Result
	err    error
}

func (m *mockExecutor) Execute(task pipe.Task, verbose bool) (executor.Result, error) {
	return m.result, m.err
}

// alwaysAvailable is an ollamaAvailable stub that never warns.
func alwaysAvailable(string) bool { return true }

func parseTask(t *testing.T, output string) pipe.Task {
	t.Helper()
	var task pipe.Task
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &task); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	return task
}

func TestRunNoRouteReturnsError(t *testing.T) {
	input := `{"task_id":"t1","content":"some task","tier":"medium"}`
	var stdout bytes.Buffer
	ex := &mockExecutor{result: executor.Result{}}

	err := run(strings.NewReader(input), &stdout, ex, alwaysAvailable, "", false)
	if err == nil {
		t.Fatal("expected error for missing route, got nil")
	}
}

func TestRunSuccessfulExecutionSetsStatusComplete(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{
		result: executor.Result{FilesWritten: []string{"pkg/foo.go"}, DurationMS: 1234},
	}

	if err := run(strings.NewReader(input), &stdout, ex, alwaysAvailable, "", false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Status != "complete" {
		t.Errorf("status = %q, want complete", task.Status)
	}
	if len(task.FilesWritten) != 1 || task.FilesWritten[0] != "pkg/foo.go" {
		t.Errorf("files_written = %v, want [pkg/foo.go]", task.FilesWritten)
	}
	if task.DurationMS != 1234 {
		t.Errorf("duration_ms = %d, want 1234", task.DurationMS)
	}
}

func TestRunFailedExecutionSetsStatusFailed(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{
		result: executor.Result{DurationMS: 500},
		err:    fmt.Errorf("claude exited with code 1"),
	}

	err := run(strings.NewReader(input), &stdout, ex, alwaysAvailable, "", false)
	if err == nil {
		t.Fatal("expected error from failed execution")
	}

	// Even on failure, the task is written to stdout with status=failed.
	task := parseTask(t, stdout.String())
	if task.Status != "failed" {
		t.Errorf("status = %q, want failed", task.Status)
	}
	if task.Error == "" {
		t.Error("error field should be set on failure")
	}
	if task.DurationMS != 500 {
		t.Errorf("duration_ms = %d, want 500", task.DurationMS)
	}
}

func TestRunOutputIsValidNDJSON(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{result: executor.Result{}}

	if err := run(strings.NewReader(input), &stdout, ex, alwaysAvailable, "", false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestRunPreservesTaskID(t *testing.T) {
	input := `{"task_id":"keep-me","content":"task","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{result: executor.Result{}}

	if err := run(strings.NewReader(input), &stdout, ex, alwaysAvailable, "", false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.TaskID != "keep-me" {
		t.Errorf("task_id = %q, want keep-me", task.TaskID)
	}
}

func TestRunOllamaUnavailableWarnsForLocalTier(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"medium","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{result: executor.Result{}}

	warned := false
	unavailable := func(string) bool {
		warned = true
		return false
	}

	if err := run(strings.NewReader(input), &stdout, ex, unavailable, "http://localhost:11434", false); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !warned {
		t.Error("expected ollamaAvailable to be called for local tier")
	}
}

func TestRunOllamaUnavailableNoWarnForExceptionalTier(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"exceptional","route":{"model":""}}`
	var stdout bytes.Buffer
	ex := &mockExecutor{result: executor.Result{}}

	called := false
	available := func(string) bool {
		called = true
		return false
	}

	if err := run(strings.NewReader(input), &stdout, ex, available, "", false); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if called {
		t.Error("ollamaAvailable should not be called for exceptional tier")
	}
}

func TestCheckOllamaReturnsTrueWhenUp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if !checkOllama(ts.URL) {
		t.Error("expected checkOllama to return true for reachable server")
	}
}

func TestCheckOllamaReturnsFalseWhenDown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // shut it down immediately

	if checkOllama(ts.URL) {
		t.Error("expected checkOllama to return false for unreachable server")
	}
}

func TestLoadConfigDefaultsWhenNoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := loadConfig()
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q, want default", cfg.OllamaURL)
	}
}
