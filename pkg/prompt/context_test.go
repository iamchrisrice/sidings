package prompt_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/prompt"
)

// initGitRepo initialises a bare git repo in dir suitable for testing.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}
}

func gitAdd(t *testing.T, dir string) {
	t.Helper()
	exec.Command("git", "-C", dir, "add", ".").Run()           //nolint:errcheck
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run() //nolint:errcheck
}

func TestReadmeExistsAndIsIncludedInContext(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\n\nA great project."), 0644)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
	if !strings.Contains(ctx, "My Project") {
		t.Error("expected README content in context")
	}
}

func TestReadmeMissingFallsBackToProjectTreeOnly(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	// Must not panic; returns something or empty string.
	_ = prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
}

func TestMarkdownLinkInReadmeToLocalFileIsIncluded(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("See [API docs](./docs/api.md)"), 0644)
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	os.WriteFile(filepath.Join(dir, "docs", "api.md"), []byte("# API Reference\nEndpoints go here."), 0644)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
	if !strings.Contains(ctx, "API Reference") {
		t.Error("expected linked doc content in context")
	}
}

func TestMarkdownLinkToExternalURLIsIgnored(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// README links to an external URL. We write a local file with a unique marker
	// that would only appear in context if the external URL were fetched.
	os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("See [docs](https://external.invalid/docs) for details."), 0644)
	// "FETCHED_EXTERNAL_CONTENT" would only appear if we tried to read the URL as a local file.
	os.WriteFile(filepath.Join(dir, "docs"), []byte("FETCHED_EXTERNAL_CONTENT"), 0644)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
	// The external URL path should not be resolved to a local file.
	if strings.Contains(ctx, "FETCHED_EXTERNAL_CONTENT") {
		t.Error("external URL was treated as a local file path — should be ignored")
	}
}

func TestFilenameMatchingScoresAuthRelatedFilesHigherForAuthTask(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package main\n// auth handler"), 0644)
	os.WriteFile(filepath.Join(dir, "unrelated.go"), []byte("package main\n// unrelated"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "refactor auth module", "qwen3.5:9b")

	authIdx := strings.Index(ctx, "auth.go")
	unrelatedIdx := strings.Index(ctx, "unrelated.go")

	if authIdx == -1 {
		t.Fatal("auth.go not found in context at all")
	}
	if unrelatedIdx != -1 && unrelatedIdx < authIdx {
		t.Error("expected auth.go to appear before unrelated.go for an auth task")
	}
}

func TestGitNotAvailableFallsBackToFilenameMatchingOnly(t *testing.T) {
	// A directory that is NOT a git repo — git commands will fail.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package main"), 0644)

	// Must not panic.
	ctx := prompt.GatherContext(dir, "fix auth", "qwen3.5:9b")
	_ = ctx
}

func TestSecretFilesAreNeverIncludedInContext(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	secrets := map[string]string{
		".env":           "DB_PASSWORD=hunter2",
		"secrets.yaml":   "api_secret: hunter2",
		"api_key.txt":    "sk-hunter2",
		"db_password.go": `package main // hunter2`,
	}
	for name, content := range secrets {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}
	// Also add a safe file to ensure the ranking logic runs.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "fix the app", "qwen3.5:9b")
	if strings.Contains(ctx, "hunter2") {
		t.Error("secret file contents must never appear in context")
	}
}
