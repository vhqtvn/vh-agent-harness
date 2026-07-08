package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- isSensitiveKey ---------------------------------------------------------

func TestIsSensitiveKey(t *testing.T) {
	positive := []string{
		"apiKey", "API_KEY", "api-key", "apikey", "x-api-key",
		"token", "authToken", "ACCESS_TOKEN", "refresh_token",
		"secret", "clientSecret", "client_secret",
		"password", "PASSWORD", "passwd", "db_password",
		"credential", "credentials",
		"auth", "authorization", "bearer", "bearerToken",
		"private_key", "privateKey", "PRIVATE-KEY",
		"access_key", "accessKeyId", "aws_access_key_id",
		"client_secret", "oauth_client_secret",
	}
	for _, k := range positive {
		if !isSensitiveKey(k) {
			t.Errorf("expected %q to be sensitive", k)
		}
	}
	negative := []string{
		"name", "path", "timestamp", "id", "status", "createdAt",
		"updatedAt", "version", "slug", "owner", "type", "role",
		"description", "note", "label", "title", "content",
		"endpoint", "region", "model", "provider", "enabled",
	}
	for _, k := range negative {
		if isSensitiveKey(k) {
			t.Errorf("expected %q to be NOT sensitive (false positive)", k)
		}
	}
}

// "auth" is a substring match by design; document the known false positive so a
// future change to the matcher is deliberate.
func TestIsSensitiveKeyKnownFalsePositiveAuthor(t *testing.T) {
	// "auth" is listed as a sensitive substring per spec, so "author" matches.
	// This is an accepted, safety-leaning tradeoff (see diagnostics.go).
	if !isSensitiveKey("author") {
		t.Error("expected 'author' to match 'auth' substring (known false positive)")
	}
}

// --- redactStringValue ------------------------------------------------------

func TestRedactStringValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "***REDACTED(3chars)***"},
		{"sk-1234567890", "***REDACTED(13chars)***"},
		{"héllo", "***REDACTED(5chars)***"}, // rune count, not byte count
	}
	for _, c := range cases {
		if got := redactStringValue(c.in); got != c.want {
			t.Errorf("redactStringValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- redactValue via redactJSON --------------------------------------------

func mustRedactJSON(t *testing.T, in string, stats *RedactionStats) string {
	t.Helper()
	out, err := redactJSON([]byte(in), stats)
	if err != nil {
		t.Fatalf("redactJSON: %v", err)
	}
	return string(out)
}

func decodeJSON(t *testing.T, b string) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(b), &v); err != nil {
		t.Fatalf("decode redacted json: %v\ninput: %s", err, b)
	}
	return v
}

func TestRedactJSONFieldName(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{
		"apiKey": "sk-live-1234567890",
		"authToken": "tok_abc",
		"password": "hunter2",
		"client_secret": "shh",
		"name": "demo",
		"count": 42
	}`, &stats)

	v := decodeJSON(t, got)
	if v["apiKey"] != "***REDACTED(18chars)***" {
		t.Errorf("apiKey not redacted: %v", v["apiKey"])
	}
	if v["authToken"] != "***REDACTED(7chars)***" {
		t.Errorf("authToken not redacted: %v", v["authToken"])
	}
	if v["password"] != "***REDACTED(7chars)***" {
		t.Errorf("password not redacted: %v", v["password"])
	}
	if v["client_secret"] != "***REDACTED(3chars)***" {
		t.Errorf("client_secret not redacted: %v", v["client_secret"])
	}
	// Non-sensitive preserved.
	if v["name"] != "demo" {
		t.Errorf("name changed: %v", v["name"])
	}
	if v["count"] != float64(42) {
		t.Errorf("count changed: %v", v["count"])
	}
	if stats.FieldName != 4 {
		t.Errorf("FieldName count = %d, want 4", stats.FieldName)
	}
}

func TestRedactJSONNestedAndPreservesStructure(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{
		"connection": {"dsn": "postgres://u:secret@host/db", "host": "host"},
		"auth": {"type": "bearer", "token": "xyz"},
		"plugins": [{"name": "p", "apiKey": "k"}]
	}`, &stats)
	v := decodeJSON(t, got)

	// "auth" is a sensitive key with a container value: per the security fix
	// (G2) the whole subtree is suspect, so BOTH children are redacted — secret
	// material must not hide behind a benign child key name like "type".
	auth := v["auth"].(map[string]interface{})
	if authType, ok := auth["type"].(string); !ok || authType == "bearer" || !strings.Contains(authType, "REDACTED") {
		t.Errorf("auth.type should be redacted under sensitive container, got: %v", auth["type"])
	}
	if auth["token"] != "***REDACTED(3chars)***" {
		t.Errorf("auth.token not redacted: %v", auth["token"])
	}

	// nested plugin apiKey redacted.
	plugins := v["plugins"].([]interface{})
	p0 := plugins[0].(map[string]interface{})
	if p0["name"] != "p" {
		t.Errorf("plugin name changed: %v", p0["name"])
	}
	if p0["apiKey"] != "***REDACTED(1chars)***" {
		t.Errorf("plugin apiKey not redacted: %v", p0["apiKey"])
	}

	// connection.dsn is value-pattern redacted (password scrubbed, host kept).
	conn := v["connection"].(map[string]interface{})
	dsn := conn["dsn"].(string)
	if !strings.Contains(dsn, "u:***@host") {
		t.Errorf("connection dsn password not scrubbed: %q", dsn)
	}
	if conn["host"] != "host" {
		t.Errorf("connection.host changed: %v", conn["host"])
	}
}

