// staged_validation.go — the staged-errata card SHAPE VERIFIER.
//
// Card defer-release-injector-degraded-eligibility (disposal b′, rescope
// 2026-11-19 after the 3-rejection escalation): staged errata cards are a
// NARROW, KNOWN shape — they are created directly into staging (the
// erratum_staged writer / the sanctioned restage op) and NEVER execute. The
// release injector (internal/cli/release_inject.go) and doctor check #13
// (internal/cli/release_gate.go checkStagedErrataContent) therefore do not
// need the general coordination-card health semantics (the JS
// normalizeCoordinationTaskRecordWithDiagnostics degradation machinery): they
// need to know that a selected staged errata card IS that known shape and
// carries NO execution-era or unexpected state.
//
// THE CONTRACT (deliberately narrower-and-stricter than general card health):
//
//   - ALLOWLIST. The only keys a staged errata card may carry are the
//     creation/transport fields the two real staged cards
//     (.local/coordinator/tasks/errata-v0290-hub-token-normalization.json,
//     errata-v0290-preserved-file-skew.json — the verified template) carry —
//     schema_version, task_id, title, status, staged_path, files_in_scope,
//     rough_scope, history, created_at, updated_at, task_type,
//     coordination_mode, primary_lane, success_criteria, validation_plan —
//     plus the optional creation/transport fields report_envelope,
//     owner_notes, open_questions, next_action, and workstream_slug which is
//     legal ONLY as an explicit null. ANY other key → uncheckable.
//
//   - RESTAGE PRODUCER CONTRACT (round-5 FIX 2, 2026-11-19). The sanctioned
//     restage op (state-lib.js restageCoordinationTask) writes through the
//     canonical coordination-task write, which merges
//     defaultCoordinationTaskPayload (state-lib.js:992) into every card it
//     saves (state-lib.js:1757) — so a restage-produced staged card
//     MATERIALIZES the execution-era keys (session_aliases,
//     active_session_alias, claimed_at, report_paths, review_paths,
//     latest_report, decision_record, measured_outcome, last_review) and the
//     other canonical-default keys (research_question, source_policy,
//     source_allowlist, desired_artifact_type, target_artifact_path,
//     ready_criteria, constraints, non_goals, backlog_id, dependencies,
//     f3_design_readiness, predicted_impact) at their canonical EMPTY
//     states. Those keys are therefore admitted by the allowlist ONLY at
//     their canonical EMPTY state — session_aliases/report_paths/
//     review_paths: absent, null, or empty array; the remaining
//     execution-era keys: absent or null (NO object/string content);
//     research_question: absent, null, or empty string; the remaining
//     canonical-default keys: absent/null, or empty array for the list
//     defaults. ANY non-empty / present-with-content value in one of those
//     keys refuses the card as execution-era (or non-canonical) content:
//     staged errata cards never execute, so only the empty state the
//     producer itself writes is legal. workstream_slug stays null-only and
//     unknown keys stay refused. (The empirical producer output — derived
//     by actually running restageCoordinationTask against a scratch
//     registry — is pinned byte-faithful as the Go test fixture
//     restageProducedCardFixture, internal/cli/release_gate_test.go.)
//
//   - REQUIRED FIELDS. Every template field above (except the optional
//     quartet) must be present, correctly typed, and non-empty, where
//     emptiness is a Go-native check that explicitly treats U+FEFF-only and
//     whitespace-only strings as empty (this harness owns the rule now — no
//     JS-parity claim, just conservative). Enum fields must be valid for the
//     narrow set: task_type, coordination_mode, primary_lane (non-empty
//     string, no vocabulary), status == "staged", report_envelope (when
//     present). The scope/criteria/validation lists must be non-empty with
//     non-empty entries. history must be a non-empty list of event objects,
//     each with a non-empty "event" and "at" (creation/repair-class events;
//     entry payloads beyond that are not inspected). staged_path must be
//     repo-relative: POSIX-absolute, Windows drive-letter, UNC, and ".."
//     parent-traversal forms are all refused — the same cross-platform rules
//     as state-lib's isNonRepoRelativeStagedPath (the JS write-side guard),
//     implemented Go-side. staged_path must ALSO be UNPADDED (round-5 FIX 1):
//     both release consumers (release_inject.go, release_gate.go check #13)
//     TrimSpace the value before filepath.IsAbs/Join, so any staged_path
//     whose TrimSpace form differs from the raw value is refused outright —
//     the consumed representation must equal the stored one (a
//     whitespace-prefixed " /outside/x.md" would pass the repo-relative
//     rules raw while the consumer reads the trimmed ABSOLUTE path — an
//     out-of-repo read).
//
//   - FAIL-CLOSED BY CONSTRUCTION. Unknown-key refusal means there is NO
//     parity surface left to get wrong: a card the verifier does not fully
//     understand is uncheckable, and both release surfaces treat uncheckable
//     as refuse. The problem text for the unexpected-field class is
//     "unexpected field <X> for a staged errata card; clean the card or
//     withdraw it from staging".
//
// PURITY: the verifier operates on raw card bytes only — no filesystem
// access, no repo root, no review-file channel (the prior JS-normalizer
// port's ReviewFileSource/OSReviewFileSource machinery is deleted).
package taskcard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// StagedErrataShape is the deterministic shape verdict for one raw staged
// errata card. Valid is true iff Problems is empty; Problems carries one
// entry per violation (stable order: unexpected fields first in lexical
// order, then per-field checks in canonical field order); OffendingFields is
// the deduped, lexically sorted set of field names named by the problems.
type StagedErrataShape struct {
	Valid           bool
	Problems        []string
	OffendingFields []string
}

