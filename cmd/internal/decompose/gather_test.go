package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
}

func TestGatherContextFromEmptyDirIsEmpty(t *testing.T) {
	dir := t.TempDir()
	ctx := gatherContextFrom(dir)
	// No git repo, no files — should return empty string.
	if ctx != "" {
		t.Errorf("expected empty context for empty dir, got:\n%s", ctx)
	}
}

func TestGatherContextFromIncludesGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := gatherContextFrom(dir)
	if !strings.Contains(ctx, "go.mod") {
		t.Error("expected context to include go.mod")
	}
	if !strings.Contains(ctx, "example.com/test") {
		t.Error("expected context to include go.mod content")
	}
}

func TestGatherContextFromIncludesREADME(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Project\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := gatherContextFrom(dir)
	if !strings.Contains(ctx, "README.md") {
		t.Error("expected context to include README.md")
	}
	if !strings.Contains(ctx, "My Project") {
		t.Error("expected context to include README.md content")
	}
}

func TestGatherContextFromIncludesGoFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := gatherContextFrom(dir)
	if !strings.Contains(ctx, "main.go") {
		t.Error("expected context to include main.go")
	}
	if !strings.Contains(ctx, "package main") {
		t.Error("expected context to include main.go content")
	}
}

func TestGatherContextFromIncludesGitFiles(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create and stage a file so git ls-files lists it.
	goFile := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(goFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "add", "foo.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	ctx := gatherContextFrom(dir)
	if !strings.Contains(ctx, "Tracked files:") {
		t.Error("expected 'Tracked files:' section in context")
	}
	if !strings.Contains(ctx, "foo.go") {
		t.Error("expected foo.go to appear in tracked files")
	}
}

func TestGatherContextFromLimitsGoFilesTo5(t *testing.T) {
	dir := t.TempDir()
	// Create 7 .go files.
	for i := 0; i < 7; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1)+".go")
		if err := os.WriteFile(name, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := gatherContextFrom(dir)
	// Count how many "--- *.go ---" headers appear.
	count := strings.Count(ctx, "--- ")
	if count > 5 {
		t.Errorf("expected at most 5 .go file sections, got %d", count)
	}
}

func TestGatherContextFromNonGitDirOmitsTrackedFiles(t *testing.T) {
	dir := t.TempDir()
	// No git init — git ls-files will fail silently.
	ctx := gatherContextFrom(dir)
	if strings.Contains(ctx, "Tracked files:") {
		t.Error("expected no 'Tracked files:' for non-git directory")
	}
}