func TestRedactJSONSectionRedaction(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{
		"env": {"DATABASE_URL": "postgres://a:b@h/d", "DEBUG": "1", "COUNT": "3"},
		"secrets": {"a": "x"},
		"models": {"openai": {"apiKey": "sk-x"}, "name": "gpt"},
		"config": {"keep": "me", "nested": {"models": {"should": "survive"}}}
	}`, &stats)
	v := decodeJSON(t, got)

	// env section: all string values redacted.
	env := v["env"].(map[string]interface{})
	if env["DATABASE_URL"] != "***REDACTED(18chars)***" {
		t.Errorf("env.DATABASE_URL not redacted: %v", env["DATABASE_URL"])
	}
	if env["DEBUG"] != "***REDACTED(1chars)***" {
		t.Errorf("env.DEBUG not redacted: %v", env["DEBUG"])
	}

	// top-level models section: all string values redacted.
	models := v["models"].(map[string]interface{})
	if models["openai"].(map[string]interface{})["apiKey"] != "***REDACTED(4chars)***" {
		t.Errorf("models.openai.apiKey not redacted: %v", models)
	}

	// config.keep preserved; nested "models" (depth>=1) is NOT a section — data
	// preserved (its values are not secrets).
	cfg := v["config"].(map[string]interface{})
	if cfg["keep"] != "me" {
		t.Errorf("config.keep changed: %v", cfg["keep"])
	}
	nested := cfg["nested"].(map[string]interface{})
	nestedModels := nested["models"].(map[string]interface{})
	if nestedModels["should"] != "survive" {
		t.Errorf("nested models data-model should survive: %v", nestedModels)
	}

	if stats.Section == 0 {
		t.Error("expected section redaction count > 0")
	}
}

func TestRedactJSONValuePatterns(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{
		"bearer": "Bearer abc.def-ghi",
		"aws": "AKIAIOSFODNN7EXAMPLE",
		"url": "https://user:p4ssw0rd@example.com/path",
		"plain": "not a secret"
	}`, &stats)
	v := decodeJSON(t, got)
	if v["bearer"] != "***REDACTED(18chars)***" {
		t.Errorf("bearer not redacted: %v", v["bearer"])
	}
	if v["aws"] != "***REDACTED***" {
		t.Errorf("aws key not redacted: %v", v["aws"])
	}
	url := v["url"].(string)
	if !strings.Contains(url, "user:***@example.com") {
		t.Errorf("url password not scrubbed: %q", url)
	}
	if v["plain"] != "not a secret" {
		t.Errorf("plain changed: %v", v["plain"])
	}
}

// --- G2: sensitive key with a nested container must not leak children -------

// Regression: a sensitive field name whose value is an object used to leak
// child strings, because redactSensitiveField recursed and judged children by
// their OWN (benign) key names. {"auth":{"header":"Basic dXNlcjpwYXNz"}}
// survived: "header" is not sensitive and "Basic dXNlcjpwYXNz" matches no
// value pattern.
func TestRedactJSONSensitiveNestedObjectAuthHeaderNoLeak(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{"auth":{"header":"Basic dXNlcjpwYXNz"}}`, &stats)
	v := decodeJSON(t, got)
	auth := v["auth"].(map[string]interface{})
	header, ok := auth["header"].(string)
	if !ok {
		t.Fatalf("auth.header missing or non-string: %v", auth["header"])
	}
	if header == "Basic dXNlcjpwYXNz" {
		t.Errorf("nested secret under sensitive key 'auth' leaked verbatim: %q", header)
	}
	if !strings.Contains(header, "REDACTED") {
		t.Errorf("auth.header should be redacted, got: %q", header)
	}
	if stats.FieldName == 0 {
		t.Error("expected field-name redaction count > 0 for nested sensitive container")
	}
}

// Regression: a sensitive key whose child holds an arbitrary string with no
// recognizable secret shape must still be redacted.
func TestRedactJSONSensitiveNestedNonPatternValueNoLeak(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{"secret":{"value":"not-pattern-shaped"}}`, &stats)
	v := decodeJSON(t, got)
	sec := v["secret"].(map[string]interface{})
	val, ok := sec["value"].(string)
	if !ok {
		t.Fatalf("secret.value missing or non-string: %v", sec["value"])
	}
	if val == "not-pattern-shaped" {
		t.Errorf("nested non-pattern secret leaked verbatim: %q", val)
	}
	if !strings.Contains(val, "REDACTED") {
		t.Errorf("secret.value should be redacted, got: %q", val)
	}
}