// The narrow enum vocabularies for the staged-errata shape (the coordination
// task types / modes / envelopes the errata writers use; status has exactly
// one legal value for a card in staging).
var (
	stagedErrataTaskTypes      = []string{"implementation", "study", "research", "docs", "verification"}
	stagedErrataModes          = []string{"short", "medium", "long"}
	stagedErrataEnvelopes      = []string{"minimal", "standard", "synthesis"}
	stagedErrataStatuses       = []string{"staged"}
	stagedErrataSchemaVersions = "1"
)

// stagedErrataRequiredFields is the canonical per-field check order (every
// template field except the optional quartet).
var stagedErrataRequiredFields = []string{
	"schema_version",
	"task_id",
	"title",
	"status",
	"staged_path",
	"files_in_scope",
	"rough_scope",
	"history",
	"created_at",
	"updated_at",
	"task_type",
	"coordination_mode",
	"primary_lane",
	"success_criteria",
	"validation_plan",
}

// stagedErrataOptionalFields may be present (typed, checked) or absent.
var stagedErrataOptionalFields = []string{"report_envelope", "owner_notes", "open_questions", "next_action"}

// canonicalEmptyClass says HOW a key admitted by the restage producer
// contract may appear on a staged errata card (round-5 FIX 2): only at its
// canonical EMPTY state — the state defaultCoordinationTaskPayload
// materializes and the canonical write persists.
type canonicalEmptyClass int

const (
	// nullOnly: the key may be ABSENT or JSON null — nothing else (any
	// object/string/array content, including empty-but-typed values like
	// "" or {}, is execution-era or non-canonical content).
	nullOnly canonicalEmptyClass = iota
	// emptyList: ABSENT, JSON null, or an EMPTY JSON array (the producer's
	// list defaults — session_aliases, report_paths, review_paths, …).
	emptyList
	// emptyString: ABSENT, JSON null, or a string that is empty under the
	// harness emptiness rule (the producer's research_question default "").
	emptyString
)

