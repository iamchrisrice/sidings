package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/iamchrisrice/sidings/pkg/telemetry"
	"github.com/spf13/cobra"
)

// summary is the merged result emitted for one parent task.
type summary struct {
	TaskID           string   `json:"task_id"`
	Content          string   `json:"content"`
	Status           string   `json:"status"`
	SubtasksTotal    int      `json:"subtasks_total"`
	SubtasksComplete int      `json:"subtasks_complete"`
	SubtasksFailed   int      `json:"subtasks_failed"`
	FilesWritten     []string `json:"files_written"`
	DurationMS       int64    `json:"duration_ms"`
}

// group accumulates subtask results for one parent_task_id.
type group struct {
	content   string
	total     int
	complete  int
	failed    int
	files     map[string]struct{}
	firstSeen time.Time
	lastSeen  time.Time
}

// inputLine holds the fields we need from each incoming NDJSON line.
type inputLine struct {
	ParentTaskID  string   `json:"parent_task_id"`
	ParentContent string   `json:"parent_content"`
	Status        string   `json:"status"`
	FilesWritten  []string `json:"files_written"`
}

func run(stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)

	groups := map[string]*group{}     // parent_task_id → group
	var groupOrder []string           // tracks insertion order for deterministic output
	var passThrough []json.RawMessage // lines without parent_task_id

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		// Copy bytes — scanner reuses the underlying buffer.
		line := make([]byte, len(raw))
		copy(line, raw)

		var t inputLine
		if err := json.Unmarshal(line, &t); err != nil {
			fmt.Fprintf(os.Stderr, "sidings: skipping malformed NDJSON: %v\n", err)
			continue
		}

		if t.ParentTaskID == "" {
			passThrough = append(passThrough, json.RawMessage(line))
			continue
		}

		now := time.Now()
		g, exists := groups[t.ParentTaskID]
		if !exists {
			g = &group{
				content:   t.ParentContent,
				files:     map[string]struct{}{},
				firstSeen: now,
			}
			groups[t.ParentTaskID] = g
			groupOrder = append(groupOrder, t.ParentTaskID)
		}
		g.lastSeen = now
		g.total++

		status := t.Status
		if status == "" {
			status = "complete"
		}
		if status == "complete" {
			g.complete++
		} else {
			g.failed++
		}

		for _, f := range t.FilesWritten {
			if f != "" {
				g.files[f] = struct{}{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	// Emit pass-through lines first.
	for _, raw := range passThrough {
		fmt.Fprintf(stdout, "%s\n", raw)
	}

	// Emit one summary line per parent group.
	for _, parentID := range groupOrder {
		g := groups[parentID]

		var status string
		switch {
		case g.failed == 0:
			status = "complete"
		case g.complete == 0:
			status = "failed"
		default:
			status = "partial"
		}

		files := make([]string, 0, len(g.files))
		for f := range g.files {
			files = append(files, f)
		}
		sort.Strings(files)

		durationMS := g.lastSeen.Sub(g.firstSeen).Milliseconds()

		s := summary{
			TaskID:           parentID,
			Content:          g.content,
			Status:           status,
			SubtasksTotal:    g.total,
			SubtasksComplete: g.complete,
			SubtasksFailed:   g.failed,
			FilesWritten:     files,
			DurationMS:       durationMS,
		}

		b, err := json.Marshal(s)
		if err != nil {
			return fmt.Errorf("marshalling summary: %w", err)
		}
		fmt.Fprintf(stdout, "%s\n", b)

		telemetry.Emit(telemetry.Event{
			Tool:       "task-merge",
			TaskID:     parentID,
			Status:     status,
			DurationMS: durationMS,
		})
	}

	return nil
}

func main() {
	root := &cobra.Command{
		Use:          "task-merge",
		Short:        "Merge parallel subtask results into a single summary",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(os.Stdin, os.Stdout)
		},
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
