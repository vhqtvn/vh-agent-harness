package cli

// doctor_behavioral_closure.go — the BEHAVIORAL-CLOSURE structural-consistency
// gate (the 14th doctor check).
//
// This is the SAFETY LAYER acting (per the authority line that governs this
// repo: "coordinator state INFORMS; safety-layer gates ACT"). It is a
// STRUCTURAL validator, not a behavioral prover: it scans durable closeout
// artifacts for the `behavioral-closure` fenced declaration token and FAILs
// only when a declaration is internally inconsistent — specifically when a
// `verdict: proven` is claimed WITHOUT a proven crux `result:`.
//
// THE TOKEN IS A DECLARATION, NOT A PROOF (docker-gold caveat). Making the
// declaration HONEST and non-droppable is the gate's entire job: a
// `verdict: proven` that cannot point at a proven crux is internally
// contradictory and is rejected. The gate does NOT, and cannot, prove the
// cited crux command actually executed end-to-end — a syntactically-consistent
// `result: proven` declares the path was exercised; proving it needs
// repo-specific live verification (the test suite, the demo run, etc.). State
// this honestly wherever the token is documented.
//
// WHY DOCTOR (and not release_gate.go / the task-closeout command): the
// validator must be mechanical, unbypassable, and cover closeouts that NEVER
// reach a release. doctor is the seam health surface that already scans
// `.local/coordinator/` and durable markdown artifacts; release_gate.go owns
// RELEASE properties (defer-liveness against shipped migration notes), and the
// task-closeout command is prompt wording (advisory). Behavioral-completion
// truth belongs to doctor so it gates every closeout, not just pre-release
// ones. This file is INDEPENDENT of release_gate.go and the claims kernel: it
// reads closeout markdown directly.
//
// THE GATE (narrow pilot):
//   - Absent token entirely            => PASS  (the pilot does NOT require every
//                                                closeout to carry one; forcing
//                                                adoption would mark every
//                                                pre-pilot closeout UNHEALTHY.
//                                                The token makes a declaration
//                                                honest, it does not force one.)
//   - verdict: proven + result: proven => PASS
//   - verdict != proven + any result   => PASS  (inconclusive/failed/abandoned
//                                                may pair with any crux result)
//   - verdict: proven + result != proven (skipped, not-demonstrable, OR absent)
//                                        => FAIL  (internally contradictory)
//   - unknown verdict/result enum      => FAIL  (fail-closed on garbage,
//                                                mirroring defer-liveness)
//   - declaration with no verdict field => FAIL  (malformed declaration)
//
// SCAN SURFACES (durable closeout artifacts):
//   - .local/coordinator/reports/**/*.md  (local closeout reports; transport)
//   - docs/checkpoints/*.md               (committed durable closeouts)
// Both are safe because an artifact with no token contributes no finding.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// behavioralClosureRe matches a fenced code block whose info string begins with
// "behavioral-closure" and captures the block body (the lines between the
// opening and closing fences). (?s) lets . match newlines; .*? is non-greedy so
// a block ends at its FIRST closing fence. The opening info line may carry
// trailing spaces/tabs only (a "behavioral-closures"-style different info string
// does not match because the char after the literal prefix is not [ \t] or \n).
// Deliberately a double-quoted string (the pattern contains literal backticks,
// which cannot appear inside a Go raw string literal).
var behavioralClosureRe = regexp.MustCompile("(?s)```behavioral-closure[ \\t]*\\n(.*?)\\n```")

