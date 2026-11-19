package taskcard

// staged_validation_test.go — unit tests for the staged-errata card SHAPE
// VERIFIER (CheckStagedErrataShape).
//
// Card defer-release-injector-degraded-eligibility (disposal b′, rescope
// 2026-11-19; round-5 fixes 2026-11-19): the verifier accepts EXACTLY the
// shape of the two real staged errata cards (byte-faithful fixtures below —
// copied values, never read from .local at test time) plus the canonical
// EMPTY states of the keys the sanctioned restage op materializes (round-5
// FIX 2 producer contract), and refuses everything else: execution-era or
// canonical-default CONTENT, unknown keys, missing/empty core fields,
// U+FEFF/whitespace-only strings (including space-wrapped-BOM mixtures),
// invalid enums, bad staged_path forms (including whitespace-padded ones —
// round-5 FIX 1, the consumed representation must equal the stored one),
// wrong types, malformed history, and non-object JSON.
//
// RED provenance: against the OLD dirty implementation (the JS-normalizer
// port, now deleted) every unexpected-field arm below was verified HEALTHY
// by the pre-rewrite probe (tmp/red-shape-probe.txt — 11/11 arms), so each
// refusal is a genuine behavior change. The old implementation DID already
// refuse some of the empty-list/enum arms (its DoR semantics degraded an
// empty success_criteria at non-draft status, and invalid enums) — those
// arms keep their coverage but are NOT red receipts. Round-5 RED receipts:
// tmp/red-round5-fix1.txt (whitespace-padded staged_path bypass) and
// tmp/red-round5-fix2.txt (restage-produced card refused as
// unexpected-field), captured by temporarily reverting the new guards.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// realErrataCard1 / realErrataCard2 are byte-faithful copies of the two real
// staged errata cards (.local/coordinator/tasks/errata-v0290-hub-token-
// normalization.json and errata-v0290-preserved-file-skew.json) — the
// verified template the allowlist was enumerated from. Copied as FIXTURE
// DATA; tests never read .local at test time.
const realErrataCard1 = `{
  "schema_version": 1,
  "task_id": "errata-v0290-hub-token-normalization",
  "title": "Erratum: v0.29.0 — hub-token normalization in state-lib.js (migration notice for the next release)",
  "status": "staged",
  "staged_path": "docs/migration-errata/v0.29.0-hub-token-normalization.md",
  "files_in_scope": [
    "docs/migration-errata/v0.29.0-hub-token-normalization.md"
  ],
  "rough_scope": [
    "Staged erratum transport for the committed hub-token-normalization migration notice (state-lib.js:5554 i++ → i += 1, commit 4469f3c; grammar-only intent, hub origin-hash advance at 0b56ab6, stale-preserve consequence, safeguard-first reconciliation, customization routes, link-integrity rider). The release ceremony's inject-errata step reads this card, injects the staged_path body into the next release note as an '## Errata for v0.29.0' section, and flips this card staged→completed."
  ],
  "history": [
    {
      "at": "2026-11-11T18:16:10Z",
      "event": "erratum_staged",
      "session_name": "erratum-stage-hub-token",
      "status": "staged",
      "note": "Staged by execution session for task-2026-11-11t18-10-20 (stage migration notice + erratum for state-lib.js hub-token normalization 4469f3c ahead of next release). staged_path points at the committed notice file per the canonical errata-card schema (internal/memory/claims DeferCard.staged_path; writeStagedErrataCard test helper mirrors this shape). Committed file left untouched; forward-notice framing (change landed post-v0.29.0 at 4469f3c, ships next release) carried in the file's status blockquote."
    }
  ],
  "created_at": "2026-11-11T18:16:10Z",
  "updated_at": "2026-11-11T18:16:10Z",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "success_criteria": [
    "Next release note carries an '## Errata for v0.29.0' section with the staged hub-token-normalization corrective body injected",
    "Card reaches completed state via the release ceremony's inject-errata step"
  ],
  "validation_plan": [
    "Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note",
    "Operator confirms the errata section appears in the published release note"
  ]
}`

