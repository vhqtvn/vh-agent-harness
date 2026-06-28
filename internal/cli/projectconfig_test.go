package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeProjectConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectConfigAnswers_ListArchitecture(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, `{
	  "project": {
	    "mission_summary": "Do the thing.",
	    "architecture_summary": ["apps/api - backend", "apps/web - frontend"],
	    "db_user": "u", "db_name": "d"
	  }
	}`)
	got := projectConfigAnswers(root)
	if got["mission_summary"] != "Do the thing." {
		t.Errorf("mission_summary = %q", got["mission_summary"])
	}
	if got["architecture_summary"] != "- apps/api - backend\n- apps/web - frontend" {
		t.Errorf("architecture_summary = %q", got["architecture_summary"])
	}
	if got["db_user"] != "u" || got["db_name"] != "d" {
		t.Errorf("db = %q/%q", got["db_user"], got["db_name"])
	}
}

func TestProjectConfigAnswers_StringArchitecture(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, `{"project":{"architecture_summary":"freeform text"}}`)
	if got := projectConfigAnswers(root)["architecture_summary"]; got != "freeform text" {
		t.Errorf("string architecture_summary = %q", got)
	}
}

func TestProjectConfigAnswers_AbsentAndMalformed(t *testing.T) {
	if got := projectConfigAnswers(t.TempDir()); len(got) != 0 {
		t.Errorf("absent config must yield empty map, got %v", got)
	}
	root := t.TempDir()
	writeProjectConfig(t, root, `{not json`)
	if got := projectConfigAnswers(root); len(got) != 0 {
		t.Errorf("malformed config must be ignored (empty), got %v", got)
	}
}

// exampleProjectConfigPath is the embedded example project.config.json, relative
// to this test file's package directory (internal/cli). The contract test below
// reads it directly from the source tree so a change to the example cannot drift
// past CI.
const exampleProjectConfigPath = "../../templates/examples/.vh-agent-harness/project.config.json"

// consumedProjectConfigFields is the exact set of keys under the `project` block
// that projectConfigAnswers reads (projectconfig.go struct tags). It is the
// contract the example must advertise — duplicated here on purpose so a change
// in EITHER direction (an inert field added to the example, or a consumed field
// dropped from the example) fails the test. Keep in sync with the struct tags in
// projectConfigAnswers when adding a consumed field.
var consumedProjectConfigFields = map[string]bool{
	"mission_summary":      true,
	"architecture_summary": true,
	"db_user":              true,
	"db_name":              true,
}