// bcKeyRe matches a single "key: value" line inside a behavioral-closure block.
// The key is alphanumeric (dots/dashes allowed); the value is the remainder of
// the line (trimmed), so a value may itself contain colons.
var bcKeyRe = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*:\s*(.+?)\s*$`)

// bcValidVerdicts is the enum for the behavioral verdict (the overall verdict
// on the load-bearing path).
var bcValidVerdicts = map[string]bool{
	"proven":       true,
	"inconclusive": true,
	"failed":       true,
	"abandoned":    true,
}

// bcValidResults is the enum for the crux result (whether the load-bearing path
// was actually exercised end-to-end). "result" in the token IS the crux result;
// the block name scopes it unambiguously.
var bcValidResults = map[string]bool{
	"proven":           true,
	"skipped":          true,
	"not-demonstrable": true,
}

// checkBehavioralClosure is the 14th doctor check. See the file-level comment
// for the full gate contract. It is READ-ONLY: it never mutates a closeout
// artifact and never shells out.
func checkBehavioralClosure(target string) checkResult {
	const name = "behavioral-closure"

	surfaces := []string{
		filepath.Join(".local", "coordinator", "reports"),
		filepath.Join("docs", "checkpoints"),
	}

	type finding struct{ source, reason string }
	var findings []finding
	mdFiles := 0

	for _, surf := range surfaces {
		root := filepath.Join(target, surf)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			// Surface absent (common: no reports yet, no checkpoints) => skip it.
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			mdFiles++
			rel, relErr := filepath.Rel(target, path)
			if relErr != nil {
				rel = path
			}
			data, err := os.ReadFile(path)
			if err != nil {
				// Unreadable closeout artifact => fail-closed (cannot verify consistency).
				findings = append(findings, finding{rel, "unreadable closeout artifact: " + err.Error()})
				return nil
			}
			for _, reason := range analyzeBehavioralClosureBlocks(string(data)) {
				findings = append(findings, finding{rel, reason})
			}
			return nil
		})
	}

	if mdFiles == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: "no closeout/checkpoint artifacts to scan (no behavioral-closure declarations yet)"}
	}
	if len(findings) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d artifact(s) scanned; every behavioral-closure declaration is internally consistent", mdFiles)}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].source < findings[j].source })
	var b strings.Builder
	fmt.Fprintf(&b, "%d inconsistent/malformed behavioral-closure declaration(s):", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "\n  - %s: %s", f.source, f.reason)
	}
	b.WriteString("\nA behavioral-closure declaration is a fenced ```behavioral-closure block. verdict: proven REQUIRES result: proven (the crux / load-bearing path exercised end-to-end); any other verdict may pair with any result. Fix the declaration, or remove the block if it does not apply. (The token declares consistency; it does not prove the path executed.)")
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// analyzeBehavioralClosureBlocks parses every ```behavioral-closure fenced block
// in body and returns a reason string for each block that is malformed or
// internally inconsistent. A consistent block contributes no reason. This is the
// pure parsing core, separated so tests can drive it without touching the
// filesystem.
func analyzeBehavioralClosureBlocks(body string) []string {
	blocks := behavioralClosureRe.FindAllStringSubmatch(body, -1)
	var reasons []string
	for _, m := range blocks {
		kv := parseBehavioralClosureKV(m[1])
		verdict, hasVerdict := kv["verdict"]
		result, hasResult := kv["result"]

		if !hasVerdict {
			reasons = append(reasons, "behavioral-closure block has no 'verdict:' field (a declaration must state its verdict)")
			continue
		}
		if !bcValidVerdicts[verdict] {
			reasons = append(reasons, fmt.Sprintf("unknown verdict %q (expected one of proven, inconclusive, failed, abandoned)", verdict))
			continue
		}
		if hasResult && !bcValidResults[result] {
			reasons = append(reasons, fmt.Sprintf("unknown crux result %q (expected one of proven, skipped, not-demonstrable)", result))
			continue
		}
		// Consistency gate: verdict: proven REQUIRES a proven crux result.
		// result != "proven" covers skipped, not-demonstrable, AND absent/empty.
		if verdict == "proven" && result != "proven" {
			what := result
			if !hasResult || result == "" {
				what = "(absent)"
			}
			reasons = append(reasons, fmt.Sprintf("verdict: proven but crux result is %q — a proven verdict requires result: proven (the load-bearing path must be exercised end-to-end)", what))
		}
	}
	return reasons
}

// stripInlineComment removes a trailing inline comment from a token value: a
// '#' that is preceded by whitespace (space or tab) starts a comment to the end
// of the line, mirroring the shell/git/make convention. A '#' NOT preceded by
// whitespace (e.g. a URL fragment '#anchor' or a path 'src#tag') is part of the
// value and is preserved. This lets the shipped CLOSEOUT_TEMPLATE annotate each
// enum value inline (e.g. "verdict: proven  # proven | inconclusive | ...") and
// still pass the gate when a closeout is copied verbatim from the template —
// the producer-facing canonical example and the consumer validator stay
// consistent.
func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

// parseBehavioralClosureKV extracts key:value pairs from a behavioral-closure
// block body. Keys are lower-cased; values are whitespace-trimmed and have any
// trailing " # ..." inline comment stripped (see stripInlineComment). Lines that
// do not match the key:value shape (blank lines, prose) are ignored.
func parseBehavioralClosureKV(raw string) map[string]string {
	kv := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		m := bcKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kv[strings.ToLower(strings.TrimSpace(m[1]))] = stripInlineComment(m[2])
	}
	return kv
}
