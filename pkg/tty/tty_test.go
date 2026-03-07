package tty_test

import (
	"testing"

	"github.com/iamchrisrice/sidings/pkg/tty"
)

func TestReaderReturnsNonNil(t *testing.T) {
	r := tty.Reader()
	if r == nil {
		t.Error("Reader() returned nil")
	}
}