const realErrataCard2 = `{
  "schema_version": 1,
  "task_id": "errata-v0290-preserved-file-skew",
  "title": "Erratum: v0.29.0 — preserved-file skew: update re-render and closeout-gate claims",
  "status": "staged",
  "staged_path": "docs/migration-errata/v0.29.0.md",
  "files_in_scope": [
    "docs/migration-errata/v0.29.0.md"
  ],
  "rough_scope": [
    "Staged erratum transport for the committed v0.29.0 erratum (preserved-file skew: per-file update re-render and JS-only closeout-gate claims). The release ceremony's inject-errata step reads this card, injects the staged_path corrective body into the next release note as an '## Errata for v0.29.0' section, and flips this card staged→completed."
  ],
  "history": [
    {
      "at": "2026-11-07T06:38:29Z",
      "event": "erratum_staged",
      "session_name": "erratum-stage",
      "status": "staged",
      "note": "Staged by execution session for task-2026-11-07t04-39-27 (release-ceremony inject erratum into next release note). staged_path points at the committed erratum file per the canonical errata-card schema (internal/memory/claims DeferCard.staged_path; writeStagedErrataCard test helper mirrors this shape). Committed file left untouched."
    }
  ],
  "created_at": "2026-11-07T06:38:29Z",
  "updated_at": "2026-11-07T06:38:29Z",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "success_criteria": [
    "Next release note carries an '## Errata for v0.29.0' section with the staged preserved-file-skew corrective body injected",
    "Card reaches completed state via the release ceremony's inject-errata step"
  ],
  "validation_plan": [
    "Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note",
    "Operator confirms the errata section appears in the published release note"
  ]
}`

// internalRestageProducedCard is the byte-faithful output of the REAL
// sanctioned producer (round-5 FIX 2): derived by actually running
// restageCoordinationTask — the rendered .opencode/scripts/state-lib.js,
// imported from a hermetic scratch under repo tmp/ (OPENCODE_LOCAL_COORDINATOR_
// ROOT isolated; scratch deleted after the run) — over a healthy ready
// errata-class card created through saveCoordinationTask. The canonical
// write merges defaultCoordinationTaskPayload into the saved card, so the
// output carries EVERY execution-era and canonical-default key at its
// canonical EMPTY state. A hand-shaped exemplar would not be producer
// evidence; these are the produced bytes. (The twin fixture in
// internal/cli/release_gate_test.go carries the same bytes for the surface
// tests.)
const internalRestageProducedCard = `{
  "schema_version": 1,
  "task_id": "errata-v0300-restage-fixture",
  "title": "Erratum: v0.30.0 — restage-produced fixture erratum (round-5 shape contract)",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "research_question": "",
  "source_policy": null,
  "source_allowlist": [],
  "desired_artifact_type": null,
  "target_artifact_path": null,
  "rough_scope": [
    "Fixture erratum produced by the real restageCoordinationTask for the round-5 shape contract integration test."
  ],
  "open_questions": [],
  "ready_criteria": [],
  "files_in_scope": [
    "docs/migration-errata/v0.30.0-restage-fixture.md"
  ],
  "constraints": [],
  "non_goals": [],
  "success_criteria": [
    "Gate and injector accept the real restage-produced card shape",
    "Canonical-empty execution-era keys pass; any content refuses"
  ],
  "validation_plan": [
    "go test ./internal/taskcard/ -run TestCheckStagedErrataShape (restage-fixture PASS arm)",
    "go test ./internal/cli/ -run 'TestStagedErrataContent|TestReleaseInjectErrata' (restage-fixture PASS arms)"
  ],
  "report_envelope": "minimal",
  "backlog_id": null,
  "workstream_slug": null,
  "dependencies": [],
  "owner_notes": [],
  "status": "staged",
  "session_aliases": [],
  "active_session_alias": null,
  "claimed_at": null,
  "report_paths": [],
  "review_paths": [],
  "latest_report": null,
  "f3_design_readiness": null,
  "decision_record": null,
  "next_action": "Awaiting release inject-errata: the release ceremony reads staged_path, injects the corrective body into the next release note, and flips this card to completed.",
  "predicted_impact": null,
  "measured_outcome": null,
  "last_review": null,
  "history": [
    {
      "at": "2026-11-19T06:54:44Z",
      "event": "task_created",
      "session_name": null,
      "status": "ready",
      "note": "Created local coordination task card."
    },
    {
      "at": "2026-11-19T06:54:44Z",
      "event": "task_restaged",
      "session_name": null,
      "status": "staged",
      "note": "Round-5 fixture: real restage output pinned as the committed Go test fixture."
    }
  ],
  "created_at": "2026-11-19T06:54:44Z",
  "updated_at": "2026-11-19T06:54:44Z",
  "staged_path": "docs/migration-errata/v0.30.0-restage-fixture.md"
}
`

