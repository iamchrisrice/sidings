package classifier_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/classifier"
)

// ollamaServer starts a test HTTP server that mimics Ollama's /api/generate endpoint.
func ollamaServer(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, responseBody)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// withOllama returns a config pointed at the given test server.
func withOllama(ts *httptest.Server) classifier.Config {
	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	return cfg
}

// --- LLM tier parsing ---

func TestLLMReturnsSimple(t *testing.T) {
	ts := ollamaServer(t, `{"response":"simple"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("rename this variable")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "simple" {
		t.Errorf("tier = %q, want simple", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm", result.Method)
	}
}

func TestLLMReturnsMedium(t *testing.T) {
	ts := ollamaServer(t, `{"response":"medium"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("add a helper function for date formatting")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm", result.Method)
	}
}

func TestLLMReturnsComplex(t *testing.T) {
	ts := ollamaServer(t, `{"response":"complex"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("refactor the auth module")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm", result.Method)
	}
}

func TestLLMReturnsExceptional(t *testing.T) {
	ts := ollamaServer(t, `{"response":"exceptional"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("build a REST API with layered architecture")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm", result.Method)
	}
}

func TestLLMResponseIsTrimmedAndLowercased(t *testing.T) {
	ts := ollamaServer(t, `{"response":" Complex\n"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("refactor the payment module")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex (whitespace/case normalisation)", result.Tier)
	}
}

func TestLLMReturnsGarbageDefaultsToExceptional(t *testing.T) {
	ts := ollamaServer(t, `{"response":"banana"}`)
	c := classifier.New(withOllama(ts))
	result, err := c.Classify("some task")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (invalid LLM response)", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm (HTTP call succeeded)", result.Method)
	}
}

func TestLLMUnavailableDefaultsToExceptional(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // closed before any call

	c := classifier.New(withOllama(ts))
	result, err := c.Classify("some task")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (ollama unavailable)", result.Tier)
	}
	if result.Method != "fallback" {
		t.Errorf("method = %q, want fallback (ollama unavailable)", result.Method)
	}
}

func TestMethodIsLLMOnSuccessfulCall(t *testing.T) {
	for _, tier := range []string{"simple", "medium", "complex", "exceptional"} {
		tier := tier
		t.Run(tier, func(t *testing.T) {
			ts := ollamaServer(t, fmt.Sprintf(`{"response":"%s"}`, tier))
			c := classifier.New(withOllama(ts))
			result, err := c.Classify("some task")
			if err != nil {
				t.Fatal(err)
			}
			if result.Method != "llm" {
				t.Errorf("method = %q, want llm", result.Method)
			}
			if result.Tier != tier {
				t.Errorf("tier = %q, want %q", result.Tier, tier)
			}
		})
	}
}

func TestMethodIsFallbackWhenOllamaUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()

	c := classifier.New(withOllama(ts))
	result, err := c.Classify("some task")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "fallback" {
		t.Errorf("method = %q, want fallback", result.Method)
	}
}

func TestOllamaHTTPErrorDefaultsToExceptional(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := classifier.New(withOllama(ts))
	result, err := c.Classify("some task")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (HTTP 500)", result.Tier)
	}
	if result.Method != "fallback" {
		t.Errorf("method = %q, want fallback (HTTP 500)", result.Method)
	}
}

func TestOllamaRequestBodyContainsTaskContent(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf) //nolint:errcheck
		capturedBody = buf
		fmt.Fprintln(w, `{"response":"complex"}`)
	}))
	t.Cleanup(ts.Close)

	c := classifier.New(withOllama(ts))
	_, _ = c.Classify("refactor the auth module")

	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("Ollama request body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}
	prompt, _ := body["prompt"].(string)
	if !strings.Contains(prompt, "refactor the auth module") {
		t.Errorf("expected task content in prompt, got: %s", prompt)
	}
}
