package classifier_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/classifier"
)

// --- helpers ---

// noLLM returns a config with LLM fallback disabled. Use for tests that
// only exercise the heuristic and length passes.
func noLLM() classifier.Config {
	cfg := classifier.DefaultConfig()
	cfg.LLMFallback = false
	return cfg
}

// ollamaServer starts a test HTTP server that mimics Ollama's /api/generate
// endpoint. responseBody is returned verbatim as the JSON body.
func ollamaServer(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, responseBody)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// withOllama returns a config pointed at the given test server with LLM enabled.
func withOllama(ts *httptest.Server) classifier.Config {
	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	cfg.LLMFallback = true
	return cfg
}

// --- Heuristics ---

func TestSimpleKeywordMatchReturnsSimpleTier(t *testing.T) {
	c := classifier.New(noLLM())
	result, err := c.Classify("fix the typo in the comment")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "simple" {
		t.Errorf("tier = %q, want simple", result.Tier)
	}
	if result.Method != "heuristic" {
		t.Errorf("method = %q, want heuristic", result.Method)
	}
}

func TestComplexKeywordMatchReturnsComplexTier(t *testing.T) {
	c := classifier.New(noLLM())
	result, err := c.Classify("refactor the auth module")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex", result.Tier)
	}
	if result.Method != "heuristic" {
		t.Errorf("method = %q, want heuristic", result.Method)
	}
}

func TestMatchingKeywordsAreCaseInsensitive(t *testing.T) {
	c := classifier.New(noLLM())
	result, err := c.Classify("REFACTOR THE AUTH MODULE")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex (case insensitive)", result.Tier)
	}
}

func TestMultiWordKeywordMatchesCorrectly(t *testing.T) {
	// "system design" is a multi-word exceptional keyword.
	c := classifier.New(noLLM())
	result, err := c.Classify("do a system design for the new service")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (multi-word keyword 'system design')", result.Tier)
	}
}

func TestHigherScoreWinsWhenTiersDiffer(t *testing.T) {
	// "fix the code and refactor across the codebase"
	//   simple:  "fix"                         = 1
	//   complex: "refactor", "across the codebase" = 2
	// complex score is higher so complex wins without LLM.
	c := classifier.New(noLLM())
	result, err := c.Classify("fix the code and refactor across the codebase")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex (score complex:2 > simple:1)", result.Tier)
	}
}

func TestShortPromptWithNoKeywordsResolvesToSimpleViaLengthRule(t *testing.T) {
	c := classifier.New(noLLM())
	// Under 60 chars, no tier keywords.
	result, err := c.Classify("do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "simple" {
		t.Errorf("tier = %q, want simple (short length)", result.Tier)
	}
	if result.Method != "length" {
		t.Errorf("method = %q, want length", result.Method)
	}
}

func TestLongPromptWithNoKeywordsResolvesToExceptionalViaLengthRule(t *testing.T) {
	c := classifier.New(noLLM())
	// Over 800 chars, no tier keywords.
	task := strings.Repeat("do the thing. ", 60) // ~840 chars
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "exceptional" {
		t.Errorf("tier = %q, want exceptional (long length)", result.Tier)
	}
	if result.Method != "length" {
		t.Errorf("method = %q, want length", result.Method)
	}
}

func TestResultMatchedContainsTheKeywordsThatFired(t *testing.T) {
	c := classifier.New(noLLM())
	result, err := c.Classify("refactor the auth module")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matched) == 0 {
		t.Fatal("Matched should contain at least one keyword")
	}
	found := false
	for _, kw := range result.Matched {
		if kw == "refactor" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'refactor' in Matched, got %v", result.Matched)
	}
}

func TestResultMethodIsHeuristicWhenHeuristicsResolveItCleanly(t *testing.T) {
	// A clear single-tier match should never reach LLM.
	c := classifier.New(noLLM())
	for _, tc := range []struct {
		input string
		tier  string
	}{
		{"fix the typo", "simple"},
		{"refactor the package", "complex"},
		{"why is the server deadlocking", "exceptional"},
	} {
		result, err := c.Classify(tc.input)
		if err != nil {
			t.Fatalf("%q: %v", tc.input, err)
		}
		if result.Method != "heuristic" {
			t.Errorf("%q: method = %q, want heuristic", tc.input, result.Method)
		}
		if result.Tier != tc.tier {
			t.Errorf("%q: tier = %q, want %q", tc.input, result.Tier, tc.tier)
		}
	}
}

