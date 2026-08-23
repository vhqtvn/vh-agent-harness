// parse_test.go — the rule-file parser battery (TDD slice 1, red
// first). The parser is FAIL-CLOSED: unknown keys/sections, malformed
// lines, unreadable files, and semantically inexpressible rules are
// typed ParseErrors carrying the exact offending line (number + byte
// offset + verbatim text). Nothing is ever silently ignored except
// blank lines and whole-line `#` comments — the two documented shapes.
package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wantParseErr fails unless err is a *ParseError at the expected line
// whose message contains key.
func wantParseErr(t *testing.T, name, src string, line int, key string) {
	t.Helper()
	_, err := Parse("test.policy", []byte(src))
	if err == nil {
		t.Fatalf("%s: expected a parse error, got none (src=%q)", name, src)
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("%s: error must be a *ParseError, got %T: %v", name, err, err)
	}
	if pe.Line != line {
		t.Fatalf("%s: error line = %d, want %d (%v)", name, pe.Line, line, err)
	}
	if !strings.Contains(pe.Msg, key) {
		t.Fatalf("%s: error message %q must contain %q", name, pe.Msg, key)
	}
	if line > 0 {
		if pe.Byte < 0 {
			t.Fatalf("%s: line-bound error must carry a byte offset, got %d", name, pe.Byte)
		}
		if pe.Text == "" {
			t.Fatalf("%s: line-bound error must carry the offending line text", name)
		}
	}
}

