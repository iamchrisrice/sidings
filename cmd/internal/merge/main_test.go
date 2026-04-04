package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runMerge(t *testing.T, input string) []summary {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	var results []summary
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var s summary
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("output line is not valid JSON: %v\nline: %s", err, line)
		}
		results = append(results, s)
	}
	return results
}

// runRaw returns raw output lines (for pass-through tests where output isn't a summary).
func runRaw(t *testing.T, input string) []string {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(strings.NewReader(input), &stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestAllCompleteStatusIsComplete(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"parent task","status":"complete","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"complete","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"complete","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	s := results[0]
	if s.Status != "complete" {
		t.Errorf("status = %q, want complete", s.Status)
	}
	if s.SubtasksTotal != 3 {
		t.Errorf("subtasks_total = %d, want 3", s.SubtasksTotal)
	}
	if s.SubtasksComplete != 3 {
		t.Errorf("subtasks_complete = %d, want 3", s.SubtasksComplete)
	}
	if s.SubtasksFailed != 0 {
		t.Errorf("subtasks_failed = %d, want 0", s.SubtasksFailed)
	}
}

func TestOneFailed_StatusIsPartial(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"parent task","status":"complete","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"complete","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"failed","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	if results[0].Status != "partial" {
		t.Errorf("status = %q, want partial", results[0].Status)
	}
	if results[0].SubtasksComplete != 2 {
		t.Errorf("subtasks_complete = %d, want 2", results[0].SubtasksComplete)
	}
	if results[0].SubtasksFailed != 1 {
		t.Errorf("subtasks_failed = %d, want 1", results[0].SubtasksFailed)
	}
}

func TestAllFailed_StatusIsFailed(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"parent task","status":"failed","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"failed","files_written":[]}
{"parent_task_id":"p1","parent_content":"parent task","status":"failed","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Errorf("status = %q, want failed", results[0].Status)
	}
}

func TestFilesWrittenDeduplicatedUnion(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":["a.go","b.go"]}
{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":["b.go","c.go"]}
{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":["a.go"]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	files := results[0].FilesWritten
	if len(files) != 3 {
		t.Errorf("files_written = %v, want [a.go b.go c.go]", files)
	}
	// Should be sorted.
	if files[0] != "a.go" || files[1] != "b.go" || files[2] != "c.go" {
		t.Errorf("files_written = %v, want [a.go b.go c.go]", files)
	}
}

func TestMultipleParentTaskIDs_TwoSummaryLines(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"first parent","status":"complete","files_written":[]}
{"parent_task_id":"p2","parent_content":"second parent","status":"complete","files_written":[]}
{"parent_task_id":"p1","parent_content":"first parent","status":"complete","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(results))
	}
	// p1 was seen first.
	if results[0].TaskID != "p1" {
		t.Errorf("first summary task_id = %q, want p1", results[0].TaskID)
	}
	if results[1].TaskID != "p2" {
		t.Errorf("second summary task_id = %q, want p2", results[1].TaskID)
	}
	if results[0].SubtasksTotal != 2 {
		t.Errorf("p1 subtasks_total = %d, want 2", results[0].SubtasksTotal)
	}
	if results[1].SubtasksTotal != 1 {
		t.Errorf("p2 subtasks_total = %d, want 1", results[1].SubtasksTotal)
	}
}

func TestLineWithNoParentTaskIDPassesThrough(t *testing.T) {
	input := `{"task_id":"t1","content":"standalone task","status":"complete"}`

	lines := runRaw(t, input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 output line, got %d", len(lines))
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if m["task_id"] != "t1" {
		t.Errorf("task_id = %v, want t1", m["task_id"])
	}
}

func TestMixOfPassThroughAndSubtasks(t *testing.T) {
	input := `{"task_id":"standalone","content":"solo","status":"complete"}
{"parent_task_id":"p1","parent_content":"parent","status":"complete","files_written":["x.go"]}
{"parent_task_id":"p1","parent_content":"parent","status":"complete","files_written":[]}`

	lines := runRaw(t, input)
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines (1 pass-through + 1 summary), got %d", len(lines))
	}

	// Pass-through comes first.
	var pt map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &pt); err != nil {
		t.Fatalf("first line not valid JSON: %v", err)
	}
	if pt["task_id"] != "standalone" {
		t.Errorf("first line task_id = %v, want standalone", pt["task_id"])
	}

	// Summary comes second.
	var s summary
	if err := json.Unmarshal([]byte(lines[1]), &s); err != nil {
		t.Fatalf("second line not valid JSON: %v", err)
	}
	if s.TaskID != "p1" {
		t.Errorf("summary task_id = %q, want p1", s.TaskID)
	}
	if s.SubtasksTotal != 2 {
		t.Errorf("subtasks_total = %d, want 2", s.SubtasksTotal)
	}
}

func TestMalformedNDJSONLineIsSkipped(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":[]}
not valid json {{{
{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary (malformed line skipped), got %d", len(results))
	}
	if results[0].SubtasksTotal != 2 {
		t.Errorf("subtasks_total = %d, want 2 (malformed line excluded)", results[0].SubtasksTotal)
	}
}

func TestEmptyInput_NoOutput(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(strings.NewReader(""), &stdout); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty output, got %q", stdout.String())
	}
}

func TestDurationMS_IsWallClockNotSum(t *testing.T) {
	// Each subtask reports duration_ms=9999, but the summary should use
	// wall clock elapsed (near 0 in tests), not the sum (29997).
	input := `{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":[],"duration_ms":9999}
{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":[],"duration_ms":9999}
{"parent_task_id":"p1","parent_content":"c","status":"complete","files_written":[],"duration_ms":9999}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	// Wall clock elapsed in tests is milliseconds, not 29997.
	if results[0].DurationMS >= 9999 {
		t.Errorf("duration_ms = %d, want < 9999 (should be wall clock, not sum)", results[0].DurationMS)
	}
}

func TestMissingStatus_TreatedAsComplete(t *testing.T) {
	// Lines with no "status" field should count as complete.
	input := `{"parent_task_id":"p1","parent_content":"c","files_written":[]}
{"parent_task_id":"p1","parent_content":"c","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	if results[0].Status != "complete" {
		t.Errorf("status = %q, want complete", results[0].Status)
	}
	if results[0].SubtasksComplete != 2 {
		t.Errorf("subtasks_complete = %d, want 2", results[0].SubtasksComplete)
	}
}

func TestSummaryContainsParentContent(t *testing.T) {
	input := `{"parent_task_id":"p1","parent_content":"add auth to bookmarks API","status":"complete","files_written":[]}`

	results := runMerge(t, input)
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	if results[0].Content != "add auth to bookmarks API" {
		t.Errorf("content = %q, want %q", results[0].Content, "add auth to bookmarks API")
	}
	if results[0].TaskID != "p1" {
		t.Errorf("task_id = %q, want p1", results[0].TaskID)
	}
}