// stagedErrataExecutionEraRules — the nine execution-era keys the canonical
// write materializes, each admitted ONLY at its canonical EMPTY state. Any
// content (a claimed alias, a report path, a review object, a measured
// outcome, …) is execution-era state: staged errata cards never execute.
var stagedErrataExecutionEraRules = map[string]canonicalEmptyClass{
	"session_aliases":      emptyList,
	"active_session_alias": nullOnly,
	"claimed_at":           nullOnly,
	"report_paths":         emptyList,
	"review_paths":         emptyList,
	"latest_report":        nullOnly,
	"decision_record":      nullOnly,
	"measured_outcome":     nullOnly,
	"last_review":          nullOnly,
}

// stagedErrataCanonicalDefaultRules — the remaining defaultCoordinationTaskPayload
// keys (NOT execution-era, but NOT part of the two-real-card template either)
// that every canonical write materializes. Admitted ONLY at their canonical
// EMPTY state, exactly as the sanctioned restage op writes them; content in
// one of these is non-canonical for a staged errata card. open_questions is
// NOT here — it is already an optional creation/transport list field.
var stagedErrataCanonicalDefaultRules = map[string]canonicalEmptyClass{
	"research_question":     emptyString,
	"source_policy":         nullOnly,
	"source_allowlist":      emptyList,
	"desired_artifact_type": nullOnly,
	"target_artifact_path":  nullOnly,
	"ready_criteria":        emptyList,
	"constraints":           emptyList,
	"non_goals":             emptyList,
	"backlog_id":            nullOnly,
	"dependencies":          emptyList,
	"f3_design_readiness":   nullOnly,
	"predicted_impact":      nullOnly,
}

// stagedErrataAllowlist is the complete key universe: required fields +
// optional fields + workstream_slug (null-only) + the restage-producer
// canonical-empty keys (execution-era + canonical-default). EVERYTHING else
// is an unexpected field.
var stagedErrataAllowlist = func() map[string]bool {
	m := map[string]bool{"workstream_slug": true}
	for _, f := range stagedErrataRequiredFields {
		m[f] = true
	}
	for _, f := range stagedErrataOptionalFields {
		m[f] = true
	}
	for f := range stagedErrataExecutionEraRules {
		m[f] = true
	}
	for f := range stagedErrataCanonicalDefaultRules {
		m[f] = true
	}
	return m
}()