// Regression: the nested-container rule also covers arrays under a sensitive
// key — every string element / child string must be redacted.
func TestRedactJSONSensitiveNestedArrayNoLeak(t *testing.T) {
	var stats RedactionStats
	got := mustRedactJSON(t, `{"tokens":[{"raw":"abc-123"},{"raw":"def-456"}],"password":{"nested":{"deep":"ssh"}}}`, &stats)
	v := decodeJSON(t, got)
	toks := v["tokens"].([]interface{})
	for i, item := range toks {
		m := item.(map[string]interface{})
		if m["raw"] == "abc-123" || m["raw"] == "def-456" {
			t.Errorf("tokens[%d].raw leaked under sensitive container: %v", i, m["raw"])
		}
	}
	pw := v["password"].(map[string]interface{})
	deep := pw["nested"].(map[string]interface{})
	if deep["deep"] == "ssh" {
		t.Errorf("password.nested.deep leaked under sensitive container: %v", deep["deep"])
	}
}

func TestRedactJSONPreservesNumbers(t *testing.T) {
	// Large integers must survive the round trip (json.Number path).
	var stats RedactionStats
	in := `{"id": 9223372036854775807, "price": 19.95, "ok": true, "nothing": null, "name": "x"}`
	out, err := redactJSON([]byte(in), &stats)
	if err != nil {
		t.Fatalf("redactJSON: %v", err)
	}
	v := decodeJSON(t, string(out))
	if v["id"] != float64(9223372036854775807) {
		t.Errorf("large int lost precision: %v", v["id"])
	}
	if v["price"] != 19.95 {
		t.Errorf("float changed: %v", v["price"])
	}
	if v["ok"] != true || v["nothing"] != nil {
		t.Errorf("bool/null changed: %v %v", v["ok"], v["nothing"])
	}
}

func TestRedactJSONMalformedFallsBack(t *testing.T) {
	// Not valid JSON -> redactJSON returns an error; processFileContent falls
	// back to text redaction.
	_, err := redactJSON([]byte(`{not json`), &RedactionStats{})
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
	var stats RedactionStats
	out, class := processFileContent("x.json", []byte(`token: abc123\nname: ok`), &stats)
	if class != classUnparsed {
		t.Errorf("expected classUnparsed, got %s", class)
	}
	if !bytes.Contains(out, []byte("REDACTED")) {
		t.Errorf("text fallback should redact sensitive KV: %s", out)
	}
}

// --- redactYAML -------------------------------------------------------------

