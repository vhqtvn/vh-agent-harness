package permission

// ---------------------------------------------------------------------------
// Static deny-time grant-naming tests (card inter-layer-dead-grant-lint, PR B).
//
// These drive the REAL OpenCode wrapper (templates/core/.opencode/plugins/
// shell-guard.js — the server() factory's tool.execute.before handler) through
// a small node driver, because the naming is composed at the WRAPPER's deny
// translation point, not inside the agent-agnostic evaluate() engine that
// eval.js imports. The scratch install carries an opencode.jsonc with
// configured grants; assertions pin:
//
//   - the exact wording shape ("Denied before per-agent grants are evaluated.
//     Matching configured grants that cannot rescue this command: ..."),
//   - deterministic (agent, pattern, action) multi-match ordering,
//   - the WORDING CONTRACT as negative assertions: no engine-allowlist-
//     addition suggestion via overlay, and NO active-agent attribution (the
//     message names configured grants only — the hook input exposes
//     sessionID only and no session→agent resolution may be inferred),
//   - the no-match control: a denied command with no matching configured
//     grant keeps the plain engine deny message (no suffix),
//   - fail-open: no/broken opencode.jsonc → no suffix, deny unchanged,
//   - JSONC parity: a hand-edited config (// and /* */ comments, trailing
//     commas) yields the SAME suffix as its JSON twin (string-aware
//     stripping — a // inside a string value is not a comment), while a
//     document still unparseable after JSONC normalization stays fail-open,
//   - matcher shapes: exact no-star grants are listed; non-trailing `*`
//     shapes (interior/leading) never match and are never listed; the bare
//     "*" catch-all is never listed.
//
// The WASM parser is NOT needed for these commands (evaluate's fallback
// tokenizer handles them; the live-bridge suite covers the parser paths), so
// no npm install is performed. Node itself is required (skip otherwise).
// ---------------------------------------------------------------------------

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/jsonc"
)

