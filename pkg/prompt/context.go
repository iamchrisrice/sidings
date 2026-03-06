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
// Reads README, linked docs, ranked source files, and a project tree.
// model is used to determine the token budget for source files.
func GatherContext(dir string, task string, model string) string {
	budgetChars := tokenBudgetChars(model)
	var sb strings.Builder

	// README — primary context.
	readme := readTextFile(filepath.Join(dir, "README.md"))
	if readme != "" {
		sb.WriteString("### README\n\n")
		sb.WriteString(readme)
		sb.WriteString("\n\n")
		budgetChars -= len(readme)

		// Linked local docs, one level deep.
		for _, link := range extractLocalLinks(readme) {
			if isSecret(link) {
				continue
			}
			content := readTextFile(filepath.Join(dir, link))
			if content != "" && budgetChars > 0 {
				entry := fmt.Sprintf("### %s\n\n%s\n\n", link, content)
				sb.WriteString(entry)
				budgetChars -= len(entry)
			}
		}
	}

	// Relevant source files, ranked by relevance to the task.
	ranked := rankFiles(dir, task)
	if len(ranked) > 0 {
		sb.WriteString("### Source files\n\n")
		for _, f := range ranked {
			if budgetChars <= 0 {
				break
			}
			content := readTextFile(filepath.Join(dir, f))
			if content == "" {
				continue
			}
			block := fmt.Sprintf("// %s\n%s\n\n", f, content)
			if len(block) > budgetChars {
				break
			}
			sb.WriteString(block)
			budgetChars -= len(block)
		}
	}

	// Shallow project tree.
	tree := projectTree(dir)
	if tree != "" {
		sb.WriteString("### Project structure\n\n```\n")
		sb.WriteString(tree)
		sb.WriteString("\n```\n")
	}

	return sb.String()
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

// extractLocalLinks returns local file paths from markdown links in text.
// External URLs (http/https) are ignored.
func extractLocalLinks(text string) []string {
	var links []string
	for _, m := range mdLinkRe.FindAllStringSubmatch(text, -1) {
		href := m[1]
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			continue
		}
		href = strings.TrimPrefix(href, "./")
		links = append(links, href)
	}
	return links
}

var excludeDirs = []string{"vendor/", "node_modules/", ".git/"}
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
		if strings.HasPrefix(path, dir) || strings.Contains(path, "/"+dir) {
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

// projectTree returns a flat list of tracked files as a simple tree.
func projectTree(dir string) string {
	out, err := exec.Command("git", "-C", dir, "ls-files").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
