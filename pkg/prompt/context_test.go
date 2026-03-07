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
	exec.Command("git", "-C", dir, "add", ".").Run()            //nolint:errcheck
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run() //nolint:errcheck
}

// TestDirectoryWithReadme — tree, README content, and source files all present.
func TestDirectoryWithReadme(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\n\nA great project."), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")

	if !strings.Contains(ctx, "Project structure") {
		t.Error("expected project tree in context")
	}
	if !strings.Contains(ctx, "My Project") {
		t.Error("expected README content in context")
	}
	if !strings.Contains(ctx, "main.go") {
		t.Error("expected source file in context")
	}
}

// TestDirectoryWithoutReadme — tree and source files included, no error, no README section.
func TestDirectoryWithoutReadme(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")

	if !strings.Contains(ctx, "Project structure") {
		t.Error("expected project tree even without README")
	}
	if !strings.Contains(ctx, "main.go") {
		t.Error("expected source file even without README")
	}
}

// TestDirectoryWithoutReadmeHasNoREADMEHeader — README section header absent when file missing.
func TestDirectoryWithoutReadmeHasNoREADMEHeader(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")

	if strings.Contains(ctx, "### README") {
		t.Error("README section header must not appear when README.md is missing")
	}
}

// TestEmptyDirectory — returns tree only (or empty string), no panic.
func TestEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	// No files, no git repo.
	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
	_ = ctx // must not panic; empty or tree-only are both acceptable
}

// TestMarkdownLinkInReadmeToLocalFileIsIncluded — linked local docs included.
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

// TestMarkdownLinkToExternalURLIsIgnored — external URLs not resolved as local paths.
func TestMarkdownLinkToExternalURLIsIgnored(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("See [docs](https://external.invalid/docs) for details."), 0644)
	// This file would only appear in context if the URL was treated as a local path.
	os.WriteFile(filepath.Join(dir, "docs"), []byte("FETCHED_EXTERNAL_CONTENT"), 0644)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")
	if strings.Contains(ctx, "FETCHED_EXTERNAL_CONTENT") {
		t.Error("external URL was treated as a local file path — should be ignored")
	}
}

// TestFilenameMatchingScoresAuthRelatedFilesHigher — auth.go ranked before unrelated.go.
func TestFilenameMatchingScoresAuthRelatedFilesHigher(t *testing.T) {
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

// TestGitUnavailableFallsBackToFilenameMatchingStillIncludesTree — no git, tree still present.
func TestGitUnavailableFallsBackToFilenameMatchingStillIncludesTree(t *testing.T) {
	// A directory that is NOT a git repo — git commands will fail.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "auth.go"), []byte("package main"), 0644)

	ctx := prompt.GatherContext(dir, "fix auth", "qwen3.5:9b")

	if !strings.Contains(ctx, "Project structure") {
		t.Error("expected project tree even when git is unavailable")
	}
	if !strings.Contains(ctx, "auth.go") {
		t.Error("expected auth.go in context even when git is unavailable")
	}
}

// TestSecretFilesAreNeverIncluded — secret file contents must not appear.
func TestSecretFilesAreNeverIncluded(t *testing.T) {
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
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	gitAdd(t, dir)

	ctx := prompt.GatherContext(dir, "fix the app", "qwen3.5:9b")
	if strings.Contains(ctx, "hunter2") {
		t.Error("secret file contents must never appear in context")
	}
}

// TestDeeplyNestedDirectoryTreeCappedAt3Levels — tree section does not exceed 3 levels.
func TestDeeplyNestedDirectoryTreeCappedAt3Levels(t *testing.T) {
	dir := t.TempDir()
	// Create a 4-level deep directory: a/b/c/d/deep.go
	deep := filepath.Join(dir, "a", "b", "c", "d")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "deep.go"), []byte("package deep"), 0644)
	// Also create a file at level 1 to confirm tree works at all.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	ctx := prompt.GatherContext(dir, "fix the bug", "qwen3.5:9b")

	// Extract the project structure section only (up to the next section or end).
	treeStart := strings.Index(ctx, "### Project structure")
	if treeStart == -1 {
		t.Fatal("expected project structure section in context")
	}
	// Find the closing ``` of the code block.
	blockStart := strings.Index(ctx[treeStart:], "```\n")
	if blockStart == -1 {
		t.Fatal("project structure section has no code block")
	}
	blockContentStart := treeStart + blockStart + 4 // skip "```\n"
	blockEnd := strings.Index(ctx[blockContentStart:], "\n```")
	var treeSection string
	if blockEnd == -1 {
		treeSection = ctx[blockContentStart:]
	} else {
		treeSection = ctx[blockContentStart : blockContentStart+blockEnd]
	}

	if !strings.Contains(treeSection, "main.go") {
		t.Error("expected top-level file in project structure")
	}
	// deep.go is 4 levels in — must not appear in the tree section.
	if strings.Contains(treeSection, "deep.go") {
		t.Error("tree must be capped at 3 levels deep; deep.go must not appear in tree section")
	}
}
