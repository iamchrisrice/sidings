package router_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/router"
)

// --- Default routing table ---

func TestEachDefaultTierRoutesToExpectedBackendAndModel(t *testing.T) {
	r := router.New(router.DefaultTable())

	cases := []struct {
		tier    string
		backend string
		model   string
	}{
		{"simple", "ollama", "qwen3.5:0.8b"},
		{"medium", "ollama", "qwen3.5:9b"},
		{"complex", "ollama", "qwen2.5-coder:32b"},
		{"exceptional", "claude", "sonnet"},
	}
	for _, tc := range cases {
		d, err := r.Route(tc.tier)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.tier, err)
			continue
		}
		if d.Backend != tc.backend {
			t.Errorf("%s: backend = %q, want %q", tc.tier, d.Backend, tc.backend)
		}
		if d.Model != tc.model {
			t.Errorf("%s: model = %q, want %q", tc.tier, d.Model, tc.model)
		}
	}
}

func TestUnknownTierDefaultsToMediumWithNoPanic(t *testing.T) {
	r := router.New(router.DefaultTable())
	d, err := r.Route("nonexistent-tier")
	if err != nil {
		t.Fatalf("unexpected error for unknown tier: %v", err)
	}
	medium, _ := r.Route("medium")
	if d.Backend != medium.Backend || d.Model != medium.Model {
		t.Errorf("unknown tier: got {%s %s}, want medium route {%s %s}",
			d.Backend, d.Model, medium.Backend, medium.Model)
	}
}

// --- Config file ---

func TestConfigFileOverridesDefaultRoutingTable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "route.yaml")
	err := os.WriteFile(cfgPath, []byte(`
routes:
  complex:
    backend: claude
    model: opus
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	table := router.LoadConfigFrom(cfgPath)
	r := router.New(table)

	d, err := r.Route("complex")
	if err != nil {
		t.Fatal(err)
	}
	if d.Backend != "claude" || d.Model != "opus" {
		t.Errorf("complex: got {%s %s}, want {claude opus}", d.Backend, d.Model)
	}

	// Other tiers should still use defaults.
	simple, _ := r.Route("simple")
	if simple.Backend != "ollama" || simple.Model != "qwen3.5:0.8b" {
		t.Errorf("simple: got {%s %s}, want default {ollama qwen3.5:0.8b}",
			simple.Backend, simple.Model)
	}
}

func TestMissingConfigFileFallsBackToDefaultsGracefully(t *testing.T) {
	table := router.LoadConfigFrom("/nonexistent/path/route.yaml")
	r := router.New(table)

	d, err := r.Route("simple")
	if err != nil {
		t.Fatal(err)
	}
	if d.Backend != "ollama" || d.Model != "qwen3.5:0.8b" {
		t.Errorf("simple: got {%s %s}, want default {ollama qwen3.5:0.8b}", d.Backend, d.Model)
	}
}
