package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/pipe"
)

// --- helpers ---

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

// --- buildArgs ---

func TestBuildArgsIncludesModelFlagWhenModelSet(t *testing.T) {
	task := pipe.Task{
		Content: "rename foo to bar",
		Tier:    "medium",
		Route:   &pipe.Route{Model: "qwen3-coder"},
	}
	args := buildArgs(task)
	modelIdx := -1
	for i, a := range args {
		if a == "--model" {
			modelIdx = i
			break
		}
	}
	if modelIdx == -1 {
		t.Fatalf("--model flag not found in args: %v", args)
	}
	if modelIdx+1 >= len(args) || args[modelIdx+1] != "qwen3-coder" {
		t.Errorf("--model value = %q, want qwen3-coder", args[modelIdx+1])
	}
}

func TestBuildArgsNoModelFlagWhenModelEmpty(t *testing.T) {
	task := pipe.Task{
		Content: "some task",
		Tier:    "exceptional",
		Route:   &pipe.Route{Model: ""},
	}
	args := buildArgs(task)
	for _, a := range args {
		if a == "--model" {
			t.Errorf("unexpected --model flag in args: %v", args)
		}
	}
}

func TestBuildArgsNoModelFlagWhenRouteNil(t *testing.T) {
	task := pipe.Task{
		Content: "some task",
		Tier:    "exceptional",
		Route:   nil,
	}
	args := buildArgs(task)
	for _, a := range args {
		if a == "--model" {
			t.Errorf("unexpected --model flag when route is nil: %v", args)
		}
	}
}

// --- buildEnv ---

func TestBuildEnvLocalTierSetsAnthropicEnv(t *testing.T) {
	for _, tier := range []string{"simple", "medium", "complex"} {
		tier := tier
		t.Run(tier, func(t *testing.T) {
			env := buildEnv(tier, "http://localhost:11434")
			if env == nil {
				t.Fatal("expected non-nil env for local tier")
			}
			var hasBaseURL, hasAuthToken bool
			for _, e := range env {
				if strings.HasPrefix(e, "ANTHROPIC_BASE_URL=") {
					hasBaseURL = true
				}
				if e == "ANTHROPIC_AUTH_TOKEN=ollama" {
					hasAuthToken = true
				}
			}
			if !hasBaseURL {
				t.Error("ANTHROPIC_BASE_URL not set in env")
			}
			if !hasAuthToken {
				t.Error("ANTHROPIC_AUTH_TOKEN not set in env")
			}
		})
	}
}

func TestBuildEnvExceptionalTierReturnsNil(t *testing.T) {
	env := buildEnv("exceptional", "http://localhost:11434")
	if env != nil {
		t.Errorf("expected nil env for exceptional tier, got %d entries", len(env))
	}
}

func TestBuildEnvLocalTierBaseURLMatchesOllamaURL(t *testing.T) {
	env := buildEnv("simple", "http://custom:8080")
	for _, e := range env {
		if e == "ANTHROPIC_BASE_URL=http://custom:8080" {
			return
		}
	}
	t.Errorf("ANTHROPIC_BASE_URL not set to custom URL in: %v", env)
}

func TestBuildEnvLocalTierDefaultsOllamaURLWhenEmpty(t *testing.T) {
	env := buildEnv("medium", "")
	for _, e := range env {
		if e == "ANTHROPIC_BASE_URL=http://localhost:11434" {
			return
		}
	}
	t.Errorf("ANTHROPIC_BASE_URL not defaulted correctly in: %v", env)
}

// --- gitModifiedFiles ---

func TestGitModifiedFilesCleanRepo(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available:", err)
	}

	files, err := gitModifiedFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map for clean repo, got %v", files)
	}
}

func TestGitModifiedFilesModifiedFile(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skip("git not available:", err)
	}
	initGitRepo(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := gitModifiedFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["foo.go"]; !ok {
		t.Errorf("expected foo.go in modified files, got %v", files)
	}
}

