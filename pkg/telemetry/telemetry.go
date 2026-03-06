// Package telemetry emits structured JSON events to a Unix socket.
// It is completely silent if the socket does not exist, and never blocks
// or errors noisily — the pipeline must not be affected by monitor presence.
package telemetry

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Event is a structured telemetry event emitted by a sidings tool.
type Event struct {
	Tool       string   `json:"tool"`
	TaskID     string   `json:"task_id"`
	Tier       string   `json:"tier,omitempty"`
	Method     string   `json:"method,omitempty"`
	Matched    []string `json:"matched_keywords,omitempty"`
	Backend    string   `json:"backend,omitempty"`
	Model      string   `json:"model,omitempty"`
	Status     string   `json:"status,omitempty"`
	DurationMS int64    `json:"duration_ms,omitempty"`
	TS         string   `json:"ts"`
}

const writeTimeout = 100 * time.Millisecond

func socketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sidings", "events.sock")
}

// Emit sends e to the telemetry socket as a single JSON line.
// Returns silently if the socket does not exist, cannot be reached,
// or the write times out.
func Emit(e Event) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}

	addr := socketPath()
	if addr == "" {
		return
	}
	if _, err := os.Stat(addr); err != nil {
		// Socket not present — monitor not running.
		return
	}

	conn, err := net.DialTimeout("unix", addr, writeTimeout)
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))

	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = conn.Write(b)
}
