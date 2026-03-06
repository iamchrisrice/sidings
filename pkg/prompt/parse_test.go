package prompt_test

import (
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/prompt"
)

func TestResponseWithSingleFileBlockReturnsOneFileChange(t *testing.T) {
	response := "<file path=\"internal/auth/handler.go\">\npackage auth\n\nfunc Handler() {}\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "internal/auth/handler.go" {
		t.Errorf("path = %q, want internal/auth/handler.go", changes[0].Path)
	}
	if !strings.Contains(changes[0].Content, "Handler") {
		t.Errorf("expected content to contain 'Handler', got: %q", changes[0].Content)
	}
}

func TestResponseWithMultipleFileBlocksReturnsAll(t *testing.T) {
	response := "<file path=\"a.go\">\npackage a\n</file>\n<file path=\"b.go\">\npackage b\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].Path != "a.go" {
		t.Errorf("first path = %q, want a.go", changes[0].Path)
	}
	if changes[1].Path != "b.go" {
		t.Errorf("second path = %q, want b.go", changes[1].Path)
	}
}

func TestResponseWithNoBlocksReturnsEmptySlice(t *testing.T) {
	response := "Here is my analysis of the problem. No code changes required."
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 0 {
		t.Errorf("expected empty slice, got %d changes", len(changes))
	}
}

func TestPlainTextResponseReturnsEmptySlice(t *testing.T) {
	response := "The issue is caused by a race condition in the goroutine pool.\n" +
		"You should add a mutex around the shared state.\n" +
		"Consider using sync.RWMutex for better read performance."
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 0 {
		t.Errorf("expected empty slice for plain text response, got %d changes", len(changes))
	}
}

func TestMalformedBlockMissingClosingTagHandledGracefully(t *testing.T) {
	// No closing </file> — the block is unclosed and must be silently discarded.
	response := "<file path=\"incomplete.go\">\npackage main\n\nfunc main() {}\n"
	// Must not panic.
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 0 {
		t.Errorf("unclosed block should be discarded, got %d changes", len(changes))
	}
}

func TestFilePathWithSubdirectoriesPreservedExactly(t *testing.T) {
	response := "<file path=\"internal/auth/middleware/jwt.go\">\npackage middleware\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "internal/auth/middleware/jwt.go" {
		t.Errorf("path = %q, want internal/auth/middleware/jwt.go", changes[0].Path)
	}
}

func TestBlocksWithExtraWhitespaceAroundTagsStillParsed(t *testing.T) {
	// Leading/trailing spaces on the tag lines should not break parsing.
	response := "  <file path=\"spaced.go\">  \npackage spaced\n  </file>  "
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d\nresponse: %q", len(changes), response)
	}
	if changes[0].Path != "spaced.go" {
		t.Errorf("path = %q, want spaced.go", changes[0].Path)
	}
}

func TestMultipleBlocksWithInterleavedPlainText(t *testing.T) {
	response := "I'll update two files:\n\n<file path=\"foo.go\">\npackage foo\n</file>\n\nDone.\n\n<file path=\"bar.go\">\npackage bar\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
}

func TestEmptyResponseReturnsEmptySlice(t *testing.T) {
	changes := prompt.ParseFileChanges("")
	if len(changes) != 0 {
		t.Errorf("expected empty slice for empty response, got %d", len(changes))
	}
}

func TestEmptyFileContentIsIncluded(t *testing.T) {
	response := "<file path=\"empty.go\">\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change for empty content block, got %d", len(changes))
	}
	if changes[0].Path != "empty.go" {
		t.Errorf("path = %q, want empty.go", changes[0].Path)
	}
}

func TestNestedContentThatLooksLikeTagsIsTreatedAsContent(t *testing.T) {
	// Content containing what looks like XML should not be parsed as block delimiters.
	response := "<file path=\"template.go\">\npackage tmpl\n// renders <file path=\"x\"> tags\n</file>"
	changes := prompt.ParseFileChanges(response)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !strings.Contains(changes[0].Content, "<file path=") {
		t.Error("expected inner tag-like content to be preserved as file content")
	}
}
