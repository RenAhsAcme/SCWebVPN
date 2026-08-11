package main

import (
	"strings"
	"testing"
)

func TestBindingCodeComesFromSingleStdinValue(t *testing.T) {
	code, err := readBindingCode(strings.NewReader("opaque-code\n"))
	if err != nil || code != "opaque-code" {
		t.Fatalf("unexpected binding code: %q, %v", code, err)
	}
	if _, err := readBindingCode(strings.NewReader("first\nsecond\n")); err == nil {
		t.Fatal("multiple binding-code lines were accepted")
	}
}
