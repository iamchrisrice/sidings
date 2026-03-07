package executor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/prompt"
)

// OllamaConfig configures the Ollama executor.
type OllamaConfig struct {
	OllamaURL string // default: http://localhost:11434
	DryRun    bool   // print prompt to stderr, don't execute
	WorkDir   string // override working directory (empty = os.Getwd)
}

type ollamaExecutor struct {
	cfg    OllamaConfig
	client *http.Client
}

// NewOllama creates an Executor that calls the Ollama API.
func NewOllama(cfg OllamaConfig) Executor {
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = "http://localhost:11434"
	}
	return &ollamaExecutor{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (e *ollamaExecutor) workDir() (string, error) {
	if e.cfg.WorkDir != "" {
		if resolved, err := filepath.EvalSymlinks(e.cfg.WorkDir); err == nil {
			return resolved, nil
		}
		return e.cfg.WorkDir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(wd); err == nil {
		return resolved, nil
	}
	return wd, nil
}

func (e *ollamaExecutor) Execute(task pipe.Task, verbose bool) (Result, error) {
	start := time.Now()

	dir, err := e.workDir()
	if err != nil {
		return Result{}, err
	}

	p := prompt.Build(prompt.Config{
		Dir:   dir,
		Task:  task.Content,
		Tier:  task.Tier,
		Model: task.Route.Model,
	})

	if e.cfg.DryRun {
		fmt.Fprintln(os.Stderr, "--- built prompt ---")
		fmt.Fprintln(os.Stderr, p)
		fmt.Fprintln(os.Stderr, "--- end prompt ---")
		return Result{}, nil
	}

	response, err := e.callOllama(task.Route.Model, p, verbose)
	if err != nil {
		return Result{}, fmt.Errorf("ollama: %w", err)
	}

	changes := prompt.ParseFileChanges(response)
	if len(changes) == 0 {
		fmt.Fprintf(os.Stderr, "✓ done (%.1fs)\n", time.Since(start).Seconds())
		return Result{Output: response}, nil
	}

	gitRoot := findGitRoot(dir)

	var written []string
	for _, ch := range changes {
		if verbose {
			fmt.Fprintf(os.Stderr, "writing %s\n", ch.Path)
		}
		if err := e.writeFile(dir, gitRoot, ch); err != nil {
			return Result{}, err
		}
		written = append(written, ch.Path)
	}

	fmt.Fprintf(os.Stderr, "✓ wrote %d file(s) (%.1fs)\n", len(written), time.Since(start).Seconds())
	return Result{FilesWritten: written}, nil
}

type streamLine struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

func (e *ollamaExecutor) callOllama(model, promptText string, verbose bool) (string, error) {
	body, err := json.Marshal(ollamaRequest{Model: model, Prompt: promptText, Stream: true})
	if err != nil {
		return "", err
	}

	resp, err := e.client.Post(e.cfg.OllamaURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from Ollama", resp.StatusCode)
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line streamLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		sb.WriteString(line.Response)
		if verbose {
			fmt.Fprint(os.Stderr, line.Response)
		}
		if line.Done {
			break
		}
	}
	if verbose {
		fmt.Fprintln(os.Stderr)
	}
	return sb.String(), scanner.Err()
}

func (e *ollamaExecutor) writeFile(workDir, gitRoot string, ch prompt.FileChange) error {
	var absPath string
	if filepath.IsAbs(ch.Path) {
		absPath = filepath.Clean(ch.Path)
	} else {
		absPath = filepath.Clean(filepath.Join(workDir, ch.Path))
	}

	// Security: refuse paths outside the git repo.
	if gitRoot != "" {
		if !strings.HasPrefix(absPath, gitRoot+string(filepath.Separator)) {
			return fmt.Errorf("refusing to write %q: path is outside the git repo", ch.Path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(ch.Content), 0644)
}

func findGitRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		return resolved
	}
	return root
}
