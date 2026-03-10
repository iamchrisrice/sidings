// Package classifier classifies coding tasks into routing tiers using a local LLM.
package classifier

import (
	"fmt"
	"net/http"
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
	TimeoutSeconds  int // 0 = use default (15s)
}

// DefaultConfig returns sensible hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		OllamaURL:       "http://localhost:11434",
		ClassifierModel: "qwen3.5:9b",
	}
}

// Classifier classifies a task string into a routing tier.
type Classifier interface {
	Classify(task string) (Result, error)
}

type impl struct {
	cfg    Config
	client *http.Client
}

// New creates a Classifier with the given config.
func New(cfg Config) Classifier {
	timeout := 15 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return &impl{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func buildPrompt(task string) string {
	return fmt.Sprintf(`Classify this coding task. Reply with exactly one word.

simple = typo fix, rename variable, add comment
medium = add function, write test, small change
complex = multi-file refactor, implement feature
exceptional = new project, system design, infrastructure, docker, kubernetes

Task: %s

Tier:`, task)
}

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
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Stream  bool                   `json:"stream"`
	Think   bool                   `json:"think"`   // top-level — NOT inside options
	Options map[string]interface{} `json:"options"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func (c *impl) callLLM(task string) (string, error) {
	req := ollamaRequest{
		Model:  c.cfg.ClassifierModel,
		Prompt: buildPrompt(task),
		Stream: false,
		Think:  false, // disable chain-of-thought — must be top-level
		Options: map[string]interface{}{
			"num_predict": 5,   // cap response to 5 tokens — enough for one word
			"num_ctx":     512, // small context — classifier prompt is short
			"temperature": 0,   // deterministic — same task always returns same tier
		},
	}

	var out ollamaResponse
	client := resty.NewWithClient(c.client)
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(req).
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