// CheckStagedErrataShape verifies that raw is exactly the staged-errata card
// shape. It is pure: raw bytes in, verdict out, no environment. Invalid JSON,
// a non-object top level, or trailing data is itself a shape violation (the
// card is uncheckable), not an error — the caller refuses on !Valid.
func CheckStagedErrataShape(raw []byte) *StagedErrataShape {
	var problems []string
	offending := map[string]bool{}
	add := func(field, problem string) {
		problems = append(problems, problem)
		if field != "" {
			offending[field] = true
		}
	}

	src, derr := decodeStrictJSONObject(raw)
	if derr != nil {
		add("", fmt.Sprintf("staged errata card must be a single JSON object: %v", derr))
		return finishShape(problems, offending)
	}

	// 1. Unexpected fields first, in lexical order — the fail-closed core.
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if stagedErrataAllowlist[k] {
			continue
		}
		add(k, fmt.Sprintf("unexpected field %s for a staged errata card; clean the card or withdraw it from staging", k))
	}
	// workstream_slug is IN the allowlist but legal only as an explicit null
	// (a non-null workstream binding is execution-era state).
	if v, present := src["workstream_slug"]; present && v != nil {
		add("workstream_slug", "unexpected field workstream_slug for a staged errata card (non-null workstream binding is execution-era state); clean the card or withdraw it from staging")
	}

	// 1.5 Restage producer contract (round-5 FIX 2): the execution-era keys
	// and the other canonical-default keys are admitted ONLY at their
	// canonical EMPTY state — exactly the state the sanctioned restage op
	// writes. Content in any of them refuses the card.
	checkCanonicalEmptyClass(src, stagedErrataExecutionEraRules,
		"carries execution-era content for a staged errata card (staged errata cards never execute; only the canonical EMPTY state — null/absent, or an empty array for the alias/report/review path lists — is accepted)", add)
	checkCanonicalEmptyClass(src, stagedErrataCanonicalDefaultRules,
		"carries non-canonical content for a staged errata card (only the canonical EMPTY state produced by the sanctioned restage op is accepted)", add)

	// 2. schema_version — must be the number 1.
	if v, present := src["schema_version"]; !present {
		add("schema_version", "required field schema_version is missing or empty for a staged errata card")
	} else if n, ok := v.(float64); !ok || n != 1 {
		add("schema_version", fmt.Sprintf("field schema_version must be the number %s for a staged errata card", stagedErrataSchemaVersions))
	}

	// 3. Required plain strings.
	for _, f := range []string{"task_id", "title", "created_at", "updated_at", "primary_lane"} {
		checkRequiredString(src, f, add)
	}

	// 4. status — the one-value enum.
	if v, present := src["status"]; !present {
		add("status", "required field status is missing or empty for a staged errata card")
	} else if s, ok := v.(string); !ok {
		add("status", "field status must be one of: staged.")
	} else if !enumShapeOK(s, stagedErrataStatuses) {
		add("status", fmt.Sprintf("field status must be one of: %s.", strings.Join(stagedErrataStatuses, ", ")))
	}

	// 5. staged_path — non-empty string, UNPADDED and repo-relative (round-5
	//    FIX 1). Both release consumers (release_inject.go:289-301,
	//    release_gate.go:811-822) TrimSpace the value before
	//    filepath.IsAbs/Join, so a staged_path whose TrimSpace form differs
	//    from the raw value is refused OUTRIGHT — the consumed
	//    representation must equal the stored one. Only when raw == trimmed
	//    do the existing repo-relative rules apply (to the identical value).
	if checkRequiredString(src, "staged_path", add) {
		s, _ := src["staged_path"].(string)
		switch {
		case stagedPathPadded(s):
			add("staged_path", fmt.Sprintf("staged_path must not be whitespace-padded for a staged errata card: the release surfaces consume the trimmed value, so the consumed representation must equal the stored one (%q)", s))
		case nonRepoRelativeStagedPath(s):
			add("staged_path", fmt.Sprintf("staged_path must be a repo-relative path for a staged errata card (POSIX-absolute, Windows drive-letter, UNC, and parent-traversal forms are refused): %q", s))
		}
	}

	// 6. Required string lists.
	for _, f := range []string{"files_in_scope", "rough_scope", "success_criteria", "validation_plan"} {
		checkRequiredStringList(src, f, add)
	}

	// 7. task_type / coordination_mode enums.
	for _, ec := range []struct {
		field  string
		allow  []string
		option bool
	}{
		{"task_type", stagedErrataTaskTypes, false},
		{"coordination_mode", stagedErrataModes, false},
		{"report_envelope", stagedErrataEnvelopes, true},
	} {
		v, present := src[ec.field]
		if !present || v == nil {
			if ec.option {
				continue
			}
			add(ec.field, fmt.Sprintf("required field %s is missing or empty for a staged errata card", ec.field))
			continue
		}
		s, ok := v.(string)
		if !ok || !enumShapeOK(s, ec.allow) {
			add(ec.field, fmt.Sprintf("field %s must be one of: %s.", ec.field, strings.Join(ec.allow, ", ")))
		}
	}

	// 8. history — non-empty list of event objects with non-empty event/at.
	if v, present := src["history"]; !present {
		add("history", "required field history is missing or empty for a staged errata card")
	} else {
		checkHistoryShape(v, add)
	}

	// 9. Optional fields (absent is fine; present must be well-formed).
	if v, present := src["owner_notes"]; present && v != nil {
		checkOptionalStringList(v, "owner_notes", add)
	}
	if v, present := src["open_questions"]; present && v != nil {
		checkOptionalStringList(v, "open_questions", add)
	}
	if v, present := src["next_action"]; present && v != nil {
		if s, ok := v.(string); !ok || shapeEmpty(s) {
			add("next_action", "optional field next_action must be a non-empty string when present")
		}
	}

	return finishShape(problems, offending)
}

