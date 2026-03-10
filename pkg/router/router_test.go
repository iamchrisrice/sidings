package router_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/router"
)

// --- Default routing table ---

func TestEachDefaultTierRoutesToExpectedModel(t *testing.T) {
	r := router.New(router.DefaultTable())

	cases := []struct {
		tier  string
		model string
	}{
		{"simple", "qwen3.5:0.8b"},
		{"medium", "qwen3.5:9b"},
		{"complex", "qwen3-coder"},
		{"exceptional", ""},
	}
	for _, tc := range cases {
		d, err := r.Route(tc.tier)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.tier, err)
			continue
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
	if d.Model != medium.Model {
		t.Errorf("unknown tier: model = %q, want medium model %q", d.Model, medium.Model)
	}
}

// --- Config file ---

func TestConfigFileOverridesDefaultRoutingTable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "route.yaml")
	err := os.WriteFile(cfgPath, []byte(`
routes:
  complex:
    model: qwen2.5-coder:32b
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
	if d.Model != "qwen2.5-coder:32b" {
		t.Errorf("complex: model = %q, want qwen2.5-coder:32b", d.Model)
	}

	// Other tiers should still use defaults.
	simple, _ := r.Route("simple")
	if simple.Model != "qwen3.5:0.8b" {
		t.Errorf("simple: model = %q, want default qwen3.5:0.8b", simple.Model)
	}
}

func TestMissingConfigFileFallsBackToDefaultsGracefully(t *testing.T) {
	table := router.LoadConfigFrom("/nonexistent/path/route.yaml")
	r := router.New(table)

	d, err := r.Route("simple")
	if err != nil {
		t.Fatal(err)
	}
	if d.Model != "qwen3.5:0.8b" {
		t.Errorf("simple: model = %q, want default qwen3.5:0.8b", d.Model)
	}
}