// stageWrapperScratch stages a scratch install with the wrapper + core +
// repo-configs from the corpus templates and returns the scratch root (which
// contains the .opencode/ tree). Node itself is required (skip otherwise).
func stageWrapperScratch(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}

	modRoot := findModuleRoot(t)
	tmplOpencode := filepath.Join(modRoot, "templates", "core", ".opencode")

	scratchParent := filepath.Join(modRoot, "tmp")
	if err := os.MkdirAll(scratchParent, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scratchParent, err)
	}
	scratch, err := os.MkdirTemp(scratchParent, "sgwrap-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })

	files := []string{
		"package.json",
		filepath.Join("repo-configs", "allowed-commands.js"),
		filepath.Join("repo-configs", "forbidden-patterns.js"),
		filepath.Join("repo-configs", "forbidden-patterns.core.js"),
		filepath.Join("plugins", "shell-guard-core.js"),
		filepath.Join("plugins", "shell-guard.js"),
	}
	for _, rel := range files {
		src := filepath.Join(tmplOpencode, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read template %s: %v (template not rendered?)", rel, err)
		}
		dst := filepath.Join(scratch, ".opencode", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
	return scratch
}

// wrapperNamingScratch stages a scratch install with the wrapper + core +
// repo-configs and an opencode.jsonc whose grants exercise the naming, then
// writes the driver. Returns the driver path (run: node <driver> "<command>").
func wrapperNamingScratch(t *testing.T, opencodeJSONC string) string {
	t.Helper()
	scratch := stageWrapperScratch(t)
	if opencodeJSONC != "" {
		if err := os.WriteFile(filepath.Join(scratch, "opencode.jsonc"), []byte(opencodeJSONC), 0o644); err != nil {
			t.Fatalf("write opencode.jsonc: %v", err)
		}
	}

	driver := `
import { server } from "./.opencode/plugins/shell-guard.js";
const api = await server();
const command = process.argv[2] ?? "";
let out;
try {
    await api["tool.execute.before"]({ tool: "bash", sessionID: "ses_test" }, { args: { command } });
    out = { thrown: null };
} catch (e) {
    out = { thrown: String(e && e.message ? e.message : e) };
}
process.stdout.write(JSON.stringify(out));
`
	driverPath := filepath.Join(scratch, "driver.mjs")
	if err := os.WriteFile(driverPath, []byte(driver), 0o644); err != nil {
		t.Fatalf("write driver: %v", err)
	}
	return driverPath
}

// runWrapperDriver runs the driver for one command and returns the thrown
// message (empty string when the handler did not throw).
func runWrapperDriver(t *testing.T, driverPath, command string) string {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node disappeared: %v", err)
	}
	cmd := exec.Command(nodeBin, driverPath, command)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("driver failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("run driver: %v", err)
	}
	var res struct {
		Thrown *string `json:"thrown"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse driver output %q: %v", string(out), err)
	}
	if res.Thrown == nil {
		return ""
	}
	return *res.Thrown
}

// wrapperNamingConfig: "build" carries a dead allow ("pytest *"), "researcher"
// a dead ask ("pytest *") plus a dead allow ("docker *"), "default" a
// "terraform *" allow — only the pytest pair matches `pytest -x foo`.
const wrapperNamingConfig = `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "ask",
                    "ls *": "allow",
                    "pytest *": "allow"
                }
            }
        },
        "researcher": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "docker *": "allow",
                    "pytest *": "ask"
                }
            }
        }
    },
    "permission": {
        "bash": {
            "*": "deny",
            "terraform *": "allow"
        }
    }
}
`

// TestWrapperGrantNaming_DenyListsMatchingConfiguredGrants: a denied command
// with matching configured allow/ask grants gets the naming suffix, in
// deterministic (agent, pattern, action) order, with the sanctioned remedies.
func TestWrapperGrantNaming_DenyListsMatchingConfiguredGrants(t *testing.T) {
	driver := wrapperNamingScratch(t, wrapperNamingConfig)
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest -x foo must be denied (throw); handler returned normally")
	}
	// Exact wording shape from the card's resolved contract.
	const header = "Denied before per-agent grants are evaluated. Matching configured grants that cannot rescue this command:"
	if !strings.Contains(msg, header) {
		t.Errorf("deny message missing the naming header; got:\n%s", msg)
	}
	// Deterministic multi-match ordering: build's allow before researcher's
	// ask (agent sort), in one listed sentence.
	wantList := `agent "build" pattern "pytest *" (allow); agent "researcher" pattern "pytest *" (ask)`
	if !strings.Contains(msg, wantList) {
		t.Errorf("deny message missing deterministic grant list %q; got:\n%s", wantList, msg)
	}
	// Non-matching configured grants (docker, terraform) must NOT be listed.
	for _, absent := range []string{`"docker *"`, `"terraform *"`, `"ls *"`} {
		if strings.Contains(msg, absent) {
			t.Errorf("deny message lists non-matching grant %s; got:\n%s", absent, msg)
		}
	}
	// The plain engine deny reason stays intact ahead of the suffix.
	if !strings.Contains(msg, "Commands outside the read-only inspection surface") {
		t.Errorf("deny message must retain the engine reason; got:\n%s", msg)
	}
	// Sanctioned remedies only.
	for _, want := range []string{"remove the grant", "downgrade the grant", "vh-agent-harness exec"} {
		if !strings.Contains(msg, want) {
			t.Errorf("deny message must name remedy %q; got:\n%s", want, msg)
		}
	}
	// WORDING CONTRACT negative assertions (operator-confirmed contract; see
	// card inter-layer-dead-grant-lint):
	// no allowlist-addition suggestion via overlay; no active-agent
	// attribution (static configured-grant naming only — even though the
	// driver passes a sessionID, the message must never use it).
	for _, bad := range []string{
		"overlay", "allowlist", "allowed-commands", "forbidden-patterns", "permission-pack",
		"current agent", "active agent", "was denied by", "attempted", "session",
	} {
		if strings.Contains(strings.ToLower(msg), bad) {
			t.Errorf("deny message wording violation: contains forbidden token %q; got:\n%s", bad, msg)
		}
	}
}

// TestWrapperGrantNaming_HarnessGitDenyNamesGrant: the vh-agent-harness git
// deny branch also carries the naming for a matching configured grant.
func TestWrapperGrantNaming_HarnessGitDenyNamesGrant(t *testing.T) {
	cfg := `{
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "vh-agent-harness git *": "allow"
                }
            }
        }
    }
}
`
	driver := wrapperNamingScratch(t, cfg)
	msg := runWrapperDriver(t, driver, "vh-agent-harness git status")
	if msg == "" {
		t.Fatalf("vh-agent-harness git status must be denied; handler returned regularly")
	}
	if !strings.Contains(msg, "Git commands must be run directly, not through vh-agent-harness.") {
		t.Errorf("engine reason for the harness-git deny must stand; got:\n%s", msg)
	}
	if !strings.Contains(msg, `agent "build" pattern "vh-agent-harness git *" (allow)`) {
		t.Errorf("harness-git deny must name the matching configured grant; got:\n%s", msg)
	}
}

// TestWrapperGrantNaming_NoMatchKeepsPlainDeny: a denied command with NO
// matching configured grant keeps the plain engine message (no suffix) — the
// naming is purely additive on actual matches.
func TestWrapperGrantNaming_NoMatchKeepsPlainDeny(t *testing.T) {
	driver := wrapperNamingScratch(t, wrapperNamingConfig)
	msg := runWrapperDriver(t, driver, "curl http://example.com")
	if msg == "" {
		t.Fatalf("curl must be denied; handler returned regularly")
	}
	if strings.Contains(msg, "Matching configured grants") {
		t.Errorf("no configured grant matches curl — the naming suffix must be absent; got:\n%s", msg)
	}
	if !strings.Contains(msg, "Commands outside the read-only inspection surface") {
		t.Errorf("plain engine deny must stand; got:\n%s", msg)
	}
}

// TestWrapperGrantNaming_AllowAndAskPathsUnchanged: engine allow (readonly
// command) and engine ask (unknown git verb) never throw — the naming only
// touches the deny translation, and evaluation semantics are unchanged.
func TestWrapperGrantNaming_AllowAndAskPathsUnchanged(t *testing.T) {
	driver := wrapperNamingScratch(t, wrapperNamingConfig)
	if msg := runWrapperDriver(t, driver, "ls -la"); msg != "" {
		t.Errorf("ls -la must be allowed (no throw); got throw: %s", msg)
	}
	// git describe routes to the engine's ask branch: the handler RETURNS
	// (console.error hint + passthrough), it must not throw.
	if msg := runWrapperDriver(t, driver, "git describe --tags"); msg != "" {
		t.Errorf("git describe must pass through to the permission table (no throw); got throw: %s", msg)
	}
}

// TestWrapperGrantNaming_FailOpenWithoutConfig: no opencode.jsonc (or a
// non-JSON one) means no suffix — the deny itself is unchanged.
func TestWrapperGrantNaming_FailOpenWithoutConfig(t *testing.T) {
	driver := wrapperNamingScratch(t, "")
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest must still be denied without a config; handler returned regularly")
	}
	if strings.Contains(msg, "Matching configured grants") {
		t.Errorf("absent opencode.jsonc must not produce a naming suffix; got:\n%s", msg)
	}
}

// TestWrapperGrantNaming_FailOpenOnNonJSONConfig: a broken config file is the
// same fail-open case (no suffix, deny stands).
func TestWrapperGrantNaming_FailOpenOnNonJSONConfig(t *testing.T) {
	driver := wrapperNamingScratch(t, "{definitely not json")
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest must still be denied with a broken config; handler returned regularly")
	}
	if strings.Contains(msg, "Matching configured grants") {
		t.Errorf("broken opencode.jsonc must not produce a naming suffix; got:\n%s", msg)
	}
}

// TestWrapperGrantNaming_FailOpenOnUnparseableJSONC: JSONC that is STILL
// unparseable after comment/trailing-comma normalization (here: an object
// that never closes) keeps the intentional fail-open contract — no suffix,
// deny stands. The deterministic-JSONC outcome replaces the old
// fail-open-on-comment behavior ONLY for documents that normalize to valid
// JSON; genuinely malformed input is unchanged.
func TestWrapperGrantNaming_FailOpenOnUnparseableJSONC(t *testing.T) {
	cfg := `{
    // a comment is stripped, but the object below never closes:
    "permission": {
        "bash": {
            "pytest *": "allow",
`
	driver := wrapperNamingScratch(t, cfg)
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest must still be denied with an unparseable JSONC config; handler returned regularly")
	}
	if strings.Contains(msg, "Matching configured grants") {
		t.Errorf("unparseable-after-normalization opencode.jsonc must not produce a naming suffix; got:\n%s", msg)
	}
}

// wrapperNamingJSONCConfig is the JSONC twin of wrapperNamingConfig: same
// grants, plus // line comments, a /* block */ comment, trailing commas, and
// grant keys whose STRING VALUES contain " // " (a URL-shaped token) and an
// ESCAPED QUOTE followed by " // " text. A naive line-comment stripper would
// truncate the URL key mid-string, and one that mishandles escapes would end
// string mode at the escaped quote and strip the rest as a comment — either
// way the parse breaks and the reader fail-opens; a string-aware,
// escape-aware one (the contract) leaves both keys intact.
const wrapperNamingJSONCConfig = `{
    // agent blocks carry the same grants as the JSON twin
    "agent": {
        "build": {
            "permission": {
                "bash": {
                    "*": "ask",
                    "ls *": "allow",
                    /* block comment pins the block stripper */
                    "pytest *": "allow",
                }
            }
        },
        "researcher": {
            "permission": {
                "bash": {
                    "*": "deny",
                    "docker *": "allow",
                    "pytest *": "ask",
                    "echo see https://example.com/x // not a comment *": "allow",
                    "echo \"quoted\" // still not a comment *": "allow",
                }
            }
        },
    },
    "permission": {
        "bash": {
            "*": "deny",
            "terraform *": "allow",
        }
    },
}
`

// TestWrapperGrantNaming_JSONCConfigNamesSameGrants: a hand-edited JSONC
// config (comments + trailing commas) produces the IDENTICAL naming suffix
// its JSON twin produces — no silent JSON/JSONC divergence. The suffix being
// present at all proves string-aware comment stripping (the URL-shaped key
// survives; a naive stripper would have broken the parse and fail-opened),
// and the identical listing proves parse-level parity.
func TestWrapperGrantNaming_JSONCConfigNamesSameGrants(t *testing.T) {
	driver := wrapperNamingScratch(t, wrapperNamingJSONCConfig)
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest -x foo must be denied (throw); handler returned normally")
	}
	const header = "Denied before per-agent grants are evaluated. Matching configured grants that cannot rescue this command:"
	if !strings.Contains(msg, header) {
		t.Errorf("JSONC deny message missing the naming header; got:\n%s", msg)
	}
	// IDENTICAL to the JSON twin's listing (TestWrapperGrantNaming_Deny-
	// ListsMatchingConfiguredGrants) — the JSON/JSONC parity pin.
	wantList := `agent "build" pattern "pytest *" (allow); agent "researcher" pattern "pytest *" (ask)`
	if !strings.Contains(msg, wantList) {
		t.Errorf("JSONC deny message must list the same grants as the JSON twin %q; got:\n%s", wantList, msg)
	}
	// Non-matching grants — including the URL-shaped string-aware key and the
	// escaped-quote key — must not be listed (neither matches `pytest -x foo`).
	for _, absent := range []string{`"docker *"`, `"terraform *"`, `"ls *"`, "example.com", "still not a comment"} {
		if strings.Contains(msg, absent) {
			t.Errorf("JSONC deny message lists non-matching grant %s; got:\n%s", absent, msg)
		}
	}
	if !strings.Contains(msg, "Commands outside the read-only inspection surface") {
		t.Errorf("JSONC deny message must retain the engine reason; got:\n%s", msg)
	}
}

// TestWrapperGrantNaming_ExactNoStarGrantListed: an exact token-sequence
// grant with NO wildcard (an emitted-table shape: `uuidgen`, the gate
// `…commit-gate.sh status` probe) is matched by token equality and listed
// when the exact command is denied. The bare "*" catch-all in the same block
// must NOT be listed.
func TestWrapperGrantNaming_ExactNoStarGrantListed(t *testing.T) {
	cfg := `{
    "permission": {
        "bash": {
            "*": "ask",
            "terraform apply": "allow"
        }
    }
}
`
	driver := wrapperNamingScratch(t, cfg)
	msg := runWrapperDriver(t, driver, "terraform apply")
	if msg == "" {
		t.Fatalf("terraform apply must be denied; handler returned regularly")
	}
	if !strings.Contains(msg, `agent "default" pattern "terraform apply" (allow)`) {
		t.Errorf("exact no-star grant must be listed; got:\n%s", msg)
	}
	if strings.Contains(msg, `pattern "*"`) {
		t.Errorf(`bare "*" catch-all must never be listed; got:\n%s`, msg)
	}
}

// TestWrapperGrantNaming_NonTrailingStarNeverListed: shapes with a `*`
// anywhere other than the trailing position (leading `*pytest *`, interior
// `cat *=x *`) are outside the token-prefix model — the `*` is a literal
// token character, real command tokens never contain one, so such grants can
// never match and are never listed. The deny itself stands (documented
// limitation at the read site: message-only impact).
func TestWrapperGrantNaming_NonTrailingStarNeverListed(t *testing.T) {
	cfg := `{
    "permission": {
        "bash": {
            "*pytest *": "allow",
            "cat *=x *": "ask"
        }
    }
}
`
	driver := wrapperNamingScratch(t, cfg)
	msg := runWrapperDriver(t, driver, "pytest -x foo")
	if msg == "" {
		t.Fatalf("pytest -x foo must be denied; handler returned regularly")
	}
	if strings.Contains(msg, "Matching configured grants") {
		t.Errorf("non-trailing-star grants must never be listed; got:\n%s", msg)
	}
	if !strings.Contains(msg, "Commands outside the read-only inspection surface") {
		t.Errorf("plain engine deny must stand; got:\n%s", msg)
	}
}

// ---------------------------------------------------------------------------
// Go↔JS JSONC stripper-twin parity (card jsonc-stripper-twin-parity). The
// corpus plugin carries JS twins of internal/jsonc's strippers
// (stripJSONCComments/stripJSONCTrailingCommas in shell-guard.js, feeding
// parseJSONCTolerant — see the wrapper's JSONC reader contract). They are
// module-internal, so this drives them through a scratch-only re-export: the
// corpus file is copied verbatim into the scratch install by
// stageWrapperScratch and ONE export line is appended to the COPY (the corpus
// template itself is never touched). The appended export binds to the same
// function declarations, so a corpus rename/removal of the strippers fails
// the driver import loudly — the parity obligation trips instead of silently
// testing nothing.
//
// The fixture corpus is shared with internal/jsonc (its testdata file is the
// single source of truth; the Go side also pins it there, including the two
// ad36dfa advisory EOF/CRLF edges). Parity is asserted on the FULL pipeline
// both readers run: stripJSONCTrailingCommas ∘ stripJSONCComments ≡
// jsonc.Normalize, byte for byte, across the corpus.
// ---------------------------------------------------------------------------

func TestWrapperGrantNaming_JSONCStripperTwinParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}
	fixturesPath := filepath.Join(findModuleRoot(t), "internal", "jsonc", "testdata", "stripper-parity-fixtures.json")
	fixturesAbs, err := filepath.Abs(fixturesPath)
	if err != nil {
		t.Fatalf("abs fixtures path: %v", err)
	}
	data, err := os.ReadFile(fixturesPath)
	if err != nil {
		t.Fatalf("read shared fixture corpus: %v", err)
	}
	var corpus []struct {
		Name  string `json:"name"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("parse shared fixture corpus: %v", err)
	}
	if len(corpus) == 0 {
		t.Fatalf("shared fixture corpus must not be empty")
	}

	scratch := stageWrapperScratch(t)
	sgPath := filepath.Join(scratch, ".opencode", "plugins", "shell-guard.js")
	src, err := os.ReadFile(sgPath)
	if err != nil {
		t.Fatalf("read scratch shell-guard.js: %v", err)
	}
	appendExport := "\n// scratch-only test re-export (the corpus template is never modified)\nexport { stripJSONCComments, stripJSONCTrailingCommas };\n"
	if err := os.WriteFile(sgPath, append(src, []byte(appendExport)...), 0o644); err != nil {
		t.Fatalf("append scratch re-export: %v", err)
	}

	driver := `
import { readFileSync } from "node:fs";
import { stripJSONCComments, stripJSONCTrailingCommas } from "./.opencode/plugins/shell-guard.js";
const fixtures = JSON.parse(readFileSync(process.argv[2], "utf8"));
const out = fixtures.map((f) => stripJSONCTrailingCommas(stripJSONCComments(f.input)));
process.stdout.write(JSON.stringify(out));
`
	driverPath := filepath.Join(scratch, "stripper-parity-driver.mjs")
	if err := os.WriteFile(driverPath, []byte(driver), 0o644); err != nil {
		t.Fatalf("write driver: %v", err)
	}

	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node disappeared: %v", err)
	}
	out, err := exec.Command(nodeBin, driverPath, fixturesAbs).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("stripper parity driver failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("run stripper parity driver: %v", err)
	}
	var jsOut []string
	if err := json.Unmarshal(out, &jsOut); err != nil {
		t.Fatalf("parse driver output %q: %v", string(out), err)
	}
	if len(jsOut) != len(corpus) {
		t.Fatalf("driver returned %d outputs; want %d (corpus length)", len(jsOut), len(corpus))
	}
	for i, f := range corpus {
		i, f := i, f
		t.Run(f.Name, func(t *testing.T) {
			want := string(jsonc.Normalize([]byte(f.Input)))
			if jsOut[i] != want {
				t.Fatalf("JS stripper twin diverged from internal/jsonc:\ninput:        %q\ngo (Normalize): %q\njs (strippers): %q", f.Input, want, jsOut[i])
			}
		})
	}
}