// checkCanonicalEmptyClass enforces the restage producer contract (round-5
// FIX 2) for one rule table: every key in rules that is PRESENT on the card
// must be at its canonical EMPTY state (per its canonicalEmptyClass). Keys
// that are absent or JSON null pass trivially; any content refuses with the
// class-specific problem text. Iterates the rule keys in lexical order so
// the problem order stays deterministic.
func checkCanonicalEmptyClass(src map[string]interface{}, rules map[string]canonicalEmptyClass, problemText string, add func(string, string)) {
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, present := src[k]
		if !present || v == nil {
			continue
		}
		empty := false
		switch rules[k] {
		case emptyList:
			if list, isList := v.([]interface{}); isList && len(list) == 0 {
				empty = true
			}
		case emptyString:
			if s, isStr := v.(string); isStr && shapeEmpty(s) {
				empty = true
			}
		case nullOnly:
			// present with a non-null value — never empty.
		}
		if !empty {
			add(k, fmt.Sprintf("field %s %s; withdraw from staging or clean the card", k, problemText))
		}
	}
}

// stagedPathPadded reports whether the staged_path's TrimSpace form differs
// from the raw value — i.e. the release consumers (which TrimSpace before
// filepath.IsAbs/Join) would consume a DIFFERENT representation than the one
// stored/validated. Matches Go's strings.TrimSpace exactly (Unicode
// whitespace; not the BOM), because that is what both consumers run.
func stagedPathPadded(p string) bool {
	return strings.TrimSpace(p) != p
}