func TestGitModifiedFilesNotGitRepo(t *testing.T) {
	dir := t.TempDir() // plain dir, no git init

	files, err := gitModifiedFiles(dir)
	if err != nil {
		t.Errorf("expected no error for non-git dir, got: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map for non-git dir, got %v", files)
	}
}

// --- diffFiles ---

func TestDiffFilesNewFile(t *testing.T) {
	before := map[string]struct{}{}
	after := map[string]struct{}{"foo.go": {}}
	result := diffFiles(before, after)
	if len(result) != 1 || result[0] != "foo.go" {
		t.Errorf("expected [foo.go], got %v", result)
	}
}

func TestDiffFilesModifiedFile(t *testing.T) {
	// A file not in before but showing as modified in after (e.g. Claude changed it).
	before := map[string]struct{}{}
	after := map[string]struct{}{"bar.go": {}}
	result := diffFiles(before, after)
	if len(result) != 1 || result[0] != "bar.go" {
		t.Errorf("expected [bar.go], got %v", result)
	}
}

func TestDiffFilesNoChanges(t *testing.T) {
	before := map[string]struct{}{"existing.go": {}}
	after := map[string]struct{}{"existing.go": {}}
	result := diffFiles(before, after)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestDiffFilesSortedAlphabetically(t *testing.T) {
	before := map[string]struct{}{}
	after := map[string]struct{}{
		"z.go": {},
		"a.go": {},
		"m.go": {},
	}
	result := diffFiles(before, after)
	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %v", result)
	}
	if result[0] != "a.go" || result[1] != "m.go" || result[2] != "z.go" {
		t.Errorf("expected alphabetical order, got %v", result)
	}
}

// --- ensureClaudeSettings ---

func TestEnsureClaudeSettingsMissingFile(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudeSettings(dir, false); err != nil {
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

func TestEnsureClaudeSettingsMergesSandbox(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"custom":"value","permissions":{"defaultMode":"acceptEdits"}}`)

	if err := ensureClaudeSettings(dir, false); err != nil {
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

func TestEnsureClaudeSettingsNoop(t *testing.T) {
	dir := t.TempDir()
	original := `{"permissions":{"defaultMode":"acceptEdits"},"sandbox":{"enabled":true,"autoAllowBashIfSandboxed":true}}`
	p := writeSettings(t, dir, original)

	statBefore, _ := os.Stat(p)

	if err := ensureClaudeSettings(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statAfter, _ := os.Stat(p)
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Error("file was modified when it should have been a no-op")
	}
}

func TestEnsureClaudeSettingsSandboxDisabledClash(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"sandbox":{"enabled":false}}`)

	err := ensureClaudeSettings(dir, false)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

func TestEnsureClaudeSettingsDisableBypassClash(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"permissions":{"disableBypassPermissionsMode":"disable"}}`)

	err := ensureClaudeSettings(dir, false)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

func TestEnsureClaudeSettingsMultipleClashes(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"sandbox":{"enabled":false},"permissions":{"disableBypassPermissionsMode":"disable"}}`)

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

	err := ensureClaudeSettings(dir, false)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected ErrSettingsConflict, got %v", err)
	}
}

func TestEnsureClaudeSettingsDefaultModePreserved(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"permissions":{"defaultMode":"bypassPermissions"}}`)

	if err := ensureClaudeSettings(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := readSettings(t, dir)
	perms := m["permissions"].(map[string]interface{})
	if perms["defaultMode"] != "bypassPermissions" {
		t.Errorf("defaultMode was overwritten; got %q", perms["defaultMode"])
	}
}

func TestEnsureClaudeSettingsNoRelevantKeys(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{"someOtherKey":42}`)

	if err := ensureClaudeSettings(dir, false); err != nil {
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

func TestEnsureClaudeSettingsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeSettings(t, dir, `{not valid json`)

	err := ensureClaudeSettings(dir, false)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if errors.Is(err, ErrSettingsConflict) {
		t.Fatal("expected a parse error, not ErrSettingsConflict")
	}

	data, _ := os.ReadFile(p)
	if !bytes.Equal(data, []byte(`{not valid json`)) {
		t.Errorf("file was overwritten; got %q", data)
	}
}

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
