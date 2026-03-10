package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubBinary(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLibexecPathSIDINGS_LIBEXECTakesFirst(t *testing.T) {
	dir := t.TempDir()
	stub := stubBinary(t, dir, "task-classify")
	t.Setenv("SIDINGS_LIBEXEC", dir)

	path, err := libexecPath("task-classify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != stub {
		t.Errorf("path = %q, want %q", path, stub)
	}
}

func TestLibexecPathHomeLocalLibexec(t *testing.T) {
	dir := t.TempDir()
	libexecDir := filepath.Join(dir, ".local", "libexec", "sidings")
	if err := os.MkdirAll(libexecDir, 0755); err != nil {
		t.Fatal(err)
	}
	stub := stubBinary(t, libexecDir, "task-route")

	t.Setenv("HOME", dir)
	t.Setenv("SIDINGS_LIBEXEC", "") // prevent SIDINGS_LIBEXEC from winning

	path, err := libexecPath("task-route")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != stub {
		t.Errorf("path = %q, want %q", path, stub)
	}
}

func TestLibexecPathNotFound(t *testing.T) {
	t.Setenv("SIDINGS_LIBEXEC", "")
	t.Setenv("HOME", t.TempDir()) // empty home — no binaries installed

	// Use a binary name that won't exist anywhere on this machine.
	_, err := libexecPath("sidings-nonexistent-binary-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if !strings.Contains(err.Error(), "sidings-nonexistent-binary-xyz") {
		t.Errorf("error should mention the binary name, got: %v", err)
	}
}

func TestLibexecPathSameDirFallback(t *testing.T) {
	dir := t.TempDir()
	stub := stubBinary(t, dir, "task-dispatch")

	// libexecCandidates uses filepath.Dir(argv0) as the last candidate.
	// Pass a fake argv0 in the same dir as the stub.
	fakeArgv0 := filepath.Join(dir, "sidings")
	candidates := libexecCandidates("task-dispatch", fakeArgv0)

	found := false
	for _, c := range candidates {
		if c == stub {
			found = true
		}
	}
	if !found {
		t.Errorf("same-dir candidate %q not found in candidates: %v", stub, candidates)
	}
}

func TestLibexecPathSIDINGS_LIBEXECBeatsHomeLocal(t *testing.T) {
	homeDir := t.TempDir()
	libexecDir := filepath.Join(homeDir, ".local", "libexec", "sidings")
	if err := os.MkdirAll(libexecDir, 0755); err != nil {
		t.Fatal(err)
	}
	stubBinary(t, libexecDir, "task-classify") // exists in home

	envDir := t.TempDir()
	envStub := stubBinary(t, envDir, "task-classify") // also exists in SIDINGS_LIBEXEC

	t.Setenv("HOME", homeDir)
	t.Setenv("SIDINGS_LIBEXEC", envDir)

	path, err := libexecPath("task-classify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != envStub {
		t.Errorf("path = %q, want SIDINGS_LIBEXEC path %q", path, envStub)
	}
}