func TestRedactYAML(t *testing.T) {
	var stats RedactionStats
	in := []byte("name: demo\napiKey: sk-1234567890\npassword: hunter2\nenv:\n  FOO: bar\n  DEBUG: \"1\"\nmodels:\n  openai:\n    apiKey: sk-x\nnested:\n  models:\n    data: kept\n")
	out, err := redactYAML(in, &stats)
	if err != nil {
		t.Fatalf("redactYAML: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "name: demo") {
		t.Errorf("name should survive:\n%s", s)
	}
	if !strings.Contains(s, "***REDACTED(13chars)***") {
		t.Errorf("top-level apiKey not redacted:\n%s", s)
	}
	if !strings.Contains(s, "***REDACTED(7chars)***") {
		t.Errorf("password not redacted:\n%s", s)
	}
	// env section values redacted (yaml.v3 may quote the placeholder).
	if !strings.Contains(s, "FOO: ***REDACTED(3chars)***") &&
		!strings.Contains(s, `FOO: '***REDACTED(3chars)***'`) &&
		!strings.Contains(s, `FOO: "***REDACTED(3chars)***"`) {
		t.Errorf("env.FOO not redacted:\n%s", s)
	}
	// top-level models section: nested apiKey redacted.
	if strings.Contains(s, "sk-x") {
		t.Errorf("models.openai.apiKey (sk-x) should be redacted:\n%s", s)
	}
	// nested models data preserved.
	if !strings.Contains(s, "kept") {
		t.Errorf("nested models data should survive:\n%s", s)
	}
	if stats.FieldName == 0 || stats.Section == 0 {
		t.Errorf("expected field-name and section redaction, got %+v", stats)
	}
}

// --- redactText (line-based) ------------------------------------------------

func TestRedactTextEnvFile(t *testing.T) {
	var stats RedactionStats
	in := []byte("API_KEY=sk-1234567890\nTOKEN=tok_xyz\nDATABASE_URL=postgres://u:pw@host/db\nDEBUG=1\n# comment\nNAME=demo\n")
	out := redactText(in, &stats)
	s := string(out)
	if strings.Contains(s, "sk-1234567890") {
		t.Errorf("API_KEY value leaked:\n%s", s)
	}
	if strings.Contains(s, "tok_xyz") {
		t.Errorf("TOKEN value leaked:\n%s", s)
	}
	if strings.Contains(s, ":pw@") {
		t.Errorf("connection password leaked:\n%s", s)
	}
	if !strings.Contains(s, "DEBUG=1") || !strings.Contains(s, "NAME=demo") {
		t.Errorf("non-sensitive KV should survive:\n%s", s)
	}
	if !strings.Contains(s, "# comment") {
		t.Errorf("comment should survive:\n%s", s)
	}
	if stats.FieldName < 2 {
		t.Errorf("expected >=2 field-name redactions, got %d", stats.FieldName)
	}
}

func TestRedactTextBearerInMarkdown(t *testing.T) {
	var stats RedactionStats
	in := []byte("# Notes\nThe header was Authorization: Bearer abc123def456.\nAWS key AKIAIOSFODNN7EXAMPLE leaked.\n")
	out := redactText(in, &stats)
	s := string(out)
	if strings.Contains(s, "abc123def456") {
		t.Errorf("bearer token leaked:\n%s", s)
	}
	if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("aws key leaked:\n%s", s)
	}
	if !strings.Contains(s, "# Notes") {
		t.Errorf("markdown heading should survive:\n%s", s)
	}
	if stats.ValuePattern < 2 {
		t.Errorf("expected >=2 value-pattern redactions, got %d", stats.ValuePattern)
	}
}

// --- redactJSONLines --------------------------------------------------------

func TestRedactJSONLines(t *testing.T) {
	var stats RedactionStats
	in := []byte("{\"type\":\"event\",\"apiKey\":\"sk-1\",\"ts\":1}\n{\"type\":\"event\",\"token\":\"t1\",\"ts\":2}\n\nnot-json-line\n")
	out, ok := redactJSONLines(in, &stats)
	if !ok {
		t.Fatal("expected at least one line parsed as JSON")
	}
	s := string(out)
	if strings.Contains(s, "sk-1") || strings.Contains(s, "\"t1\"") {
		t.Errorf("secret leaked in jsonl:\n%s", s)
	}
	if !strings.Contains(s, "\"type\":\"event\"") {
		t.Errorf("non-sensitive field should survive:\n%s", s)
	}
	if !strings.Contains(s, "not-json-line") {
		t.Errorf("non-json line should survive:\n%s", s)
	}
}

// Regression (G3): a non-JSON line in a .jsonl/.ndjson file used to receive
// ONLY key=value field-name redaction, so a secret-shaped value that did not
// sit in a sensitive key=value pair leaked unredacted — even though the file
// was labeled structured_jsonl in the manifest, giving false confidence.
// Here line 1 is valid JSON, line 2 is a non-JSON log line carrying a Bearer
// token in free text. The Bearer token must be scrubbed.
func TestRedactJSONLinesNonJSONBearerScrubbed(t *testing.T) {
	var stats RedactionStats
	in := []byte("{\"type\":\"event\",\"ts\":1}\nERROR auth failed: Bearer eyJabc.def-ghi\n")
	out, ok := redactJSONLines(in, &stats)
	if !ok {
		t.Fatal("expected at least one line parsed as JSON")
	}
	s := string(out)
	if strings.Contains(s, "eyJabc.def-ghi") {
		t.Errorf("bearer token leaked in non-JSON jsonl line:\n%s", s)
	}
	if !strings.Contains(s, "REDACTED") {
		t.Errorf("expected bearer token to be redacted:\n%s", s)
	}
	if stats.ValuePattern == 0 {
		t.Error("expected value-pattern redaction in jsonl non-JSON line")
	}
	// The parsed JSON line must survive structurally and not be double-scanned.
	if !strings.Contains(s, "\"type\":\"event\"") {
		t.Errorf("parsed JSON line corrupted:\n%s", s)
	}
}