// --- Tie detection and LLM fallback ---

func TestTieBetweenTwoTiersTriggerLLMFallback(t *testing.T) {
	// "add a refactor" → medium:1 ("add"), complex:1 ("refactor") → tied.
	ts := ollamaServer(t, `{"response":"complex"}`)
	c := classifier.New(withOllama(ts))

	result, err := c.Classify("add a refactor")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm (tie should trigger LLM fallback)", result.Method)
	}
	if result.Tier != "complex" {
		t.Errorf("tier = %q, want complex (LLM returned 'complex')", result.Tier)
	}
}

func TestZeroKeywordsWithMidLengthPromptTriggerLLMFallback(t *testing.T) {
	// 120 chars, no tier keywords → not resolved by length rules → LLM.
	ts := ollamaServer(t, `{"response":"medium"}`)
	c := classifier.New(withOllama(ts))

	task := strings.Repeat("neutral ", 15) // 120 chars, no keywords
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm (mid-length, no keywords)", result.Method)
	}
}

// --- LLM fallback behaviour ---

func TestOllamaReturnsValidTierMethodIsLLM(t *testing.T) {
	for _, tier := range []string{"simple", "medium", "complex", "exceptional"} {
		tier := tier
		t.Run(tier, func(t *testing.T) {
			ts := ollamaServer(t, fmt.Sprintf(`{"response":"%s"}`, tier))
			c := classifier.New(withOllama(ts))

			// Mid-length, no keywords → falls through to LLM.
			task := strings.Repeat("neutral ", 15)
			result, err := c.Classify(task)
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

func TestOllamaReturnsUnexpectedStringDefaultsToMediumGracefully(t *testing.T) {
	// Ollama returns a nonsense string — classifier defaults to medium.
	// The HTTP call succeeded so Method is still "llm".
	ts := ollamaServer(t, `{"response":"banana"}`)
	c := classifier.New(withOllama(ts))

	task := strings.Repeat("neutral ", 15)
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium (unexpected LLM output)", result.Tier)
	}
	if result.Method != "llm" {
		t.Errorf("method = %q, want llm (HTTP call succeeded)", result.Method)
	}
}

func TestOllamaUnavailableDefaultsToMedium(t *testing.T) {
	// Point at a closed server to simulate connection refused.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // close before any call is made

	c := classifier.New(withOllama(ts))
	task := strings.Repeat("neutral ", 15)
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium (ollama unavailable)", result.Tier)
	}
	if result.Method != "default" {
		t.Errorf("method = %q, want default (ollama unavailable)", result.Method)
	}
}

func TestOllamaHTTPErrorDefaultsToMediumGracefully(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := classifier.New(withOllama(ts))
	task := strings.Repeat("neutral ", 15)
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium (HTTP 500)", result.Tier)
	}
	if result.Method != "default" {
		t.Errorf("method = %q, want default (HTTP 500)", result.Method)
	}
}

func TestOllamaDropsConnectionDefaultsToMediumGracefully(t *testing.T) {
	// Simulate a connection that is accepted then immediately dropped —
	// same recovery path as a timeout.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(500)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close() // drop the connection
	}))
	t.Cleanup(ts.Close)

	c := classifier.New(withOllama(ts))
	task := strings.Repeat("neutral ", 15)
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium (dropped connection)", result.Tier)
	}
	if result.Method != "default" {
		t.Errorf("method = %q, want default (dropped connection)", result.Method)
	}
}

// --- --no-llm flag behaviour ---