// healthyShapeBase is the real-card shape as a mutable map for arm fixtures.
func healthyShapeBase(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(realErrataCard1), &m); err != nil {
		t.Fatalf("parse healthy base: %v", err)
	}
	return m
}

// marshalCard re-serializes an arm fixture.
func marshalCard(t *testing.T, m map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal arm card: %v", err)
	}
	return raw
}

// refuse asserts the verdict is invalid, names wantField, and (when
// wantProblem != "") carries that problem-text fragment.
func refuse(t *testing.T, raw []byte, wantField, wantProblem string) *StagedErrataShape {
	t.Helper()
	shape := CheckStagedErrataShape(raw)
	if shape.Valid {
		t.Fatalf("want REFUSE (field %q), got VALID", wantField)
	}
	if wantField != "" && !containsField(shape.OffendingFields, wantField) {
		t.Errorf("offending fields %v want %q; problems: %v", shape.OffendingFields, wantField, shape.Problems)
	}
	if wantProblem != "" && !problemHas(shape.Problems, wantProblem) {
		t.Errorf("problems %v want fragment %q", shape.Problems, wantProblem)
	}
	return shape
}

func containsField(fields []string, want string) bool {
	for _, f := range fields {
		if f == want {
			return true
		}
	}
	return false
}

func problemHas(problems []string, fragment string) bool {
	for _, p := range problems {
		if strings.Contains(p, fragment) {
			return true
		}
	}
	return false
}

// TestCheckStagedErrataShape_RealCardsPass: the two byte-faithful real-card
// fixtures (the verified template the allowlist was enumerated from) PASS.
func TestCheckStagedErrataShape_RealCardsPass(t *testing.T) {
	for name, raw := range map[string]string{
		"hub-token-normalization": realErrataCard1,
		"preserved-file-skew":     realErrataCard2,
	} {
		shape := CheckStagedErrataShape([]byte(raw))
		if !shape.Valid {
			t.Errorf("real card %s must PASS the shape verifier; problems: %v", name, shape.Problems)
		}
	}
}

// TestCheckStagedErrataShape_OptionalFieldsPass: the optional creation/
// transport fields (report_envelope, owner_notes, open_questions,
// next_action) and an explicit-null workstream_slug are accepted on the
// healthy base.
func TestCheckStagedErrataShape_OptionalFieldsPass(t *testing.T) {
	m := healthyShapeBase(t)
	m["report_envelope"] = "minimal"
	m["owner_notes"] = []any{"rescope:2026-11-19 note"}
	m["open_questions"] = []any{}
	m["next_action"] = "awaiting release ceremony"
	m["workstream_slug"] = nil
	if shape := CheckStagedErrataShape(marshalCard(t, m)); !shape.Valid {
		t.Fatalf("healthy base + optional creation/transport fields must PASS; problems: %v", shape.Problems)
	}
}

