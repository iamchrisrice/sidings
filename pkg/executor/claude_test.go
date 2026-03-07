package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSettings(t *testing.T, dir string, content string) string {
	t.Helper()
	dotClaude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(dotClaude, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dotClaude, "settings.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readSettings(t *testing.T, dir string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	return m
}

// TestEnsureClaudeSettingsMissingFile — file missing → created with sandbox enabled.
func TestEnsureClaudeSettingsMissingFile(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudeSettings(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := readSettings(t, dir)

	perms, ok := m["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("expected permissions object")
	}
	if perms["defaultMode"] != "acceptEdits" {
		t.Errorf("permissions.defaultMode = %q, want acceptEdits", perms["defaultMode"])
	}

	sandbox, ok := m["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sandbox object")
	}
	if sandbox["enabled"] != true {
		t.Errorf("sandbox.enabled = %v, want true", sandbox["enabled"])
	}
	if sandbox["autoAllowBashIfSandboxed"] != true {
		t.Errorf("sandbox.autoAllowBashIfSandboxed = %v, want true", sandbox["autoAllowBashIfSandboxed"])
	}
}

// TestEnsureClaudeSettingsMergesSandbox — file exists without sandbox block → sandbox block added, rest untouched.
func TestEnsureClaudeSettingsMergesSandbox(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"custom":"value","permissions":{"defaultMode":"acceptEdits"}}`)

	if err := ensureClaudeSettings(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := readSettings(t, dir)
	if m["custom"] != "value" {
		t.Errorf("custom key lost; got %v", m["custom"])
	}
	sandbox, ok := m["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sandbox object after merge")
	}
	if sandbox["enabled"] != true {
		t.Errorf("sandbox.enabled = %v, want true", sandbox["enabled"])
	}
}

// TestEnsureClaudeSettingsNoop — file exists with sandbox already enabled → no-op, file unchanged.
func TestEnsureClaudeSettingsNoop(t *testing.T) {
	dir := t.TempDir()
	original := `{"permissions":{"defaultMode":"acceptEdits"},"sandbox":{"enabled":true,"autoAllowBashIfSandboxed":true}}`
	p := writeSettings(t, dir, original)

	statBefore, _ := os.Stat(p)

	if err := ensureClaudeSettings(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statAfter, _ := os.Stat(p)
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Error("file was modified when it should have been a no-op")
	}
}

// TestEnsureClaudeSettingsSandboxDisabledClash — sandbox.enabled false → ErrSettingsConflict.
func TestEnsureClaudeSettingsSandboxDisabledClash(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"sandbox":{"enabled":false}}`)

	err := ensureClaudeSettings(dir)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

// TestEnsureClaudeSettingsDisableBypassClash — disableBypassPermissionsMode "disable" → ErrSettingsConflict.
func TestEnsureClaudeSettingsDisableBypassClash(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"permissions":{"disableBypassPermissionsMode":"disable"}}`)

	err := ensureClaudeSettings(dir)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

// TestEnsureClaudeSettingsMultipleClashes — multiple clashes → all reported before exiting.
func TestEnsureClaudeSettingsMultipleClashes(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"sandbox":{"enabled":false},"permissions":{"disableBypassPermissionsMode":"disable"}}`)

	// Capture stderr by temporarily redirecting — we use detectClashes directly
	// since stderr capture from ensureClaudeSettings requires process-level tricks.
	// Instead, test detectClashes directly for multi-clash reporting.
	settings := map[string]interface{}{
		"sandbox": map[string]interface{}{"enabled": false},
		"permissions": map[string]interface{}{
			"disableBypassPermissionsMode": "disable",
		},
	}
	clashes := detectClashes(settings)
	if len(clashes) != 2 {
		t.Errorf("expected 2 clashes, got %d: %v", len(clashes), clashes)
	}

	// Also confirm ensureClaudeSettings returns ErrSettingsConflict.
	err := ensureClaudeSettings(dir)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

// TestEnsureClaudeSettingsDefaultModePreserved — existing permissions.defaultMode left unchanged.
func TestEnsureClaudeSettingsDefaultModePreserved(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"permissions":{"defaultMode":"bypassPermissions"}}`)

	if err := ensureClaudeSettings(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := readSettings(t, dir)
	perms := m["permissions"].(map[string]interface{})
	if perms["defaultMode"] != "bypassPermissions" {
		t.Errorf("defaultMode was overwritten; got %q", perms["defaultMode"])
	}
}

// TestEnsureClaudeSettingsNoRelevantKeys — valid JSON but no relevant keys → all sidings keys merged in.
func TestEnsureClaudeSettingsNoRelevantKeys(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"someOtherKey":42}`)

	if err := ensureClaudeSettings(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := readSettings(t, dir)
	if m["someOtherKey"] != float64(42) {
		t.Errorf("someOtherKey lost; got %v", m["someOtherKey"])
	}
	sandbox, ok := m["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sandbox object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled not set")
	}
	perms, ok := m["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("expected permissions object")
	}
	if perms["defaultMode"] != "acceptEdits" {
		t.Errorf("defaultMode = %q, want acceptEdits", perms["defaultMode"])
	}
}

// TestEnsureClaudeSettingsMalformedJSON — malformed JSON → error, file not overwritten.
func TestEnsureClaudeSettingsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, `{not valid json`)

	err := ensureClaudeSettings(dir)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if errors.Is(err, ErrSettingsConflict) {
		t.Fatal("expected a parse error, not ErrSettingsConflict")
	}

	// File must not have been overwritten.
	data, _ := os.ReadFile(p)
	if !bytes.Equal(data, []byte(`{not valid json`)) {
		t.Errorf("file was overwritten; got %q", data)
	}
}

// TestDetectClashesNoClash — valid settings → no clashes returned.
func TestDetectClashesNoClash(t *testing.T) {
	settings := map[string]interface{}{
		"sandbox": map[string]interface{}{"enabled": true},
		"permissions": map[string]interface{}{
			"defaultMode": "acceptEdits",
		},
	}
	if clashes := detectClashes(settings); len(clashes) != 0 {
		t.Errorf("expected no clashes, got %v", clashes)
	}
}

// TestMergeSettingsIdempotent — calling mergeSettings twice reports changed=false on second call.
func TestMergeSettingsIdempotent(t *testing.T) {
	settings := map[string]interface{}{}
	merged, changed := mergeSettings(settings)
	if !changed {
		t.Error("expected changed=true on first merge")
	}
	_, changed2 := mergeSettings(merged)
	if changed2 {
		t.Error("expected changed=false on second merge (idempotent)")
	}
}

// TestDetectClashesMessages — clash messages mention the offending key.
func TestDetectClashesMessages(t *testing.T) {
	settings := map[string]interface{}{
		"sandbox":     map[string]interface{}{"enabled": false},
		"permissions": map[string]interface{}{"disableBypassPermissionsMode": "disable"},
	}
	clashes := detectClashes(settings)
	for _, want := range []string{"sandbox.enabled", "disableBypassPermissionsMode"} {
		found := false
		for _, c := range clashes {
			if strings.Contains(c, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("clash message for %q not found in %v", want, clashes)
		}
	}
}