func TestTieWithNoLLMFlagMakesNoHTTPCall(t *testing.T) {
	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(500)
	}))
	t.Cleanup(ts.Close)

	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	cfg.LLMFallback = false
	c := classifier.New(cfg)

	// "add a refactor" → medium:1, complex:1 → tied, but LLM is disabled.
	result, err := c.Classify("add a refactor")
	if err != nil {
		t.Fatal(err)
	}
	if callCount.Load() > 0 {
		t.Error("HTTP call was made despite LLM fallback being disabled")
	}
	if result.Method == "llm" {
		t.Error("method should not be llm when LLM fallback is disabled")
	}
}

func TestZeroKeywordsWithNoLLMFlagMakesNoHTTPCall(t *testing.T) {
	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(500)
	}))
	t.Cleanup(ts.Close)

	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	cfg.LLMFallback = false
	c := classifier.New(cfg)

	task := strings.Repeat("neutral ", 15) // no keywords, mid-length
	result, err := c.Classify(task)
	if err != nil {
		t.Fatal(err)
	}
	if callCount.Load() > 0 {
		t.Error("HTTP call was made despite LLM fallback being disabled")
	}
	if result.Tier != "medium" {
		t.Errorf("tier = %q, want medium (no-llm default)", result.Tier)
	}
	if result.Method != "default" {
		t.Errorf("method = %q, want default", result.Method)
	}
}

// --- extra coverage ---

func TestScoresMapIsPopulatedForHeuristicResults(t *testing.T) {
	c := classifier.New(noLLM())
	result, err := c.Classify("refactor the auth module")
	if err != nil {
		t.Fatal(err)
	}
	if result.Scores == nil {
		t.Fatal("Scores should be populated for heuristic results")
	}
	if result.Scores["complex"] == 0 {
		t.Error("complex score should be non-zero for a prompt containing 'refactor'")
	}
}

// --- Regression table ---

// TestRegressionTable guards against misclassification of real-world inputs.
// Every case must resolve cleanly via heuristic (method="heuristic") with no
// LLM call — if a case needs LLM it means the keyword lists need tuning.
func TestRegressionTable(t *testing.T) {
	cases := []struct {
		input string
		tier  string
	}{
		// simple
		{"fix the typo in the comment", "simple"},
		{"rename this variable", "simple"},
		// medium
		{"create a test for the login handler", "medium"},
		{"create a helper function for formatting dates", "medium"},
		{"add a retry mechanism to the HTTP client", "medium"},
		// complex
		{"refactor the payment module", "complex"},
		{"extract the database layer into its own package", "complex"},
		// exceptional
		{"build a REST API with layered architecture and full tests", "exceptional"},
		{"build me a simple CRUD API", "exceptional"},
		{"why is the login handler not returning the right response", "exceptional"},
		{"generate a REST API with Docker and Kubernetes deployment on Helm", "exceptional"},
		{"scaffold a microservice with CI/CD pipeline", "exceptional"},
	}

	c := classifier.New(noLLM())
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			result, err := c.Classify(tc.input)
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if result.Tier != tc.tier {
				t.Errorf("tier = %q, want %q (method=%s matched=%v scores=%v)",
					result.Tier, tc.tier, result.Method, result.Matched, result.Scores)
			}
			if result.Method != "heuristic" {
				t.Errorf("method = %q, want heuristic — keyword lists need tuning (matched=%v scores=%v)",
					result.Method, result.Matched, result.Scores)
			}
		})
	}
}

func TestOllamaRequestBodyContainsTaskContent(t *testing.T) {
	// Verify the classifier sends the task content in the Ollama request body.
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf) //nolint:errcheck
		capturedBody = buf
		fmt.Fprintln(w, `{"response":"medium"}`)
	}))
	t.Cleanup(ts.Close)

	cfg := classifier.DefaultConfig()
	cfg.OllamaURL = ts.URL
	cfg.LLMFallback = true
	c := classifier.New(cfg)

	task := strings.Repeat("neutral ", 15) // triggers LLM
	_, _ = c.Classify(task)

	var body map[string]interface{}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("Ollama request body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}
	prompt, _ := body["prompt"].(string)
	if !strings.Contains(prompt, "neutral") {
		t.Errorf("expected task content in prompt, got: %s", prompt)
	}
}