// TestCheckStagedErrataShape_ExecutionEraContentRefused: one arm per
// execution-era key, each carrying CONTENT. Even a benign/valid value makes
// the card uncheckable: staged errata cards never execute, so execution-era
// state has no business on one. (Round-5 contract: the canonical EMPTY
// states — null/absent/empty-array — are legal because the sanctioned
// restage op materializes them; see the canonical-empty PASS test below.
// The CONTENT arms remain refusals; against the OLD dirty implementation
// all of these were refusals too — receipt tmp/red-shape-probe.txt — and
// the round-5 FIX 2 change is the EMPTY-state flip, pinned RED in
// tmp/red-round5-fix2.txt.)
func TestCheckStagedErrataShape_ExecutionEraContentRefused(t *testing.T) {
	arms := map[string]func(m map[string]any){
		"latest_report": func(m map[string]any) {
			m["latest_report"] = map[string]any{"path": "a.md", "status": "completed", "report_envelope": "minimal"}
		},
		"last_review": func(m map[string]any) {
			m["last_review"] = map[string]any{"path": "a.md", "reviewed_at": "2026-11-11T18:16:10Z", "title": "R", "status": "ready"}
		},
		"review_paths":         func(m map[string]any) { m["review_paths"] = []any{"reports/x.md"} },
		"report_paths":         func(m map[string]any) { m["report_paths"] = []any{"reports/x.md"} },
		"measured_outcome":     func(m map[string]any) { m["measured_outcome"] = "shipped" },
		"decision_record":      func(m map[string]any) { m["decision_record"] = map[string]any{"entries": []any{}} },
		"session_aliases":      func(m map[string]any) { m["session_aliases"] = []any{"ses-owner"} },
		"active_session_alias": func(m map[string]any) { m["active_session_alias"] = "some-session" },
		"claimed_at":           func(m map[string]any) { m["claimed_at"] = "2026-11-11T18:16:10Z" },
	}
	for field, mutate := range arms {
		m := healthyShapeBase(t)
		mutate(m)
		refuse(t, marshalCard(t, m), field, fmt.Sprintf("field %s carries execution-era content for a staged errata card", field))
	}
}

// TestCheckStagedErrataShape_ExecutionEraCanonicalEmptyPasses (round-5 FIX 2):
// the canonical EMPTY states of every execution-era key are LEGAL — exactly
// what the sanctioned restage op writes (defaultCoordinationTaskPayload
// materialization through the canonical write). Covers the producer states
// (aliases/report/review path lists as EMPTY ARRAYS, scalars as null), the
// null variants of the list-tolerant keys, and (via healthyShapeBase) the
// absent variants.
func TestCheckStagedErrataShape_ExecutionEraCanonicalEmptyPasses(t *testing.T) {
	// Producer states: empty arrays + nulls.
	m := healthyShapeBase(t)
	m["session_aliases"] = []any{}
	m["active_session_alias"] = nil
	m["claimed_at"] = nil
	m["report_paths"] = []any{}
	m["review_paths"] = []any{}
	m["latest_report"] = nil
	m["decision_record"] = nil
	m["measured_outcome"] = nil
	m["last_review"] = nil
	if shape := CheckStagedErrataShape(marshalCard(t, m)); !shape.Valid {
		t.Fatalf("canonical-empty execution-era keys (producer states) must PASS; problems: %v", shape.Problems)
	}
	// Null is equivalent to absent for the list-tolerant keys too.
	m2 := healthyShapeBase(t)
	m2["session_aliases"] = nil
	m2["report_paths"] = nil
	m2["review_paths"] = nil
	if shape := CheckStagedErrataShape(marshalCard(t, m2)); !shape.Valid {
		t.Fatalf("null list-tolerant execution-era keys must PASS; problems: %v", shape.Problems)
	}
	// Absent variants: healthyShapeBase carries none of these keys — its
	// PASS in the real-cards test already pins that arm.
}