// Regression (G3) companion: AKIA and connection-string passwords on non-JSON
// lines are also scrubbed by the same per-line value-pattern pass.
func TestRedactJSONLinesNonJSONAKIAAndConnStrScrubbed(t *testing.T) {
	var stats RedactionStats
	in := []byte("{\"ts\":1}\naws key AKIAIOSFODNN7EXAMPLE in log\ndsn=postgres://u:secretpw@host/db oops\n")
	out, ok := redactJSONLines(in, &stats)
	if !ok {
		t.Fatal("expected at least one line parsed as JSON")
	}
	s := string(out)
	if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("aws key leaked in non-JSON jsonl line:\n%s", s)
	}
	if strings.Contains(s, ":secretpw@") {
		t.Errorf("connection password leaked in non-JSON jsonl line:\n%s", s)
	}
	if stats.ValuePattern < 2 {
		t.Errorf("expected >=2 value-pattern redactions, got %d", stats.ValuePattern)
	}
}

// --- processFileContent dispatch -------------------------------------------

func TestProcessFileContentBinary(t *testing.T) {
	binary := []byte{0x89, 0x50, 0x4E, 0x00, 0x47, 0x0D} // contains NUL
	var stats RedactionStats
	out, class := processFileContent("img.png", binary, &stats)
	if class != classBinary {
		t.Errorf("expected classBinary, got %s", class)
	}
	if !bytes.Equal(out, binary) {
		t.Error("binary content should pass through unchanged")
	}
}

func TestProcessFileContentDispatchByExt(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want fileClass
	}{
		{"a.json", []byte(`{"name":"x"}`), classJSON},
		{"a.yaml", []byte("name: x\n"), classYAML},
		{"a.yml", []byte("name: x\n"), classYAML},
		{"a.jsonl", []byte("{\"name\":\"x\"}\n"), classJSONL},
		{"a.md", []byte("# hi\n"), classText},
		{".env", []byte("NAME=x\n"), classText},
	}
	for _, c := range cases {
		var stats RedactionStats
		_, class := processFileContent(c.name, c.data, &stats)
		if class != c.want {
			t.Errorf("processFileContent(%q) class = %s, want %s", c.name, class, c.want)
		}
	}
}

// --- end-to-end bundle ------------------------------------------------------

