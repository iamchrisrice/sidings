package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureClaudePermissionsCreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudePermissions(dir); err != nil {
		t.Fatalf("ensureClaudePermissions: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf(".claude/settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if _, ok := settings["allowedDirectories"]; !ok {
		t.Error("expected allowedDirectories key in settings.json")
	}
}

func TestEnsureClaudePermissionsCreatesNestedDirectory(t *testing.T) {
	dir := t.TempDir()
	// .claude/ does not exist — MkdirAll must create it.

	if err := ensureClaudePermissions(dir); err != nil {
		t.Fatalf("ensureClaudePermissions: %v", err)
	}

	dotClaude := filepath.Join(dir, ".claude")
	fi, err := os.Stat(dotClaude)
	if err != nil {
		t.Fatalf(".claude directory not created: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".claude is not a directory")
	}
}

func TestEnsureClaudePermissionsNoopWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	dotClaude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(dotClaude, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dotClaude, "settings.json")
	original := []byte(`{"custom":"value"}`)
	if err := os.WriteFile(settingsPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureClaudePermissions(dir); err != nil {
		t.Fatalf("ensureClaudePermissions: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	if string(data) != string(original) {
		t.Errorf("existing file was modified: got %q, want %q", data, original)
	}
}

func TestEnsureClaudePermissionsIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudePermissions(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ensureClaudePermissions(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