// TestCheckStagedErrataShape_CanonicalDefaultKeysCanonicalEmptyPass (round-5
// FIX 2): the remaining defaultCoordinationTaskPayload keys the canonical
// write materializes are legal at their canonical EMPTY states (the restage
// producer contract). The full-producer-shape integration pin is the
// restageProducedCardFixture (release_gate_test.go / release_inject_test.go);
// this unit arm proves each key individually on the healthy template base.
func TestCheckStagedErrataShape_CanonicalDefaultKeysCanonicalEmptyPass(t *testing.T) {
	m := healthyShapeBase(t)
	m["research_question"] = ""
	m["source_policy"] = nil
	m["source_allowlist"] = []any{}
	m["desired_artifact_type"] = nil
	m["target_artifact_path"] = nil
	m["ready_criteria"] = []any{}
	m["constraints"] = []any{}
	m["non_goals"] = []any{}
	m["backlog_id"] = nil
	m["dependencies"] = []any{}
	m["f3_design_readiness"] = nil
	m["predicted_impact"] = nil
	if shape := CheckStagedErrataShape(marshalCard(t, m)); !shape.Valid {
		t.Fatalf("canonical-empty canonical-default keys must PASS; problems: %v", shape.Problems)
	}
}

// TestCheckStagedErrataShape_CanonicalDefaultKeysContentRefused (round-5
// FIX 2): any CONTENT in a canonical-default key refuses the card — the
// allowance is empty-state-only (a staged errata card is not a research or
// execution card; only what the restage producer writes is canonical).
func TestCheckStagedErrataShape_CanonicalDefaultKeysContentRefused(t *testing.T) {
	arms := map[string]any{
		"research_question":     "why does this happen?",
		"source_policy":         "allowlist",
		"source_allowlist":      []any{"https://example.com"},
		"desired_artifact_type": "decision_memo",
		"target_artifact_path":  "researches/decisions/x.md",
		"ready_criteria":        []any{"criteria met"},
		"constraints":           []any{"no widening"},
		"non_goals":             []any{"no rewrite"},
		"backlog_id":            "P1-CORE-001",
		"dependencies":          []any{"other-task"},
		"f3_design_readiness":   map[string]any{"design_digest": "abc"},
		"predicted_impact":      "high",
	}
	for field, value := range arms {
		m := healthyShapeBase(t)
		m[field] = value
		refuse(t, marshalCard(t, m), field, fmt.Sprintf("field %s carries non-canonical content for a staged errata card", field))
	}
}

// TestCheckStagedErrataShape_WorkstreamSlugNonNullRefused: workstream_slug
// is legal ONLY as an explicit null; any binding is execution-era state.
// Verified HEALTHY by the OLD implementation (probe receipt).
func TestCheckStagedErrataShape_WorkstreamSlugNonNullRefused(t *testing.T) {
	m := healthyShapeBase(t)
	m["workstream_slug"] = "some-stream"
	refuse(t, marshalCard(t, m), "workstream_slug", "unexpected field workstream_slug")
}

// TestCheckStagedErrataShape_UnknownKeyRefused: any key outside the
// enumerated allowlist refuses the card — the fail-closed-by-construction
// core (no parity surface left to get wrong). Verified HEALTHY by the OLD
// implementation (probe receipt).
func TestCheckStagedErrataShape_UnknownKeyRefused(t *testing.T) {
	m := healthyShapeBase(t)
	m["surprise_field"] = 1
	refuse(t, marshalCard(t, m), "surprise_field", "unexpected field surprise_field for a staged errata card; clean the card or withdraw it from staging")
}

// TestCheckStagedErrataShape_MissingRequiredRefused: deleting ANY required
// core field refuses the card with that field named.
func TestCheckStagedErrataShape_MissingRequiredRefused(t *testing.T) {
	for _, field := range stagedErrataRequiredFields {
		m := healthyShapeBase(t)
		delete(m, field)
		refuse(t, marshalCard(t, m), field, "")
	}
}

// TestCheckStagedErrataShape_EmptyStringsRefused: present-but-empty core
// strings refuse — including whitespace-only and U+FEFF-only values under
// the harness-owned emptiness rule.
func TestCheckStagedErrataShape_EmptyStringsRefused(t *testing.T) {
	for _, title := range []string{"", "   ", "\uFEFF", "\uFEFF \uFEFF"} {
		m := healthyShapeBase(t)
		m["title"] = title
		refuse(t, marshalCard(t, m), "title", "required field title must be a non-empty string")
	}
	// Empty string fields elsewhere.
	for _, field := range []string{"task_id", "created_at", "updated_at", "primary_lane", "staged_path"} {
		m := healthyShapeBase(t)
		m[field] = "\uFEFF  "
		refuse(t, marshalCard(t, m), field, "")
	}
}