// finishShape assembles the verdict and sorts the offending-field set
// lexically for a deterministic diagnostic surface.
func finishShape(problems []string, offending map[string]bool) *StagedErrataShape {
	fields := make([]string, 0, len(offending))
	for f := range offending {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	return &StagedErrataShape{
		Valid:           len(problems) == 0,
		Problems:        problems,
		OffendingFields: fields,
	}
}

// checkRequiredString enforces presence + JSON string + non-emptiness (the
// Go-native rule: U+FEFF-only and whitespace-only are empty). Returns true
// when the field is a present, non-empty string.
func checkRequiredString(src map[string]interface{}, field string, add func(string, string)) bool {
	v, present := src[field]
	if !present || v == nil {
		add(field, fmt.Sprintf("required field %s is missing or empty for a staged errata card", field))
		return false
	}
	s, ok := v.(string)
	if !ok || shapeEmpty(s) {
		add(field, fmt.Sprintf("required field %s must be a non-empty string", field))
		return false
	}
	return true
}

// checkRequiredStringList enforces presence + JSON array + non-empty list +
// every entry a non-empty string under the same emptiness rule.
func checkRequiredStringList(src map[string]interface{}, field string, add func(string, string)) {
	v, present := src[field]
	if !present || v == nil {
		add(field, fmt.Sprintf("required field %s is missing or empty for a staged errata card", field))
		return
	}
	list, ok := v.([]interface{})
	if !ok || len(list) == 0 {
		add(field, fmt.Sprintf("required field %s must be a non-empty list of non-empty strings", field))
		return
	}
	for i, el := range list {
		s, ok := el.(string)
		if !ok || shapeEmpty(s) {
			add(field, fmt.Sprintf("field %s must not contain empty entries (entry %d is empty after trimming)", field, i))
			return
		}
	}
}

// checkOptionalStringList enforces the present-but-optional list shape: a
// JSON array (possibly empty — absence-equivalent) whose entries are all
// non-empty strings.
func checkOptionalStringList(v interface{}, field string, add func(string, string)) {
	list, ok := v.([]interface{})
	if !ok {
		add(field, fmt.Sprintf("optional field %s must be a list of non-empty strings when present", field))
		return
	}
	for i, el := range list {
		s, ok := el.(string)
		if !ok || shapeEmpty(s) {
			add(field, fmt.Sprintf("optional field %s must not contain empty entries (entry %d is empty after trimming)", field, i))
			return
		}
	}
}

// checkHistoryShape enforces the history shape: non-empty list, each entry a
// JSON object carrying a non-empty "event" and a non-empty "at". Entry
// payloads beyond event/at (session_name, status, note, …) are not inspected
// — the shape check is over the card's top-level key universe; history entry
// interiors get this one light pass only.
func checkHistoryShape(v interface{}, add func(string, string)) {
	list, ok := v.([]interface{})
	if !ok || len(list) == 0 {
		add("history", "required field history must be a non-empty list of event objects")
		return
	}
	for i, el := range list {
		entry, ok := el.(map[string]interface{})
		if !ok {
			add("history", fmt.Sprintf("history entry %d must be a JSON object", i))
			continue
		}
		ev, ok := entry["event"].(string)
		if !ok || shapeEmpty(ev) {
			add("history", fmt.Sprintf("history entry %d is missing a non-empty event field", i))
		}
		at, ok := entry["at"].(string)
		if !ok || shapeEmpty(at) {
			add("history", fmt.Sprintf("history entry %d is missing a non-empty at field", i))
		}
	}
}

// enumShapeOK reports whether s (untrimmed comparison — the value must BE the
// enum literal) is one of allowed.
func enumShapeOK(s string, allowed []string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

// shapeEmpty is the harness-owned emptiness rule: leading/trailing U+FEFF
// (the BOM artifact) AND Unicode whitespace are trimmed TOGETHER in one pass
// (round-5 c-F2 ride-along: the previous BOM-then-space two-step trim missed
// space-wrapped-BOM mixtures like " \uFEFF ", where the outer spaces shield
// the BOM from Trim and the BOM shields the spaces from TrimSpace); what
// remains empty counts as empty. Deliberately conservative; no JS-parity
// claim.
func shapeEmpty(s string) bool {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == '\uFEFF' || unicode.IsSpace(r)
	}) == ""
}

// --- staged_path guard (state-lib isNonRepoRelativeStagedPath, Go-side) ----

var (
	// windowsDriveAbsolute matches "C:/x" and "C:\x" drive-absolute forms.
	windowsDriveAbsolute = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	// pathSegmentSplit splits on runs of either separator, like the JS
	// stagedPath.split(/[\\/]+/).
	pathSegmentSplit = regexp.MustCompile(`[\\/]+`)
)

// nonRepoRelativeStagedPath mirrors state-lib's isNonRepoRelativeStagedPath
// (the JS write-side guard both staging paths share): POSIX-absolute,
// Windows drive-absolute, UNC, and any ".." parent-traversal segment make a
// staged_path non-repo-relative. The release injector joins only non-absolute
// paths under the repo root, so any of these forms is an out-of-repo read at
// inject time.
func nonRepoRelativeStagedPath(p string) bool {
	return strings.HasPrefix(p, "/") ||
		windowsDriveAbsolute.MatchString(p) ||
		strings.HasPrefix(p, `\\`) ||
		slicesContains(pathSegmentSplit.Split(p, -1), "..")
}

// slicesContains is the tiny contains helper (no generics dependency needed).
func slicesContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// decodeStrictJSONObject parses exactly one JSON document and returns its
// top-level object; trailing data, decode failure, or a non-object top level
// are errors (the caller reports the card as uncheckable).
func decodeStrictJSONObject(raw []byte) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var inst interface{}
	if err := dec.Decode(&inst); err != nil {
		return nil, err
	}
	var trailing interface{}
	if err := dec.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("trailing data after the top-level JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	m, ok := inst.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("top-level JSON value is not an object")
	}
	return m, nil
}
