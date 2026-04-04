package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/pipe"
	"github.com/iamchrisrice/sidings/pkg/router"
)

func defaultTable() map[string]router.Decision {
	return router.DefaultTable()
}

func parseTask(t *testing.T, output string) pipe.Task {
	t.Helper()
	var task pipe.Task
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &task); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	return task
}

func TestRunSetsRouteOnClassifiedTask(t *testing.T) {
	input := `{"task_id":"t1","content":"refactor auth","tier":"complex"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Route == nil {
		t.Fatal("route is nil")
	}
	if task.Route.Model != "qwen3-coder" {
		t.Errorf("model = %q, want qwen3-coder", task.Route.Model)
	}
}

func TestRunMissingTierDefaultsToMedium(t *testing.T) {
	input := `{"task_id":"t1","content":"some task"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Tier != "medium" {
		t.Errorf("tier = %q, want medium (default)", task.Tier)
	}
	if task.Route == nil {
		t.Fatal("route is nil")
	}
}

func TestRunOutputIsValidNDJSON(t *testing.T) {
	input := `{"task_id":"t1","content":"add tests","tier":"simple"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	line := strings.TrimSpace(stdout.String())
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestRunPreservesTaskID(t *testing.T) {
	input := `{"task_id":"keep-me","content":"task","tier":"medium"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.TaskID != "keep-me" {
		t.Errorf("task_id = %q, want keep-me", task.TaskID)
	}
}

func TestRunAllDefaultTiersHaveRoutes(t *testing.T) {
	tiers := map[string]string{
		"simple":      "qwen3.5:0.8b",
		"medium":      "qwen3.5:9b",
		"complex":     "qwen3-coder",
		"exceptional": "",
	}
	for tier, wantModel := range tiers {
		tier, wantModel := tier, wantModel
		t.Run(tier, func(t *testing.T) {
			input := `{"task_id":"t1","content":"task","tier":"` + tier + `"}`
			var stdout bytes.Buffer
			if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
				t.Fatalf("run error: %v", err)
			}
			task := parseTask(t, stdout.String())
			if task.Route == nil {
				t.Fatal("route is nil")
			}
			if task.Route.Model != wantModel {
				t.Errorf("model = %q, want %q", task.Route.Model, wantModel)
			}
		})
	}
}

func TestRunUnknownTierRoutesAsMedium(t *testing.T) {
	input := `{"task_id":"t1","content":"task","tier":"mystery"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, defaultTable(), false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Route == nil {
		t.Fatal("route is nil")
	}
	medium := defaultTable()["medium"]
	if task.Route.Model != medium.Model {
		t.Errorf("model = %q, want medium model %q", task.Route.Model, medium.Model)
	}
}

func TestRunCustomTableOverridesModel(t *testing.T) {
	table := defaultTable()
	table["complex"] = router.Decision{Model: "custom-model"}

	input := `{"task_id":"t1","content":"task","tier":"complex"}`
	var stdout bytes.Buffer

	if err := run(strings.NewReader(input), &stdout, table, false); err != nil {
		t.Fatalf("run error: %v", err)
	}

	task := parseTask(t, stdout.String())
	if task.Route == nil || task.Route.Model != "custom-model" {
		t.Errorf("model = %v, want custom-model", task.Route)
	}
}