// TestCheckStagedErrataShape_EmptyListEntriesRefused: U+FEFF-only and
// whitespace-only entries inside the required lists refuse (our own
// emptiness rule — no JS-parity claim).
func TestCheckStagedErrataShape_EmptyListEntriesRefused(t *testing.T) {
	for _, field := range []string{"files_in_scope", "rough_scope", "success_criteria", "validation_plan"} {
		for _, entry := range []string{"  ", "\uFEFF"} {
			m := healthyShapeBase(t)
			m[field] = []any{entry}
			refuse(t, marshalCard(t, m), field, "empty entries")
		}
		// Empty list itself refuses too.
		m := healthyShapeBase(t)
		m[field] = []any{}
		refuse(t, marshalCard(t, m), field, fmt.Sprintf("required field %s must be a non-empty list", field))
	}
}

// TestCheckStagedErrataShape_InvalidEnumsRefused: the narrow enum set.
// (The OLD implementation also degraded invalid enums — these arms keep
// coverage but are not red receipts.)
func TestCheckStagedErrataShape_InvalidEnumsRefused(t *testing.T) {
	arms := []struct {
		field string
		value any
	}{
		{"task_type", "banana"},
		{"task_type", ""},
		{"coordination_mode", "medium-ish"},
		{"status", "draft"},
		{"status", "completed"},
		{"status", ""},
		{"report_envelope", "huge"},
		{"report_envelope", ""},
	}
	for _, arm := range arms {
		m := healthyShapeBase(t)
		m[arm.field] = arm.value
		refuse(t, marshalCard(t, m), arm.field, fmt.Sprintf("field %s must be one of:", arm.field))
	}
}

// TestCheckStagedErrataShape_BadStagedPathsRefused: the cross-platform
// repo-relative rules (state-lib isNonRepoRelativeStagedPath, Go-side):
// POSIX-absolute, Windows drive-absolute (either separator), UNC, and any
// ".." parent-traversal segment.
func TestCheckStagedErrataShape_BadStagedPathsRefused(t *testing.T) {
	for _, p := range []string{
		"/etc/passwd",
		"C:/docs/x.md",
		`C:\docs\x.md`,
		`\\server\share\x.md`,
		"docs/../docs/x.md",
		"..",
		"docs/..",
	} {
		m := healthyShapeBase(t)
		m["staged_path"] = p
		refuse(t, marshalCard(t, m), "staged_path", "staged_path must be a repo-relative path")
	}
}

// TestCheckStagedErrataShape_GoodStagedPathsPass: the shapes the JS guard
// also allows (relative, nested, dot-segments other than "..").
func TestCheckStagedErrataShape_GoodStagedPathsPass(t *testing.T) {
	for _, p := range []string{
		"docs/migration-errata/v0.29.0.md",
		"docs/./migration-errata/v0.29.0.md",
		"deeply/nested/dir/erratum.md",
	} {
		m := healthyShapeBase(t)
		m["staged_path"] = p
		if shape := CheckStagedErrataShape(marshalCard(t, m)); !shape.Valid {
			t.Errorf("staged_path %q must pass; problems: %v", p, shape.Problems)
		}
	}
}

