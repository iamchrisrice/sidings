package executor_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/executor"
	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// initGitRepo creates a git repo in dir with an empty initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}
}

// ollamaServer starts a test server that returns the given text as a single
// Ollama streaming response (one JSON line, done=true).
func ollamaServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(map[string]interface{}{
			"response": response,
			"done":     true,
		})
		fmt.Fprintf(w, "%s\n", b)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func testTask(ts *httptest.Server) pipe.Task {
	url := ""
	if ts != nil {
		url = ts.URL
	}
	_ = url
	return pipe.Task{
		TaskID:  "test-123",
		Content: "fix the bug",
		Tier:    "simple",
		Route:   &pipe.Route{Backend: "ollama", Model: "qwen3.5:0.8b"},
	}
}

func TestSuccessfulResponseWithFileBlocksWritesFilesAndPopulatesFilesWritten(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	ts := ollamaServer(t, "<<<< hello.go\npackage main\n>>>>")
	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: ts.URL,
		Yes:       true,
		WorkDir:   dir,
	})

	result, err := ex.Execute(testTask(ts))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.FilesWritten) == 0 {
		t.Fatal("expected FilesWritten to be populated")
	}
	if result.FilesWritten[0] != "hello.go" {
		t.Errorf("FilesWritten[0] = %q, want hello.go", result.FilesWritten[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "hello.go")); err != nil {
		t.Error("hello.go was not written to disk")
	}
}

func TestSuccessfulResponseWithoutFileBlocksPopulatesOutput(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	ts := ollamaServer(t, "The issue is a race condition in the goroutine pool.")
	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: ts.URL,
		Yes:       true,
		WorkDir:   dir,
	})

	result, err := ex.Execute(testTask(ts))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output == "" {
		t.Error("expected Output to be populated for a plain text response")
	}
	if len(result.FilesWritten) != 0 {
		t.Errorf("expected no FilesWritten for plain text response, got %v", result.FilesWritten)
	}
}

func TestOllamaUnavailableReturnsErrorWithNoPanic(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: "http://localhost:1", // nothing listening here
		Yes:       true,
		WorkDir:   dir,
	})

	_, err := ex.Execute(testTask(nil))
	if err == nil {
		t.Error("expected an error when Ollama is unavailable, got nil")
	}
}

func TestResponseOutsideGitRepoPathIsRefused(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Model attempts to write a file using an absolute path outside the repo.
	outsidePath := filepath.Join(t.TempDir(), "evil.go")
	ts := ollamaServer(t, fmt.Sprintf("<<<< %s\npackage evil\n>>>>", outsidePath))
	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: ts.URL,
		Yes:       true,
		WorkDir:   dir,
	})

	_, err := ex.Execute(testTask(ts))
	if err == nil {
		t.Error("expected an error when writing outside the git repo")
	}
	if _, statErr := os.Stat(outsidePath); statErr == nil {
		os.Remove(outsidePath)
		t.Error("file was written outside the git repo — should have been refused")
	}
}

func TestYesFlagSkipsConfirmationPrompt(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	ts := ollamaServer(t, "<<<< confirmed.go\npackage main\n>>>>")
	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: ts.URL,
		Yes:       true,
		WorkDir:   dir,
		Stdin:     strings.NewReader(""), // empty — would block if confirmation were requested
	})

	result, err := ex.Execute(testTask(ts))
	if err != nil {
		t.Fatalf("Execute with Yes=true: %v", err)
	}
	if len(result.FilesWritten) == 0 {
		t.Error("expected file to be written when Yes=true")
	}
}

func TestOllamaHTTPErrorReturnsError(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	ex := executor.NewOllama(executor.OllamaConfig{
		OllamaURL: ts.URL,
		Yes:       true,
		WorkDir:   dir,
	})

	_, err := ex.Execute(testTask(ts))
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}
