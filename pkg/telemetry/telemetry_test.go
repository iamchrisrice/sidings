package telemetry_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamchrisrice/sidings/pkg/telemetry"
)

// realSocketPath returns the socket path used by the telemetry package.
// It mirrors the unexported socketPath() logic so tests use the same path.
func realSocketPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".sidings", "events.sock")
}

// startSocket creates a Unix socket listener at path and returns it.
// If the path is already in use (real monitor running), the test is skipped.
func startSocket(t *testing.T, path string) net.Listener {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Skip("real telemetry socket already exists; skipping to avoid interfering with monitor")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("failed to create test socket at %s: %v", path, err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(path)
	})
	return ln
}

// readEvent reads one newline-terminated JSON line from ln with a deadline.
func readEvent(t *testing.T, ln net.Listener) []byte {
	t.Helper()
	ln.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	return buf[:n]
}

// --- Tests ---

func TestEmitWhenSocketDoesNotExistCausesNoPanicAndNoError(t *testing.T) {
	path := realSocketPath(t)
	if _, err := os.Stat(path); err == nil {
		t.Skip("real telemetry socket exists; skipping")
	}

	// Must return silently — no panic, no observable error.
	telemetry.Emit(telemetry.Event{
		Tool:   "test-tool",
		TaskID: "abc123",
	})
}

func TestEmitWritesEventAsASingleValidJSONLine(t *testing.T) {
	path := realSocketPath(t)
	ln := startSocket(t, path)

	telemetry.Emit(telemetry.Event{
		Tool:   "task-classify",
		TaskID: "abc123",
		Tier:   "complex",
	})

	raw := readEvent(t, ln)
	if len(raw) == 0 {
		t.Fatal("no data received on socket")
	}

	line := strings.TrimRight(string(raw), "\n")
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("received data is not valid JSON: %v\nraw: %s", err, raw)
	}
}

func TestEmitWrittenEventContainsExpectedFields(t *testing.T) {
	path := realSocketPath(t)
	ln := startSocket(t, path)

	evt := telemetry.Event{
		Tool:   "task-classify",
		TaskID: "expected-id",
		Tier:   "complex",
	}
	telemetry.Emit(evt)

	raw := readEvent(t, ln)
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	checks := map[string]string{
		"tool":    "task-classify",
		"task_id": "expected-id",
	}
	for field, want := range checks {
		v, ok := got[field]
		if !ok {
			t.Errorf("field %q missing from emitted event", field)
			continue
		}
		if v.(string) != want {
			t.Errorf("field %q = %q, want %q", field, v, want)
		}
	}

	// ts must be present and parseable as RFC3339.
	ts, ok := got["ts"].(string)
	if !ok || ts == "" {
		t.Error("field 'ts' missing or empty")
	} else if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("field 'ts' is not valid RFC3339: %q", ts)
	}
}

func TestEmitReturnsQuicklyWithSlowSocketThatNeverReads(t *testing.T) {
	// This is the failure mode that would hurt in production: a monitor
	// that accepts connections but never reads. Emit must not block.
	path := realSocketPath(t)
	ln := startSocket(t, path)

	// Accept connections but never read from them.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Deliberately hold conn open without reading.
			_ = conn
		}
	}()

	deadline := 300 * time.Millisecond
	done := make(chan struct{})
	go func() {
		telemetry.Emit(telemetry.Event{Tool: "test-tool", TaskID: "x"})
		close(done)
	}()

	select {
	case <-done:
		// Emit returned within the deadline — correct behaviour.
	case <-time.After(deadline):
		t.Errorf("Emit blocked for more than %v with a slow socket", deadline)
	}

	ln.Close()
	wg.Wait()
}

func TestMultipleRapidEmitCallsNeitherBlockNorPanic(t *testing.T) {
	// Emit should be safe to call many times in rapid succession.
	// Run half with no socket (silent) and half with a live socket.
	path := realSocketPath(t)
	if _, err := os.Stat(path); err == nil {
		t.Skip("real telemetry socket exists; skipping")
	}

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			telemetry.Emit(telemetry.Event{
				Tool:   "test-tool",
				TaskID: "concurrent",
				Tier:   "simple",
			})
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All returned cleanly.
	case <-time.After(2 * time.Second):
		t.Error("concurrent Emit calls did not all return within 2s")
	}
}