func TestCollectBundleFilesRedactsAndExcludes(t *testing.T) {
	root := t.TempDir()
	seed := map[string]string{
		".opencode/state/sessions/abc/memory.json": `{"name":"abc","apiKey":"sk-1234567890","token":"tok_xyz","ts":1}`,
		".opencode/state/workstreams/w/brief.md":   "# brief\nBearer leakedtoken123\n",
		".local/coordinator/tasks/001.json":        `{"id":"1","password":"hunter2","status":"open"}`,
		".local/config/operator.json":              `{"name":"op","secrets":{"vault":"vvv"},"api_key":"kkk"}`,
		"docs/checkpoints/2026-01-01-snap.md":      "# snap\nauthor: jane\n",
		// excluded by segment rules — these sit UNDER a source root but match an
		// excluded segment, so collectBundleFiles skips them and lists them:
		".opencode/state/tmp/junk.txt":              "tmp-segment\n",
		".opencode/state/node_modules/pkg/index.js": "module.exports={}\n",
	}
	for rel, content := range seed {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	files, excluded, stats, err := collectBundleFiles(root)
	if err != nil {
		t.Fatalf("collectBundleFiles: %v", err)
	}

	// No secret value leaks in any redacted file content.
	leaks := []string{"sk-1234567890", "tok_xyz", "hunter2", "vvv", "kkk", "leakedtoken123"}
	for _, f := range files {
		for _, leak := range leaks {
			if bytes.Contains(f.content, []byte(leak)) {
				t.Errorf("secret %q leaked in %s:\n%s", leak, f.rel, f.content)
			}
		}
	}

	// Excluded paths include node_modules and tmp segments nested in a source.
	var sawTmp, sawNodeModules bool
	for _, e := range excluded {
		if strings.HasPrefix(e, ".opencode/state/tmp/") {
			sawTmp = true
		}
		if strings.HasPrefix(e, ".opencode/state/node_modules/") {
			sawNodeModules = true
		}
	}
	if !sawTmp {
		t.Error("expected .opencode/state/tmp/ entry to be excluded")
	}
	if !sawNodeModules {
		t.Error("expected .opencode/state/node_modules/ entry to be excluded")
	}

	// "author: jane" — 'auth' substring → redacted (known false positive).
	var authorFile *bundleFile
	for i := range files {
		if files[i].rel == "docs/checkpoints/2026-01-01-snap.md" {
			authorFile = &files[i]
		}
	}
	if authorFile == nil {
		t.Fatal("checkpoint not collected")
	}
	if bytes.Contains(authorFile.content, []byte("jane")) {
		t.Errorf("author value should be redacted (auth substring), got:\n%s", authorFile.content)
	}

	// Redaction stats reflect activity across categories.
	if stats.FieldName == 0 {
		t.Error("expected field-name redactions")
	}
	if stats.ValuePattern == 0 {
		t.Error("expected value-pattern redactions (bearer/connection)")
	}
}

func TestDiagnosticsExportEndToEnd(t *testing.T) {
	root := t.TempDir()
	seed := map[string]string{
		".opencode/state/s.json": `{"name":"x","apiKey":"sk-1234567890abcdefg","ok":true}`,
		".local/config/c.json":   `{"token":"tok_abcdef","env":{"SECRET":"shh"}}`,
		"docs/checkpoints/c.md":  "# c\nname: demo\n",
	}
	for rel, content := range seed {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// Drive the command in-process via cobra against this root.
	cmd, buf := newOutCmd()
	cmd.SetArgs([]string{"--output", filepath.Join("tmp", "diag.tar.gz")})

	// We cannot easily override os.Getwd for the cobra RunE through the test
	// helper's *cobra.Command, so call the collector + writer directly against
	// root and assert the archive contents.
	files, excluded, stats, err := collectBundleFiles(root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	outPath := filepath.Join(root, "tmp", "diag.tar.gz")
	m := buildManifest(root, outPath, files, excluded, stats, false, 0)
	if err := writeArchive(root, outPath, files, m); err != nil {
		t.Fatalf("writeArchive: %v", err)
	}

	// Archive lives under repo tmp/.
	if !strings.HasPrefix(outPath, filepath.Join(root, "tmp")) {
		t.Errorf("archive outside repo tmp/: %s", outPath)
	}

	// Open archive and inspect.
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tr := tar.NewReader(gz)
	gotNames := map[string][]byte{}
	for {
		hdr, rerr := tr.Next()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			t.Fatalf("tar next: %v", rerr)
		}
		b, _ := io.ReadAll(tr)
		gotNames[hdr.Name] = b
	}
	// manifest.json present.
	if _, ok := gotNames["manifest.json"]; !ok {
		t.Error("manifest.json missing from archive")
		var names []string
		for n := range gotNames {
			names = append(names, n)
		}
		t.Logf("archive entries: %v", names)
	} else {
		var man bundleManifest
		if err := json.Unmarshal(gotNames["manifest.json"], &man); err != nil {
			t.Fatalf("unmarshal manifest: %v", err)
		}
		if man.Tool == "" || man.Version == "" {
			t.Errorf("manifest missing tool/version: %+v", man)
		}
		if man.Redaction.FieldName == 0 {
			t.Error("manifest should report field-name redactions")
		}
	}
	// bundled state files present and redacted.
	state, ok := gotNames[".opencode/state/s.json"]
	if !ok {
		t.Fatal("state json missing from archive")
	}
	if bytes.Contains(state, []byte("sk-1234567890abcdefg")) {
		t.Errorf("apiKey leaked in archive:\n%s", state)
	}
	// Non-sensitive fields survive (decode rather than fragile substring match
	// since MarshalIndent adds spaces).
	var stateObj map[string]interface{}
	if err := json.Unmarshal(state, &stateObj); err != nil {
		t.Fatalf("decode state json: %v\n%s", err, state)
	}
	if stateObj["name"] != "x" {
		t.Errorf("non-sensitive name field lost: %v", stateObj["name"])
	}
	if stateObj["ok"] != true {
		t.Errorf("non-sensitive ok field lost: %v", stateObj["ok"])
	}

	// keep buf use to avoid unused warnings on newOutCmd plumbing.
	_ = cmd
	_ = buf
}

// --- resolveOutputPath ------------------------------------------------------

func TestResolveOutputPathRejectsOutsideRepo(t *testing.T) {
	root := t.TempDir()
	tmpDir := os.TempDir() // system temp, outside root
	_, err := resolveOutputPath(root, filepath.Join(tmpDir, "x.tar.gz"))
	if err == nil {
		t.Error("expected error for output outside repo root")
	}
}

func TestResolveOutputPathDefaultsToTmp(t *testing.T) {
	root := t.TempDir()
	p, err := resolveOutputPath(root, "")
	if err != nil {
		t.Fatalf("resolveOutputPath: %v", err)
	}
	if !strings.HasSuffix(p, ".tar.gz") {
		t.Errorf("expected .tar.gz suffix: %s", p)
	}
	if !strings.Contains(filepath.ToSlash(p), "/tmp/diagnostics-") {
		t.Errorf("expected default under tmp/diagnostics-: %s", p)
	}
}

// Regression (G1): resolveOutputPath used to do only a LEXICAL containment
// check (filepath.Abs + filepath.Rel) without resolving symlinks. An in-repo
// symlinked directory pointing outside the repo could route --output past the
// repo boundary. The fix resolves symlinks in the existing path prefix before
// the containment check, so the symlinked escape must now be rejected.
func TestResolveOutputPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir() // lives OUTSIDE the repo root
	// In-repo symlink "leak" -> outside. The lexical path root/leak/... looks
	// in-repo, but its real target escapes.
	link := filepath.Join(root, "leak")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unsupported on this filesystem: %v", err)
	}

	// --output routed through the in-repo symlink that escapes the repo: must
	// be rejected with a non-nil error before any write happens.
	flag := filepath.Join(link, "diag.tar.gz") // absolute, via symlink
	if _, err := resolveOutputPath(root, flag); err == nil {
		t.Fatal("expected error for --output escaping repo via symlink, got nil")
	}
	// And nothing must land in the outside target directory.
	if _, err := os.Stat(filepath.Join(outside, "diag.tar.gz")); err == nil {
		t.Error("archive was written outside the repo via symlinked --output")
	}

	// Positive control: a normal in-repo relative path (no symlink) is still
	// accepted and stays inside the resolved repo root — the gate is not simply
	// refusing everything.
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root: %v", err)
	}
	got, err := resolveOutputPath(root, filepath.FromSlash("tmp/diag.tar.gz"))
	if err != nil {
		t.Fatalf("expected in-repo path to resolve, got error: %v", err)
	}
	rel, err := filepath.Rel(rootResolved, got)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Errorf("resolved in-repo path escaped root: %s", got)
	}
}

