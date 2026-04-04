package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func fakeRoot() *cobra.Command {
	return &cobra.Command{Use: "sidings"}
}

func TestInstallBashCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := installBash(fakeRoot()); err != nil {
		t.Fatalf("installBash error: %v", err)
	}

	path := filepath.Join(dir, ".bash_completion.d", "sidings")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("completion file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("completion file is empty")
	}
}

func TestInstallBashFileContainsSidingsCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	root := fakeRoot()
	root.AddCommand(&cobra.Command{Use: "classify", Short: "Classify"})

	if err := installBash(root); err != nil {
		t.Fatalf("installBash error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".bash_completion.d", "sidings"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sidings") {
		t.Error("completion script does not mention 'sidings'")
	}
}

func TestInstallZshCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := installZsh(fakeRoot()); err != nil {
		t.Fatalf("installZsh error: %v", err)
	}

	// Should create either ~/.zfunc/_sidings or ~/.zsh/completions/_sidings.
	candidates := []string{
		filepath.Join(dir, ".zfunc", "_sidings"),
		filepath.Join(dir, ".zsh", "completions", "_sidings"),
	}
	found := false
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.Size() > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("zsh completion file not found at any expected path: %v", candidates)
	}
}

func TestInstallZshPrefersExistingZfunc(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Pre-create ~/.zfunc so installZsh prefers it.
	zfunc := filepath.Join(dir, ".zfunc")
	if err := os.MkdirAll(zfunc, 0755); err != nil {
		t.Fatal(err)
	}

	if err := installZsh(fakeRoot()); err != nil {
		t.Fatalf("installZsh error: %v", err)
	}

	path := filepath.Join(zfunc, "_sidings")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected completion file at %s: %v", path, err)
	}
}

func TestInstallFishCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := installFish(fakeRoot()); err != nil {
		t.Fatalf("installFish error: %v", err)
	}

	path := filepath.Join(dir, ".config", "fish", "completions", "sidings.fish")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("fish completion file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("fish completion file is empty")
	}
}

func TestInstallCompletionUnknownShellReturnsError(t *testing.T) {
	t.Setenv("SHELL", "/bin/unknownshell")
	root := fakeRoot()
	err := installCompletion(root)
	if err == nil {
		t.Fatal("expected error for unknown shell, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Errorf("error = %v, want unsupported shell error", err)
	}
}
