// Package classifier classifies coding tasks into routing tiers.
// Two-pass approach: heuristic keyword counting first, LLM fallback second.
package classifier

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// Classifier classifies a task string into a routing tier.
type Classifier interface {
	Classify(task string) (Result, error)
}

// Result holds the outcome of a classification.
type Result struct {
	Tier    string
	Method  string         // "heuristic", "llm", "length", or "default"
	Matched []string       // keywords that matched (heuristic only)
	Scores  map[string]int // per-tier keyword counts (heuristic only)
}

// Config holds classifier configuration.
type Config struct {
	OllamaURL       string
	ClassifierModel string
	LLMFallback     bool
}

// DefaultConfig returns sensible hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		OllamaURL:       "http://localhost:11434",
		ClassifierModel: "qwen3.5:0.8b",
		LLMFallback:     true,
	}
}

type impl struct {
	cfg Config
}

// New creates a Classifier with the given config.
func New(cfg Config) Classifier {
	return &impl{cfg: cfg}
}

// Classify runs two passes: heuristic then LLM fallback.
func (c *impl) Classify(task string) (Result, error) {
	lower := strings.ToLower(task)

	// Pass 1: keyword heuristics.
	scores := make(map[string]int, len(tiers))
	matched := make(map[string][]string, len(tiers))
	total := 0

	for _, tier := range tiers {
		for _, kw := range tier.Keywords {
			if strings.Contains(lower, kw) {
				scores[tier.Name]++
				matched[tier.Name] = append(matched[tier.Name], kw)
				total++
			}
		}
	}

	if total > 0 {
		winner, tied := topTier(scores)
		if !tied {
			return Result{
				Tier:    winner,
				Method:  "heuristic",
				Matched: matched[winner],
				Scores:  scores,
			}, nil
		}
		// Tied — fall through to LLM unless disabled.
		if !c.cfg.LLMFallback {
			// Pick the highest-priority tier among those that are tied.
			for _, t := range tiers {
				if scores[t.Name] == scores[winner] {
					return Result{
						Tier:    t.Name,
						Method:  "heuristic",
						Matched: matched[t.Name],
						Scores:  scores,
					}, nil
				}
			}
		}
	} else {
		// No keywords matched — use prompt length as a tiebreaker.
		l := len(task)
		if l < 60 {
			return Result{Tier: "simple", Method: "length"}, nil
		}
		if l > 800 {
			return Result{Tier: "exceptional", Method: "length"}, nil
		}
		// Length is ambiguous — fall through to LLM.
		if !c.cfg.LLMFallback {
			return Result{Tier: "medium", Method: "default"}, nil
		}
	}

	// Pass 2: LLM fallback.
	tier, err := c.llmClassify(task)
	if err != nil {
		// Ollama unavailable — degrade gracefully.
		return Result{Tier: "medium", Method: "default"}, nil
	}
	return Result{Tier: tier, Method: "llm"}, nil
}

// topTier returns the highest-scoring tier name and whether there is a tie
// at the top score. Iterates tiers in priority order.
func topTier(scores map[string]int) (string, bool) {
	best := ""
	bestScore := -1
	tied := false

	for _, t := range tiers {
		s := scores[t.Name]
		if s > bestScore {
			best = t.Name
			bestScore = s
			tied = false
		} else if s == bestScore && bestScore > 0 {
			tied = true
		}
	}
	return best, tied
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func (c *impl) llmClassify(task string) (string, error) {
	prompt := fmt.Sprintf(
		"Classify this coding task as one of: simple, medium, complex, exceptional.\nReply with one word only.\n\nTask: %s",
		task,
	)

	var out ollamaResponse
	client := resty.New().SetTimeout(30 * time.Second)
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(ollamaRequest{Model: c.cfg.ClassifierModel, Prompt: prompt, Stream: false}).
		SetResult(&out).
		Post(c.cfg.OllamaURL + "/api/generate")

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("ollama returned HTTP %d", resp.StatusCode())
	}

	tier := strings.ToLower(strings.TrimSpace(out.Response))
	switch tier {
	case "simple", "medium", "complex", "exceptional":
		return tier, nil
	}
	return "medium", nil
}