// Regression (F-B1): writeArchive used to trust resolveOutputPath's pre-write
// containment check and then call os.MkdirAll(parent) + os.Create(outPath) on
// the SAME literal path without re-resolving. When the output parent did not
// exist yet, resolveSymlinkSafe treated the missing tail as a literal suffix
// (non-existent components cannot be symlinks), so resolveOutputPath passed. An
// attacker with concurrent write access who swapped that parent for a symlink
// pointing outside the repo — between the resolve check and os.Create — routed
// the secret-bearing archive out of the repo via the followed symlink.
//
// This test reproduces the swap deterministically: it runs resolveOutputPath
// while the parent is still absent (mirroring the real pre-write check), then
// swaps the parent for an escaping symlink before invoking writeArchive. The
// fix makes writeArchive re-resolve the materialized parent and re-check
// containment immediately before opening the file, so the swap must be refused.
//
// A narrower kernel-level TOCTOU (swap between openArchiveForWrite's final
// EvalSymlinks and os.Create) is not addressable in pure Go without OS
// openat/O_NOFOLLOW flags and is out of scope; this test covers the resolve→
// create window the fix targets, which is the window an unprivileged attacker
// actually controls.
func TestWriteArchiveRejectsSymlinkSwapParent(t *testing.T) {
	root := t.TempDir()
	outsideTarget := t.TempDir() // lives OUTSIDE the repo root

	// outPath's parent (root/tmp/sub) does NOT exist yet, mirroring the real
	// flow where resolveOutputPath runs before writeArchive creates anything.
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	outPath := filepath.Join(root, "tmp", "sub", "diag.tar.gz")

	// Pre-write check: passes because the missing "sub" tail is treated
	// literally and the lexical path stays inside the repo.
	resolved, err := resolveOutputPath(root, outPath)
	if err != nil {
		t.Fatalf("resolveOutputPath before swap: %v", err)
	}

	// TOCTOU swap: replace the still-absent parent with a symlink that
	// escapes the repo. The literal path is unchanged, but its real target is
	// now outside.
	swappedParent := filepath.Join(root, "tmp", "sub")
	if err := os.Symlink(outsideTarget, swappedParent); err != nil {
		t.Skipf("symlink creation unsupported on this filesystem: %v", err)
	}

	m := bundleManifest{Tool: "vh-agent-harness", Version: "test"}
	files := []bundleFile{{rel: "a.txt", content: []byte("alpha")}}

	// writeArchive must refuse: the materialized parent resolves outside the
	// repo and the post-MkdirAll re-check catches it.
	werr := writeArchive(root, resolved, files, m)
	if werr == nil {
		t.Fatal("expected writeArchive to refuse the swapped (escaping) parent, got nil error")
	}

	// No archive may land outside the repo.
	if _, serr := os.Stat(filepath.Join(outsideTarget, "diag.tar.gz")); serr == nil {
		t.Errorf("archive was written outside the repo via symlink-swapped parent: %s",
			filepath.Join(outsideTarget, "diag.tar.gz"))
	}
}