// TestCheckStagedErrataShape_StagedPathRawVsTrimmedRefused (round-5 FIX 1):
// any staged_path whose TrimSpace form differs from the raw value is refused
// OUTRIGHT. Both release consumers (release_inject.go:289-301,
// release_gate.go:811-822) TrimSpace before filepath.IsAbs/Join, so a
// whitespace-prefixed " /outside/x.md" passed the OLD raw-string
// repo-relative rules while the consumer read the trimmed ABSOLUTE path — an
// out-of-repo read. RED receipt (guard temporarily reverted):
// tmp/red-round5-fix1.txt.
func TestCheckStagedErrataShape_StagedPathRawVsTrimmedRefused(t *testing.T) {
	for _, p := range []string{
		" /outside/x.md",        // whitespace-prefixed POSIX-absolute
		" ../outside.md",        // whitespace-prefixed parent traversal
		" C:/x.md",              // whitespace-prefixed Windows drive-letter
		" \\\\srv\\share\\x.md", // whitespace-prefixed UNC
		"docs/x.md ",            // trailing space (raw≠trimmed either side)
		"\tdocs/x.md",           // leading tab
		"docs/x.md\n",           // trailing newline
		"  docs/x.md",           // leading run — repo-relative BUT padded
	} {
		m := healthyShapeBase(t)
		m["staged_path"] = p
		refuse(t, marshalCard(t, m), "staged_path", "staged_path must not be whitespace-padded for a staged errata card: the release surfaces consume the trimmed value, so the consumed representation must equal the stored one")
	}
}

// TestCheckStagedErrataShape_WhitespaceWrappedBOMIsEmpty (round-5 c-F2
// ride-along): " \uFEFF " — space-wrapped BOM — is EMPTY under the harness
// emptiness rule. The old two-step trim (BOM-then-space) missed it (the
// outer spaces shield the BOM from Trim and the BOM shields the spaces from
// TrimSpace), so a U+FEFF-between-spaces title passed as non-empty.
func TestCheckStagedErrataShape_WhitespaceWrappedBOMIsEmpty(t *testing.T) {
	for _, field := range []string{"title", "task_id", "primary_lane"} {
		m := healthyShapeBase(t)
		m[field] = " \uFEFF "
		refuse(t, marshalCard(t, m), field, fmt.Sprintf("required field %s must be a non-empty string", field))
	}
	// Deeper interleavings at both edges, and inside list entries.
	for _, v := range []string{" \uFEFF \uFEFF ", "\uFEFF \uFEFF", " \t\uFEFF  ", " \uFEFF"} {
		if !shapeEmpty(v) {
			t.Errorf("shapeEmpty(%q) must be true (BOM+whitespace filler)", v)
		}
	}
	m := healthyShapeBase(t)
	m["success_criteria"] = []any{" \uFEFF "}
	refuse(t, marshalCard(t, m), "success_criteria", "empty entries")
}

// TestCheckStagedErrataShape_RestageProducedCardPasses (round-5 FIX 2): the
// byte-faithful output of the REAL sanctioned producer — derived by actually
// running restageCoordinationTask (rendered .opencode/scripts/state-lib.js)
// against a hermetic scratch registry — PASSes the verifier. The canonical
// write merges defaultCoordinationTaskPayload into the card, so it carries
// every execution-era and canonical-default key at its canonical EMPTY
// state; the pre-FIX-2 allowlist refused it as unexpected fields (RED
// receipt tmp/red-round5-fix2.txt), making sanctioned-path staged cards
// structurally un-injectable.
func TestCheckStagedErrataShape_RestageProducedCardPasses(t *testing.T) {
	shape := CheckStagedErrataShape([]byte(internalRestageProducedCard))
	if !shape.Valid {
		t.Fatalf("the real restage-produced card must PASS the shape verifier; problems: %v", shape.Problems)
	}
}

// TestCheckStagedErrataShape_WrongTypesRefused: the shape check is strict
// about JSON types — strings must be strings, lists lists, objects objects,
// schema_version the number 1.
func TestCheckStagedErrataShape_WrongTypesRefused(t *testing.T) {
	type arm struct {
		field string
		value any
	}
	for _, a := range []arm{
		{"title", 123},
		{"task_id", true},
		{"status", 5},
		{"schema_version", 2},
		{"schema_version", "1"},
		{"files_in_scope", "docs/x.md"},
		{"success_criteria", map[string]any{}},
		{"history", "a note"},
		{"history", []any{"not-an-object"}},
		{"report_envelope", 7},
		{"next_action", 9},
		{"owner_notes", "single-string"},
	} {
		m := healthyShapeBase(t)
		m[a.field] = a.value
		refuse(t, marshalCard(t, m), a.field, "")
	}
}