func TestParseValidRuleFile(t *testing.T) {
	src := "# allow the read-only stuff\n" +
		"\n" +
		"[[allow]]\n" +
		"tool = \"read\"\n" +
		"path = \"docs/\"\n" +
		"\n" +
		"[[allow]]\n" +
		"tool = \"run_shell\"\n" +
		"argv0 = \"git\"\n" +
		"\n" +
		"[[allow]]\n" +
		"tool = \"echo\"\n" +
		"[[allow]]\n" +
		"tool = \"edit:*\"\n"

	p, err := Parse("test.policy", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rs := p.Rules()
	if len(rs) != 4 {
		t.Fatalf("rules = %d, want 4: %+v", len(rs), rs)
	}
	want := []Rule{
		{Tool: "read", Path: "docs/"},
		{Tool: "run_shell", Argv0: "git"},
		{Tool: "echo"},
		{Tool: "edit:*"},
	}
	for i, w := range want {
		if rs[i] != w {
			t.Fatalf("rule[%d] = %+v, want %+v", i, rs[i], w)
		}
	}
}

func TestParseCommentsBlankLinesAndCRLF(t *testing.T) {
	src := "# leading comment\r\n\r\n[[allow]]\r\n# inner comment\r\ntool = \"clock\"\r\n"
	p, err := Parse("test.policy", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Rules()) != 1 || p.Rules()[0].Tool != "clock" {
		t.Fatalf("rules = %+v, want one clock rule", p.Rules())
	}
}

func TestParseEmptyFileMeansEverythingAsks(t *testing.T) {
	p, err := Parse("test.policy", nil)
	if err != nil {
		t.Fatalf("an empty file is valid (zero rules, everything asks): %v", err)
	}
	if len(p.Rules()) != 0 {
		t.Fatalf("rules = %+v, want none", p.Rules())
	}
}

func TestParseAbsoluteAndBarePathPrefixes(t *testing.T) {
	p, err := Parse("t", []byte("[[allow]]\ntool = \"write\"\npath = \"/tmp/build/\"\n\n[[allow]]\ntool = \"write\"\npath = \"docs\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rs := p.Rules()
	if len(rs) != 2 || rs[0].Path != "/tmp/build/" || rs[1].Path != "docs" {
		t.Fatalf("rules = %+v", rs)
	}
}

func TestParseKeySpacingVariants(t *testing.T) {
	p, err := Parse("t", []byte("[[allow]]\ntool=\"read\"\npath =\"docs/\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Rules()[0]; got != (Rule{Tool: "read", Path: "docs/"}) {
		t.Fatalf("rule = %+v", got)
	}
}

func TestParseFailureTable(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
		key  string
	}{
		{"unknown key", "[[allow]]\ntool = \"read\"\nmode = \"auto\"\n", 3, "unknown key"},
		{"unknown section", "[[deny]]\ntool = \"read\"\n", 1, "unknown section"},
		{"single section", "[policy]\nverbose = true\n", 1, "unknown section"},
		{"malformed line no equals", "[[allow]]\ntool \"read\"\n", 2, "malformed"},
		{"unquoted value", "[[allow]]\ntool = read\n", 2, "malformed"},
		{"trailing junk after value", "[[allow]]\ntool = \"read\" # inline\n", 2, "malformed"},
		{"backslash in value", "[[allow]]\ntool = \"read\"\npath = \"a\\b\"\n", 3, "backslash"},
		{"embedded quote in value", "[[allow]]\ntool = \"re\"ad\"\n", 2, "malformed"},
		{"content outside table", "tool = \"read\"\n", 1, "outside"},
		{"missing tool key", "[[allow]]\npath = \"docs/\"\n", 1, "tool"},
		{"duplicate key", "[[allow]]\ntool = \"read\"\ntool = \"write\"\n", 3, "duplicate"},
		{"bare star tool", "[[allow]]\ntool = \"*\"\n", 2, "tool"},
		{"mid wildcard", "[[allow]]\ntool = \"re*d\"\n", 2, "tool"},
		{"glob star not final segment", "[[allow]]\ntool = \"a*b:*\"\n", 2, "tool"},
		{"empty tool value", "[[allow]]\ntool = \"\"\n", 2, "tool"},
		{"path on non-file tool", "[[allow]]\ntool = \"echo\"\npath = \"docs/\"\n", 3, "path"},
		{"path on run_shell", "[[allow]]\ntool = \"run_shell\"\npath = \"docs/\"\n", 3, "path"},
		{"argv0 on non-shell tool", "[[allow]]\ntool = \"read\"\nargv0 = \"git\"\n", 3, "argv0"},
		{"path traversal in rule", "[[allow]]\ntool = \"read\"\npath = \"../docs/\"\n", 3, "path"},
		{"dot segment in rule path", "[[allow]]\ntool = \"read\"\npath = \"./docs/\"\n", 3, "path"},
		{"empty segment in rule path", "[[allow]]\ntool = \"read\"\npath = \"docs//x\"\n", 3, "path"},
		{"empty path value", "[[allow]]\ntool = \"read\"\npath = \"\"\n", 3, "path"},
		{"backslash in path", "[[allow]]\ntool = \"read\"\npath = \"docs\\x\"\n", 3, "path"},
		{"argv0 with space", "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git push\"\n", 3, "argv0"},
		{"empty argv0", "[[allow]]\ntool = \"run_shell\"\nargv0 = \"\"\n", 3, "argv0"},
		{"empty rule table at EOF", "[[allow]]\n", 1, "tool"},
	}
	for _, c := range cases {
		wantParseErr(t, c.name, c.src, c.line, c.key)
	}
}

func TestParseErrorCarriesLineTextAndByteOffset(t *testing.T) {
	src := "[[allow]]\ntool = \"read\"\nwhoops = \"x\"\n"
	_, err := Parse("test.policy", []byte(src))
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %v", err)
	}
	if pe.Text != "whoops = \"x\"" {
		t.Fatalf("Text = %q, want the offending line verbatim", pe.Text)
	}
	// Byte offset must point at the START of line 3.
	want := len("[[allow]]\ntool = \"read\"\n")
	if pe.Byte != want {
		t.Fatalf("Byte = %d, want %d", pe.Byte, want)
	}
	if !strings.Contains(pe.Error(), "test.policy:3") {
		t.Fatalf("Error() must name file:line, got %q", pe.Error())
	}
}

func TestLoadFileUnreadableIsTypedParseError(t *testing.T) {
	dir := t.TempDir()

	// Missing file.
	_, err := LoadFile(filepath.Join(dir, "nope.policy"))
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("missing file: want *ParseError, got %T: %v", err, err)
	}
	if pe.Line != 0 {
		t.Fatalf("missing file: Line = %d, want 0 (not line-bound)", pe.Line)
	}

	// A directory is unreadable as a file — deterministic EISDIR on
	// every platform/user (unlike chmod 000 under root).
	_, err = LoadFile(dir)
	if !errors.As(err, &pe) {
		t.Fatalf("directory path: want *ParseError, got %T: %v", err, err)
	}
	if pe.Line != 0 {
		t.Fatalf("directory path: Line = %d, want 0", pe.Line)
	}
}

func TestLoadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.policy")
	if err := os.WriteFile(path, []byte("[[allow]]\ntool = \"clock\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(p.Rules()) != 1 || p.Rules()[0].Tool != "clock" {
		t.Fatalf("rules = %+v", p.Rules())
	}

	// Errors from a loaded file attribute THAT file's path and line.
	bad := filepath.Join(dir, "bad.policy")
	if err := os.WriteFile(bad, []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadFile(bad)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "bad.policy:1") {
		t.Fatalf("error must attribute file:line, got %q", err.Error())
	}
}
