package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/classifier"
	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// ollamaServer starts a test Ollama-compatible server returning a fixed tier string.
func ollamaServer(t *testing.T, tier string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"response":%q}`, tier)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func classifierFor(ts *httptest.Server) classifier.Classifier {
	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	return classifier.New(cfg)
}

// parseTask parses a single NDJSON line as a pipe.Task.
func parseTask(t *testing.T, line string) pipe.Task {
	t.Helper()
	var task pipe.Task
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &task); err != nil {
		t.Fatalf("output is not valid JSON: %v\nline: %s", err, line)
	}
	return task
}

func TestRunNDJSONInputSetsTeir(t *testing.T) {
	ts := ollamaServer(t, "complex")
	input := `{"task_id":"abc","content":"refactor the auth module"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, classifierFor(ts), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Tier != "complex" {
		t.Errorf("tier = %q, want complex", task.Tier)
	}
	if task.Method != "llm" {
		t.Errorf("method = %q, want llm", task.Method)
	}
}

func TestRunPlainTextInputIsWrapped(t *testing.T) {
	ts := ollamaServer(t, "simple")
	var stdout bytes.Buffer

	if err := run(strings.NewReader("fix a typo"), &stdout, classifierFor(ts), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Content != "fix a typo" {
		t.Errorf("content = %q, want %q", task.Content, "fix a typo")
	}
	if task.TaskID == "" {
		t.Error("task_id should be set for plain text input")
	}
}

func TestRunOutputIsValidNDJSON(t *testing.T) {
	ts := ollamaServer(t, "medium")
	input := `{"task_id":"t1","content":"add a helper function"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, classifierFor(ts), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	out := strings.TrimSpace(stdout.String())
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 output line, got %d", len(lines))
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestRunPreservesExistingTaskFields(t *testing.T) {
	ts := ollamaServer(t, "medium")
	input := `{"task_id":"preserve-me","content":"add tests","route":{"model":"qwen3.5:9b"}}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, classifierFor(ts), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.TaskID != "preserve-me" {
		t.Errorf("task_id = %q, want preserve-me", task.TaskID)
	}
	if task.Route == nil || task.Route.Model != "qwen3.5:9b" {
		t.Errorf("route lost: %+v", task.Route)
	}
}

func TestRunOllamaUnavailableFallsBackToExceptional(t *testing.T) {
	// Use a closed server so Ollama is unreachable.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()

	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	c := classifier.New(cfg)

	var stdout bytes.Buffer
	if err := run(strings.NewReader("some task"), &stdout, c, false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (fallback)", task.Tier)
	}
	if task.Method != "fallback" {
		t.Errorf("method = %q, want fallback", task.Method)
	}
}

func TestRunAllFourTiers(t *testing.T) {
	for _, tier := range []string{"simple", "medium", "complex", "exceptional"} {
		tier := tier
		t.Run(tier, func(t *testing.T) {
			ts := ollamaServer(t, tier)
			var stdout bytes.Buffer
			if err := run(strings.NewReader("some task"), &stdout, classifierFor(ts), false); err != nil {
				t.Fatalf("run error: %v", err)
			}
			task := parseTask(t, stdout.String())
			if task.Tier != tier {
				t.Errorf("tier = %q, want %q", task.Tier, tier)
			}
		})
	}
}

func TestLoadConfigDefaultsWhenNoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // empty home — no config file
	cfg := loadConfig()
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q, want default", cfg.OllamaURL)
	}
}

func TestLoadConfigReadsOllamaURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(dir+"/.sidings", 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir+"/.sidings/classify.yaml", []byte("ollama_url: http://custom:9999\n"), 0600)

	cfg := loadConfig()
	if cfg.OllamaURL != "http://custom:9999" {
		t.Errorf("OllamaURL = %q, want http://custom:9999", cfg.OllamaURL)
	}
}