// Regression (c-F1, round 4 of the symlink-containment review): the F-B1 fix
// re-resolved and re-checked the PARENT after MkdirAll, but writeArchive then
// called os.Create on the joined parent+base path, which FOLLOWS a symlink
// planted at the archive filename itself. Attack chain: resolveOutputPath
// passes (the archive file does not exist yet, so resolveSymlinkSafe re-attaches
// a literal tail and the containment check passes); an attacker plants
// root/tmp/diag.tar.gz as a symlink to an outside target; writeArchive
// re-resolves the parent (inside the repo, passes containment), then
// os.Create(resolvedParent/base) follows the final symlink and writes the
// secret-bearing archive outside the repo. This needs no kernel race — same
// threat model as the original G1.
//
// The fix stages the archive in a temp file with an unpredictable O_EXCL name
// inside the contained parent, then os.Rename's it onto the final path. On
// Linux rename(2) does NOT follow a symlink on the destination: it atomically
// replaces the destination directory entry itself. So the planted symlink at
// base is REPLACED by the regular file, the outside target is untouched, and the
// write stays in-repo. The symlink escape is closed with no TOCTOU window.
//
// This test asserts the post-fix semantics directly: writeArchive SUCCEEDS, the
// archive lands at root/tmp/diag.tar.gz as a REGULAR FILE (not a symlink), and
// the outside symlink target is NOT created. Reverting writeArchive to the old
// os.Create(joined path) behavior makes the outside target get created (test
// fails), confirming the test has teeth — see the verification note in the
// round-4 closeout.
func TestWriteArchiveRejectsFinalComponentSymlink(t *testing.T) {
	root := t.TempDir()
	outsideTarget := t.TempDir() // lives OUTSIDE the repo root

	// The tmp parent exists, so the parent-swap vector (F-B1) is NOT triggered
	// — only the final-component vector is exercised here.
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	outPath := filepath.Join(root, "tmp", "diag.tar.gz")

	// Pre-write check: passes because diag.tar.gz does not exist yet, so
	// resolveSymlinkSafe treats it as a literal tail and the lexical path stays
	// inside the repo.
	resolved, err := resolveOutputPath(root, outPath)
	if err != nil {
		t.Fatalf("resolveOutputPath before symlink plant: %v", err)
	}

	// Plant the final-component symlink: root/tmp/diag.tar.gz -> outside.
	if err := os.Symlink(filepath.Join(outsideTarget, "diag.tar.gz"), outPath); err != nil {
		t.Skipf("symlink creation unsupported on this filesystem: %v", err)
	}

	m := bundleManifest{Tool: "vh-agent-harness", Version: "test"}
	files := []bundleFile{{rel: "a.txt", content: []byte("alpha")}}

	// With the temp-file + os.Rename fix, writeArchive SUCCEEDS: the temp is
	// staged inside the contained parent and the rename replaces the symlink
	// directory entry with a regular file. The outside symlink target is never
	// evaluated.
	werr := writeArchive(root, resolved, files, m)
	if werr != nil {
		t.Fatalf("writeArchive with final-component symlink planted: unexpected error: %v", werr)
	}

	// The archive must now be a REGULAR FILE at outPath (the symlink was
	// replaced), not a symlink to the outside target.
	fi, err := os.Lstat(outPath)
	if err != nil {
		t.Fatalf("lstat archive after write: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		real, _ := os.Readlink(outPath)
		t.Errorf("archive at %s is still a symlink (-> %s); expected the symlink to be replaced by a regular file", outPath, real)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("archive at %s is not a regular file: mode=%v", outPath, fi.Mode())
	}

	// The secret-bearing archive must NOT have escaped: the outside symlink
	// target must not exist.
	if _, serr := os.Stat(filepath.Join(outsideTarget, "diag.tar.gz")); serr == nil {
		t.Errorf("archive escaped the repo via the final-component symlink: %s was created",
			filepath.Join(outsideTarget, "diag.tar.gz"))
	}
}

// --- shouldExcludePath ------------------------------------------------------

func TestShouldExcludePath(t *testing.T) {
	yes := []string{"refs/x", ".git/config", "node_modules/y", "tmp/z", ".opencode/state/tmp/w"}
	for _, p := range yes {
		if !shouldExcludePath(p) {
			t.Errorf("expected %q to be excluded", p)
		}
	}
	no := []string{".opencode/state/s.json", ".local/coordinator/tasks/1.json", "docs/checkpoints/c.md"}
	for _, p := range no {
		if shouldExcludePath(p) {
			t.Errorf("expected %q to be included", p)
		}
	}
}