// TestExampleProjectConfig_AdvertisesOnlyConsumedFields is the W1/Q3 contract
// test. It parses the embedded example project.config.json, collects every
// NON-COMMENT key under the `project` object, and asserts that set EXACTLY equals
// the fields projectConfigAnswers consumes. This converts "the example advertises
// inert fields that have no reader" (the original adoption footgun: a consumer
// filled in 16 fields, 12 of which were silently ignored) into a CI failure in
// both directions: no extra advertised keys, no missing consumed keys. Comment
// keys (starting with "//", a JSON-doc convention) are treated as documentation
// and excluded, so the example may carry inline docs under `project`.
func TestExampleProjectConfig_AdvertisesOnlyConsumedFields(t *testing.T) {
	raw, err := os.ReadFile(exampleProjectConfigPath)
	if err != nil {
		t.Fatalf("read example project.config.json: %v\n(path %s — run from repo root via `go test ./internal/cli/`)", err, exampleProjectConfigPath)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse example project.config.json: %v", err)
	}
	projectRaw, ok := doc["project"]
	if !ok {
		t.Fatal("example project.config.json has no `project` block")
	}
	var projectObj map[string]json.RawMessage
	if err := json.Unmarshal(projectRaw, &projectObj); err != nil {
		t.Fatalf("parse `project` block: %v", err)
	}

	advertised := map[string]bool{}
	for k := range projectObj {
		// Comment/doc keys ("//", "//_note", …) are JSON-doc conventions, not
		// settable fields the seam reads. Exclude them so the example may carry
		// inline documentation without tripping the contract.
		if strings.HasPrefix(k, "//") {
			continue
		}
		advertised[k] = true
	}

	var extra, missing []string
	for k := range advertised {
		if !consumedProjectConfigFields[k] {
			extra = append(extra, k)
		}
	}
	for k := range consumedProjectConfigFields {
		if !advertised[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 {
		t.Errorf("example project.config.json advertises fields the seam never reads (inert → footgun): %v\nonly these fields are consumed: %v\nremove the inert keys, or document them as comments (keys starting with //), not settable fields.", extra, sortedKeys(consumedProjectConfigFields))
	}
	if len(missing) > 0 {
		t.Errorf("example project.config.json is MISSING consumed field(s) %v\nthe seam reads these (projectconfig.go :: projectConfigAnswers); the example must advertise them so a consumer knows to fill them in.", missing)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestWarnUnresolvedProjectConfigTokens is the W3 unit test (closes the G3
// coverage gap surfaced by the Slice-1 commit-review). It exercises
// warnUnresolvedProjectConfigTokens across the cases that matter for the adoption
// footgun: absent config (all empty), a fully-filled config (no warning), a
// config saved verbatim from `vh-agent-harness example` (fields still hold their
// literal {{...}} sentinels — the original incident this epic addresses), and a
// mixed config. The function is NON-FATAL, so each case asserts on the captured
// output, not on a returned error.
func TestWarnUnresolvedProjectConfigTokens(t *testing.T) {
	// A config with all four fields filled with real (non-token) values — the
	// only case that must stay SILENT.
	filledJSON := `{"project":{
		"mission_summary":"Build the thing.",
		"architecture_summary":["apps/api - backend"],
		"db_user":"svc","db_name":"core"
	}}`

	tests := []struct {
		name      string
		setupJSON string // empty string => write no config (absent)
		wantWarn  bool
		// substrings that MUST appear when wantWarn; ignored when !wantWarn.
		wantSubs []string
	}{
		{
			name:     "absent config warns all four are empty",
			wantWarn: true,
			wantSubs: []string{
				"UNRESOLVED",
				"No .vh-agent-harness/project.config.json found",
				"mission_summary",
				"architecture_summary",
				"db_user",
				"db_name",
			},
		},
		{
			name:      "fully filled config is silent",
			setupJSON: filledJSON,
			wantWarn:  false,
		},
		{
			// The verbatim-save footgun: a consumer runs `vh-agent-harness example`
			// and saves the output unchanged. Every field is non-empty (so the
			// prior empty-only check missed it) but still a literal sentinel, so
			// the renderer substitutes the sentinel for itself and ships literal
			// {{TOKEN}}s. This case MUST warn.
			name:      "verbatim example sentinels warn",
			setupJSON: `{"project":{"mission_summary":"{{MISSION_SUMMARY}}","architecture_summary":["{{ARCHITECTURE_SUMMARY}}"],"db_user":"{{DB_USER}}","db_name":"{{DB_NAME}}"}}`,
			wantWarn:  true,
			wantSubs: []string{
				"UNRESOLVED",
				"still literal {{MISSION_SUMMARY}}",
				"still literal {{DB_USER}}",
				"still literal {{DB_NAME}}",
				"still literal {{ARCHITECTURE_SUMMARY}}",
			},
		},
		{
			name: "mixed empty and sentinel fields warn on the right subset",
			// mission_summary filled (silent for it); db_user empty; db_name a
			// literal sentinel; architecture_summary filled.
			setupJSON: `{"project":{"mission_summary":"Build it.","architecture_summary":["apps/api"],"db_user":"","db_name":"{{DB_NAME}}"}}`,
			wantWarn:  true,
			wantSubs: []string{
				"db_name",
				"db_user",
				"still literal {{DB_NAME}}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.setupJSON != "" {
				writeProjectConfig(t, root, tt.setupJSON)
			}
			var buf bytes.Buffer
			warnUnresolvedProjectConfigTokens(&buf, root)
			got := buf.String()

			if tt.wantWarn {
				if got == "" {
					t.Fatalf("expected a warning, got empty output")
				}
				for _, sub := range tt.wantSubs {
					if !strings.Contains(got, sub) {
						t.Errorf("warning output missing substring %q\n--- output ---\n%s", sub, got)
					}
				}
				if !strings.Contains(got, "non-fatal") {
					t.Errorf("warning must state it is non-fatal")
				}
			} else {
				if got != "" {
					t.Errorf("expected SILENT (no warning) for a fully-filled config, got:\n%s", got)
				}
			}
		})
	}
}
