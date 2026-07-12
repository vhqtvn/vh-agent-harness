package permconfig

import (
	"io/fs"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// TestLockstep_ConfigTransformCoreJS asserts that the Decision constants in the
// harness-owned support file (config-transform.core.mjs) match the Go Decision
// enum exactly. This catches drift: if someone changes the Go enum without
// updating the JS support file (or vice versa), the transform would produce
// decisions the Go validator rejects.
//
// This is the "option b" lockstep mitigation from the design decision: for v1
// we assert the JS type shape matches the Go validator's accepted shape via a
// test. Code-generation from Go is a deferred follow-up.
func TestLockstep_ConfigTransformCoreJS(t *testing.T) {
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	source, err := fs.ReadFile(sub, ".vh-agent-harness/config-transform.core.mjs")
	if err != nil {
		t.Fatalf("read config-transform.core.mjs: %v", err)
	}
	src := string(source)

	// Each Go Decision constant must appear in the JS Decision enum with the
	// matching key and value. If either side drifts, this fails.
	cases := []struct {
		jsKey string
		goVal Decision
	}{
		{"ALLOW", Allow},
		{"DENY", Deny},
		{"ASK", Ask},
	}
	for _, c := range cases {
		// Check the Go value is what we expect (sanity).
		expectedVal := string(c.goVal)
		if expectedVal != "allow" && expectedVal != "deny" && expectedVal != "ask" {
			t.Fatalf("Go enum drift: Decision(%q) is not allow/deny/ask", expectedVal)
		}
		// Check the JS source contains the exact pair: KEY: "value"
		needle := c.jsKey + `: "` + expectedVal + `"`
		if !strings.Contains(src, needle) {
			t.Errorf("config-transform.core.mjs: expected %q in Decision enum — the JS %s must match Go Decision %q", needle, c.jsKey, expectedVal)
		}
	}

	// Also assert the builder helpers return the correct decision values so a
	// transform importing { allow, deny, ask } from the core file gets valid
	// decisions the Go validator accepts.
	builders := []struct {
		fnName string
		goVal  Decision
	}{
		{"allow", Allow},
		{"deny", Deny},
		{"ask", Ask},
	}
	for _, b := range builders {
		// The builder returns { pattern, decision: Decision.KEY }. Check the
		// function exists and references the right Decision constant.
		needle := `Decision.` + strings.ToUpper(string(b.goVal))
		// Find the builder function and verify it references the correct Decision.
		fnSig := `function ` + b.fnName + `(`
		if !strings.Contains(src, fnSig) {
			t.Errorf("config-transform.core.mjs: expected builder %q — missing function signature %q", b.fnName, fnSig)
		}
		// The builder body should reference Decision.<UPPER>.
		fnIdx := strings.Index(src, fnSig)
		// Look in the ~200 chars after the function for the Decision reference.
		region := src[fnIdx:]
		if len(region) > 200 {
			region = region[:200]
		}
		if !strings.Contains(region, needle) {
			t.Errorf("config-transform.core.mjs: builder %q must reference %q near its definition", b.fnName, needle)
		}
	}
}
