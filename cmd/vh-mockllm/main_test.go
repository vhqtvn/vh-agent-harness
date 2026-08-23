// main_test.go — the CLI surface: --script is REQUIRED and fails
// closed (exit 2) when missing, unreadable, or malformed.
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunScriptFlagRequired(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"--addr", "127.0.0.1:1"}, &buf)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "--script is REQUIRED") {
		t.Fatalf("stderr must name the missing flag:\n%s", buf.String())
	}
}

func TestRunMalformedScriptExitsTwo(t *testing.T) {
	p := writeScript(t, `[{"text":"ok"},{"bogus":true}]`)
	var buf bytes.Buffer
	code := run([]string{"--script", p}, &buf)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "step 2") {
		t.Fatalf("stderr must name the offending step:\n%s", buf.String())
	}
}

func TestRunUnreadableScriptExitsTwo(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"--script", "/nonexistent/scenario.json"}, &buf)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "cannot read") {
		t.Fatalf("stderr must name the read failure:\n%s", buf.String())
	}
}

func TestRunUnexpectedArgumentExitsTwo(t *testing.T) {
	p := writeScript(t, `[{"text":"x"}]`)
	var buf bytes.Buffer
	if code := run([]string{"--script", p, "stray"}, &buf); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}