// TestCheckStagedErrataShape_HistoryShape: history must be a non-empty list
// of event objects, each with a non-empty event and at.
func TestCheckStagedErrataShape_HistoryShape(t *testing.T) {
	// Entry missing event.
	m := healthyShapeBase(t)
	m["history"] = []any{map[string]any{"at": "2026-11-11T18:16:10Z"}}
	refuse(t, marshalCard(t, m), "history", "history entry 0 is missing a non-empty event field")
	// Entry missing at.
	m = healthyShapeBase(t)
	m["history"] = []any{map[string]any{"event": "erratum_staged"}}
	refuse(t, marshalCard(t, m), "history", "history entry 0 is missing a non-empty at field")
	// Event/at present but empty after trimming.
	m = healthyShapeBase(t)
	m["history"] = []any{map[string]any{"event": "\uFEFF", "at": "  "}}
	refuse(t, marshalCard(t, m), "history", "")
	// Empty history list.
	m = healthyShapeBase(t)
	m["history"] = []any{}
	refuse(t, marshalCard(t, m), "history", "must be a non-empty list of event objects")
	// A repair-class event is as legal as a creation-class one.
	m = healthyShapeBase(t)
	m["history"] = []any{
		map[string]any{"at": "2026-11-07T06:38:29Z", "event": "erratum_staged", "session_name": "s", "status": "staged", "note": "n"},
		map[string]any{"at": "2026-11-12T06:38:29Z", "event": "task_updated", "session_name": nil, "status": "staged", "note": "repair"},
	}
	if shape := CheckStagedErrataShape(marshalCard(t, m)); !shape.Valid {
		t.Fatalf("creation+repair history must PASS; problems: %v", shape.Problems)
	}
}

// TestCheckStagedErrataShape_NonObjectJSONRefused: invalid JSON, a
// non-object top level, and trailing data are all shape violations.
func TestCheckStagedErrataShape_NonObjectJSONRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"invalid-json":  `{"title": "x",`,
		"top-level-str": `"just a string"`,
		"top-level-arr": `["not", "an", "object"]`,
		"trailing":      realErrataCard1 + "\n{}",
		"empty":         ``,
	} {
		shape := CheckStagedErrataShape([]byte(raw))
		if shape.Valid {
			t.Errorf("%s must be refused as uncheckable", name)
		}
		if !problemHas(shape.Problems, "must be a single JSON object") {
			t.Errorf("%s: want structure problem, got %v", name, shape.Problems)
		}
	}
}

// TestCheckStagedErrataShape_Deterministic: the verdict surface (problems
// order, offending fields order) is stable across calls.
func TestCheckStagedErrataShape_Deterministic(t *testing.T) {
	m := healthyShapeBase(t)
	m["measured_outcome"] = "shipped"
	m["title"] = "  "
	raw := marshalCard(t, m)
	a := CheckStagedErrataShape(raw)
	b := CheckStagedErrataShape(raw)
	if a.Valid || b.Valid {
		t.Fatalf("multi-violation card must refuse")
	}
	if strings.Join(a.Problems, "|") != strings.Join(b.Problems, "|") || strings.Join(a.OffendingFields, ",") != strings.Join(b.OffendingFields, ",") {
		t.Errorf("verdict not deterministic:\n%v\n%v", a.Problems, b.Problems)
	}
	// Canonical-empty class problems come first (lexical within the class,
	// execution-era before canonical-default), then per-field checks.
	if len(a.Problems) < 2 || !strings.HasPrefix(a.Problems[0], "field measured_outcome carries execution-era content") {
		t.Errorf("execution-era content problem must lead; got %v", a.Problems)
	}
	if !containsField(a.OffendingFields, "measured_outcome") || !containsField(a.OffendingFields, "title") {
		t.Errorf("offending fields must name both violators; got %v", a.OffendingFields)
	}
}
