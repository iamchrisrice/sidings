// Package classifier classifies coding tasks into routing tiers using a local LLM.
package classifier

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// Result holds the outcome of a classification.
type Result struct {
	Tier   string // "simple", "medium", "complex", "exceptional"
	Method string // "llm" or "fallback"
}

// Config holds classifier configuration.
type Config struct {
	OllamaURL       string
	ClassifierModel string
}

// DefaultConfig returns sensible hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		OllamaURL:       "http://localhost:11434",
		ClassifierModel: "qwen3.5:0.8b",
	}
}

// Classifier classifies a task string into a routing tier.
type Classifier interface {
	Classify(task string) (Result, error)
}

type impl struct {
	cfg Config
}

// New creates a Classifier with the given config.
func New(cfg Config) Classifier {
	return &impl{cfg: cfg}
}

const classifyPrompt = `You are a task complexity classifier. Classify the following coding task into exactly one tier:

- simple: single-line changes, typos, renames, adding a comment
- medium: adding a function, writing a test, small self-contained change
- complex: multi-file changes, refactoring, implementing a feature
- exceptional: greenfield projects, system design, deep debugging, anything involving infrastructure, deployment, Docker, Kubernetes

Reply with exactly one word: simple, medium, complex, or exceptional.
No explanation. No punctuation. Just the tier.

Task: %s`

// Classify asks the local LLM to classify the task.
// If Ollama is unavailable, defaults to "exceptional".
func (c *impl) Classify(task string) (Result, error) {
	tier, err := c.callLLM(task)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sidings: ollama unavailable, defaulting to exceptional")
		return Result{Tier: "exceptional", Method: "fallback"}, nil
	}
	return Result{Tier: tier, Method: "llm"}, nil
}

func (c *impl) ollamaURL() string {
	if c.cfg.OllamaURL == "" {
		return "http://localhost:11434"
	}
	return c.cfg.OllamaURL
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func (c *impl) callLLM(task string) (string, error) {
	prompt := fmt.Sprintf(classifyPrompt, task)

	var out ollamaResponse
	client := resty.New().SetTimeout(30 * time.Second)
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(ollamaRequest{Model: c.cfg.ClassifierModel, Prompt: prompt, Stream: false}).
		SetResult(&out).
		Post(c.ollamaURL() + "/api/generate")

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("ollama returned HTTP %d", resp.StatusCode())
	}

	return parseTier(out.Response), nil
}

func parseTier(response string) string {
	tier := strings.ToLower(strings.TrimSpace(response))
	switch tier {
	case "simple", "medium", "complex", "exceptional":
		return tier
	default:
		return "exceptional"
	}
}
