// script_test.go — the fail-closed script loader: every malformed or
// ambiguous input is a usage error naming the file, and only the four
// documented step classes parse.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScript writes s to a temp file and returns its path.
func writeScript(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return p
}

func TestLoadScriptFailClosedTable(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string // substring of the required error
	}{
		{"not json at all", `{o`, "invalid JSON"},
		{"top-level object", `{"text":"hi"}`, "top-level"},
		{"top-level string", `"hi"`, "top-level"},
		{"empty array", `[]`, "at least one step"},
		{"step not an object", `["hi"]`, "step 1"},
		{"step with no class", `[{}]`, "no recognized class"},
		{"text and empty both set", `[{"text":"x","empty":true}]`, "exactly one"},
		{"text and fault both set", `[{"text":"x","fault":{"status":500}}]`, "exactly one"},
		{"tool_calls and empty both set", `[{"tool_calls":[{"id":"c","name":"n"}],"empty":true}]`, "exactly one"},
		{"unknown key", `[{"text":"x","surprise":1}]`, "unknown field"},
		{"empty text is not a text step", `[{"text":""}]`, "present but empty"},
		{"empty text plus empty true", `[{"text":"","empty":true}]`, "exactly one"},
		{"tool call missing id", `[{"tool_calls":[{"name":"echo"}]}]`, "tool_calls[0].id"},
		{"tool call missing name", `[{"tool_calls":[{"id":"c1"}]}]`, "tool_calls[0].name"},
		{"tool call args not an object", `[{"tool_calls":[{"id":"c1","name":"echo","args":"not-json{"}]}]`, "tool_calls[0].args"},
		{"tool call args a string", `[{"tool_calls":[{"id":"c1","name":"echo","args":"{}"}]}]`, "tool_calls[0].args"},
		{"tool_calls empty array", `[{"tool_calls":[]}]`, "no recognized class"},
		{"fault missing status", `[{"fault":{"body":"boom"}}]`, "fault.status"},
		{"fault status out of range low", `[{"fault":{"status":302}}]`, "fault.status"},
		{"fault status out of range high", `[{"fault":{"status":600}}]`, "fault.status"},
		{"fault retry_after_ms negative", `[{"fault":{"status":500,"retry_after_ms":-1}}]`, "retry_after_ms"},
		{"empty false alone is not a script", `{"empty":false}`, "top-level"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeScript(t, tc.json)
			_, err := LoadScript(p)
			if err == nil {
				t.Fatalf("LoadScript(%q) succeeded, want a fail-closed error containing %q", tc.json, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
			// The error must NAME the script file (operator-facing).
			if !strings.Contains(err.Error(), p) {
				t.Fatalf("error must name the script path %s: %q", p, err.Error())
			}
		})
	}
}

func TestLoadScriptUnreadable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.json")
	if _, err := LoadScript(missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing file must fail closed naming the path, got %v", err)
	}
	dir := t.TempDir() // a directory is unreadable-as-file
	if _, err := LoadScript(dir); err == nil {
		t.Fatal("unreadable (directory) script must fail closed")
	}
}

func TestLoadScriptValidSteps(t *testing.T) {
	p := writeScript(t, `[
		{"text": "hello there"},
		{"tool_calls": [{"id": "call-1", "name": "echo", "args": {"text": "hi"}}]},
		{"tool_calls": [{"id": "call-2", "name": "clock"}]},
		{"fault": {"status": 429, "body": "{\"error\":\"slow\"}", "retry_after_ms": 2000}},
		{"fault": {"status": 500}},
		{"empty": true}
	]`)
	steps, err := LoadScript(p)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	if len(steps) != 6 {
		t.Fatalf("got %d steps, want 6", len(steps))
	}
	if steps[0].Text != "hello there" {
		t.Fatalf("step 0 = %+v", steps[0])
	}
	if len(steps[1].ToolCalls) != 1 || steps[1].ToolCalls[0].Name != "echo" || steps[1].ToolCalls[0].ID != "call-1" {
		t.Fatalf("step 1 = %+v", steps[1])
	}
	if got := string(steps[1].ToolCalls[0].Args); got != `{"text":"hi"}` && got != `{"text": "hi"}` {
		t.Fatalf("step 1 args = %s (raw object bytes must survive verbatim)", got)
	}
	if steps[2].ToolCalls[0].Args != nil {
		t.Fatalf("absent args must stay nil, got %s", steps[2].ToolCalls[0].Args)
	}
	if steps[3].Fault == nil || steps[3].Fault.Status != 429 || steps[3].Fault.RetryAfterMs != 2000 || steps[3].Fault.Body != `{"error":"slow"}` {
		t.Fatalf("step 3 = %+v", steps[3])
	}
	if steps[4].Fault == nil || steps[4].Fault.Status != 500 || steps[4].Fault.Body != "" {
		t.Fatalf("step 4 = %+v", steps[4])
	}
	if !steps[5].Empty {
		t.Fatalf("step 5 = %+v, want empty", steps[5])
	}
}
