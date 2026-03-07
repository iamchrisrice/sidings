package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GatherContext builds project context for the prompt.
// Reads directory tree, README (if present), linked docs (if README present),
// and ranked source files. model is used to determine the token budget.
func GatherContext(dir string, task string, model string) string {
	budgetChars := tokenBudgetChars(model)
	var parts []string

	// 1. Directory tree — always.
	if tree := buildTree(dir); tree != "" {
		parts = append(parts, "### Project structure\n\n```\n"+tree+"\n```")
	}

	// 2. README — optional, silently skipped if missing.
	readmeContent := ""
	if content, err := os.ReadFile(filepath.Join(dir, "README.md")); err == nil {
		readmeContent = string(content)
		parts = append(parts, "### README\n\n"+readmeContent)
		budgetChars -= len(readmeContent)
	}

	// 3. Linked local docs — only when README exists.
	if readmeContent != "" {
		for _, linked := range parseMarkdownLinks(readmeContent, dir) {
			if isSecret(linked) {
				continue
			}
			content := readTextFile(linked)
			if content == "" || budgetChars <= 0 {
				continue
			}
			rel, _ := filepath.Rel(dir, linked)
			entry := fmt.Sprintf("### %s\n\n%s", rel, content)
			parts = append(parts, entry)
			budgetChars -= len(entry)
		}
	}

	// 4. Relevant source files — always.
	ranked := rankFiles(dir, task)
	var fileBlocks []string
	for _, f := range ranked {
		if budgetChars <= 0 {
			break
		}
		content := readTextFile(filepath.Join(dir, f))
		if content == "" {
			continue
		}
		block := fmt.Sprintf("// %s\n%s", f, content)
		if len(block) > budgetChars {
			break
		}
		fileBlocks = append(fileBlocks, block)
		budgetChars -= len(block)
	}
	if len(fileBlocks) > 0 {
		parts = append(parts, "### Source files\n\n"+strings.Join(fileBlocks, "\n\n"))
	}

	return strings.Join(parts, "\n\n")
}

// tokenBudgetChars returns the character budget for source files based on model name.
// Rough estimate: 4 chars per token.
func tokenBudgetChars(model string) int {
	switch {
	case strings.Contains(model, "32b"):
		return 29000 * 4
	case strings.Contains(model, "9b"):
		return 17000 * 4
	default:
		return 17000 * 4
	}
}

var mdLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// parseMarkdownLinks returns absolute paths to local files linked from text.
// External URLs (http/https) are ignored.
func parseMarkdownLinks(text, dir string) []string {
	var paths []string
	for _, m := range mdLinkRe.FindAllStringSubmatch(text, -1) {
		href := m[1]
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			continue
		}
		href = strings.TrimPrefix(href, "./")
		paths = append(paths, filepath.Join(dir, href))
	}
	return paths
}

var excludeDirs = []string{"vendor", "node_modules", ".git"}
var excludeExts = []string{".pb.go", ".sum", ".lock"}

// isSecret reports whether a file path looks like it might contain secrets.
func isSecret(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(lower, ".env") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "key")
}

// isExcluded reports whether a file should never be included in context.
func isExcluded(path string) bool {
	if isSecret(path) {
		return true
	}
	for _, dir := range excludeDirs {
		if strings.HasPrefix(path, dir+"/") || strings.Contains(path, "/"+dir+"/") {
			return true
		}
	}
	for _, ext := range excludeExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// buildTree returns an indented directory tree for dir, up to 3 levels deep.
// Skips vendor/, node_modules/, .git/, and *.pb.go files.
// Never returns empty just because git is unavailable or README is absent.
func buildTree(dir string) string {
	var lines []string
	walkTree(dir, dir, 0, &lines)
	return strings.Join(lines, "\n")
}

func walkTree(root, current string, depth int, lines *[]string) {
	if depth >= 3 {
		return
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		indent := strings.Repeat("  ", depth)
		if e.IsDir() {
			skip := false
			for _, ex := range excludeDirs {
				if name == ex {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			*lines = append(*lines, indent+name+"/")
			walkTree(root, filepath.Join(current, name), depth+1, lines)
		} else {
			if strings.HasSuffix(name, ".pb.go") {
				continue
			}
			*lines = append(*lines, indent+name)
		}
	}
}

type scoredFile struct {
	path  string
	score int
}

// rankFiles returns source files from dir sorted by relevance to task.
func rankFiles(dir string, task string) []string {
	candidates := listSourceFiles(dir)
	if len(candidates) == 0 {
		return nil
	}

	words := taskWords(task)

	grepSet := toSet(gitGrepFiles(dir, words))
	recentSet := toSet(gitRecentFiles(dir))

	scored := make([]scoredFile, 0, len(candidates))
	for _, f := range candidates {
		if isExcluded(f) {
			continue
		}
		score := 0
		lf := strings.ToLower(f)
		for _, w := range words {
			if strings.Contains(lf, w) {
				score += 3
			}
		}
		if grepSet[f] {
			score += 2
		}
		if recentSet[f] {
			score += 1
		}
		scored = append(scored, scoredFile{path: f, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	out := make([]string, len(scored))
	for i, sf := range scored {
		out[i] = sf.path
	}
	return out
}

var commonWords = map[string]bool{
	"the": true, "and": true, "for": true, "this": true, "that": true,
	"with": true, "from": true, "into": true, "have": true, "will": true,
	"task": true, "file": true, "code": true, "make": true, "just": true,
	"some": true, "more": true, "also": true, "then": true,
}

// taskWords extracts meaningful lowercase words (4+ chars) from a task string.
func taskWords(task string) []string {
	var words []string
	seen := make(map[string]bool)
	for _, w := range strings.Fields(task) {
		w = strings.ToLower(strings.Trim(w, `.,!?;:"'()[]{}` + "`"))
		if len(w) >= 4 && !commonWords[w] && !seen[w] {
			words = append(words, w)
			seen[w] = true
		}
	}
	return words
}

// listSourceFiles returns all tracked files in dir.
// Falls back to filepath.Walk when git is not available.
func listSourceFiles(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "ls-files").Output()
	if err == nil {
		var files []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				files = append(files, line)
			}
		}
		return files
	}

	// Fallback: walk the directory tree.
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files
}

// gitGrepFiles returns files that contain any of the given words.
func gitGrepFiles(dir string, words []string) []string {
	if len(words) == 0 {
		return nil
	}
	out, err := exec.Command("git", "-C", dir, "grep", "-l", "-E", strings.Join(words, "|")).Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// gitRecentFiles returns files modified in the last 20 commits.
func gitRecentFiles(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "log", "--name-only", "--pretty=format:", "-20").Output()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			files = append(files, line)
		}
	}
	return files
}

// readTextFile reads a file and returns its contents as a string.
// Returns empty string for binary files, missing files, or read errors.
func readTextFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// Heuristic binary check: null bytes in first 512 bytes.
	check := b
	if len(check) > 512 {
		check = check[:512]
	}
	for _, c := range check {
		if c == 0 {
			return ""
		}
	}
	return string(b)
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
