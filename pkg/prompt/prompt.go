// Package prompt assembles prompts for LLM execution backends.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the information needed to build a prompt.
type Config struct {
	Dir   string // working directory (empty = os.Getwd())
	Task  string
	Tier  string
	Model string
}

// Build assembles the full prompt string for a task.
// Simple tier → bare task content.
// All other tiers → structured context preamble + file-block output instructions.
func Build(cfg Config) string {
	if cfg.Tier == "simple" {
		return cfg.Task
	}

	dir := cfg.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	lang := detectLanguage(dir)
	ctx := GatherContext(dir, cfg.Task, cfg.Model)

	return fmt.Sprintf(`You are a coding assistant. You MUST respond using <file> blocks for any file changes.

<file path="path/to/file.go">
complete file contents here
</file>

Do not use markdown code blocks. Do not explain outside file blocks.
If no file changes are needed, respond with plain text only.

---

Project context (%s project, %s):
%s

Task: %s

For each file you want to create or modify, use this format exactly:

<file path="path/to/file.go">
complete file contents here
</file>

Rules:
- One <file> block per file
- Include complete file contents — not a diff, not a snippet
- Do not include any explanation or markdown outside the file blocks
- If no file changes are needed, respond with plain text only`,
		lang, dir, ctx, cfg.Task)
}

// detectLanguage returns the primary language of the project in dir.
// Checks for well-known config files rather than scanning extensions.
func detectLanguage(dir string) string {
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"tsconfig.json", "TypeScript"},
		{"package.json", "JavaScript"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
		{"Gemfile", "Ruby"},
		{"pom.xml", "Java"},
		{"build.gradle", "Java"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.lang
		}
	}
	return "unknown"
}
