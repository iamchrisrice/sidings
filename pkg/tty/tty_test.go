package tty_test

import (
	"strings"
	"testing"

	"github.com/iamchrisrice/sidings/pkg/tty"
)

func TestConfirmReturnsTrueForY(t *testing.T) {
	if !tty.ConfirmFromReader("proceed?", strings.NewReader("y")) {
		t.Error("expected true for 'y'")
	}
}

func TestConfirmReturnsTrueForYes(t *testing.T) {
	if !tty.ConfirmFromReader("proceed?", strings.NewReader("yes")) {
		t.Error("expected true for 'yes'")
	}
}

func TestConfirmReturnsTrueForUppercaseY(t *testing.T) {
	if !tty.ConfirmFromReader("proceed?", strings.NewReader("Y")) {
		t.Error("expected true for 'Y' (case insensitive)")
	}
}

func TestConfirmReturnsFalseForN(t *testing.T) {
	if tty.ConfirmFromReader("proceed?", strings.NewReader("n")) {
		t.Error("expected false for 'n'")
	}
}

func TestConfirmReturnsFalseForNo(t *testing.T) {
	if tty.ConfirmFromReader("proceed?", strings.NewReader("no")) {
		t.Error("expected false for 'no'")
	}
}

func TestConfirmReturnsFalseForEmptyInput(t *testing.T) {
	if tty.ConfirmFromReader("proceed?", strings.NewReader("")) {
		t.Error("expected false for empty input")
	}
}
