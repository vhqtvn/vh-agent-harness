package taskcard

// validator_test.go — the GATE for the task-card contract validator
// (defer-018). This is the faithful Go port of the fixtures in the retired
// templates/core/.opencode/scripts/verify-task-card-schema.py: the same base
// task, the same per-fixture mutations, the same three verdict classes
// (validate_ok / validate_fail / validate_fail_ack). It runs in `go test ./...`
// with NO Python and NO pip — exactly the architectural correction defer-018
// requires (a standalone Python script with a `pip install jsonschema`
// dependency is an outlier in a Go repo's gate).
//
// Beyond the 20 ported fixtures (6 ok, 13 schema-reject, 1 ack-reject) this
// table adds explicit coverage the Python harness implied but never asserted:
// additionalProperties violations (top-level, recurrence block, evidence item)
// and nested evidence/aliases shape violations (missing ref/kind, empty kind,
// empty alias id). The generic draft-07 subset validator handles all of them;
// these rows prove the gate catches them.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// baseReadyJSON is build_base_task() from the Python validator verbatim (a
// status=ready research card; the schema's research-then + ready-else branches
// are fully exercised by it). Decoded once per fixture via mustCard (json.Unmarshal
// yields float64 numbers, which the validator accepts).
const baseReadyJSON = `{
  "schema_version": 1,
  "task_id": "task-verify-schema",
  "title": "Verify task-card schema",
  "task_type": "research",
  "coordination_mode": "medium",
  "primary_lane": "repo",
  "research_question": "How should long-running research be prepared and resumed?",
  "source_policy": "web_repo",
  "source_allowlist": ["docs.anthropic.com", "openai.com"],
  "desired_artifact_type": "sources",
  "target_artifact_path": "researches/sources/2026-04-30-long-research-workflow-sources.md",
  "rough_scope": [],
  "open_questions": [],
  "ready_criteria": [],
  "files_in_scope": ["docs/coordination/schemas/task-card.schema.json"],
  "constraints": ["Keep validation self-contained."],
  "non_goals": ["No product-code changes."],
  "success_criteria": ["Schema fixtures validate correctly."],
  "validation_plan": ["Run vh-agent-harness task-card validate."],
  "report_envelope": "standard",
  "backlog_id": "P0-REPO-062",
  "workstream_slug": null,
  "dependencies": [],
  "owner_notes": [],
  "status": "ready",
  "session_aliases": ["verify-schema"],
  "active_session_alias": null,
  "claimed_at": null,
  "report_paths": [],
  "review_paths": [],
  "latest_report": null,
  "next_action": "Resume when needed.",
  "last_review": null,
  "history": [
    {
      "at": "2026-04-30T00:00:00Z",
      "event": "task_created",
      "session_name": "verify-schema",
      "status": "ready",
      "note": "Fixture created for schema validation."
    }
  ],
  "created_at": "2026-04-30T00:00:00Z",
  "updated_at": "2026-04-30T00:00:00Z"
}`

// recurrenceBlockJSON is build_recurrence_block() from the Python validator
// verbatim: a well-formed recurrence block (identity + ack pair + evidence +
// alias). Domain-free values so the embedded corpus leaks no project specifics.
const recurrenceBlockJSON = `{
  "recurrence_id": "exact-defect-canonical-example",
  "symptom_class_id": "recurrence.v1/example-symptom-class",
  "recurrence_count": 2,
  "last_acknowledged_count": 1,
  "evidence": [
    {
      "kind": "path",
      "ref": "pkg/example/handler.go",
      "note": "Recurring loop observed in the example handler."
    },
    {
      "kind": "outcome",
      "ref": "review-observation-001",
      "note": "Second observation of the same symptom class."
    }
  ],
  "aliases": [
    {
      "recurrence_id": "exact-defect-prior-alias",
      "superseded": true,
      "note": "Re-pointed after later evidence."
    }
  ]
}`

// mustCard decodes a JSON card body into a fresh map (the Go analogue of the
// Python deepcopy(ready_task): every fixture starts from an independent copy).
func mustCard(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var c map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("fixture decode: %v", err)
	}
	return c
}

// mustBlock decodes the recurrence block JSON into a fresh map.
func mustBlock(t *testing.T) map[string]interface{} {
	t.Helper()
	return mustCard(t, recurrenceBlockJSON)
}

// validCardMinimalJSON is a minimal valid task card (task_type=implementation
// avoids the research-then branch; status=draft with rough_scope satisfies the
// draft-then anyOf). Used by the trailing-data / precision byte-level tests.
const validCardMinimalJSON = `{
  "task_id": "t1",
  "title": "T",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "repo",
  "report_envelope": "standard",
  "status": "draft",
  "session_aliases": [],
  "active_session_alias": null,
  "claimed_at": null,
  "report_paths": [],
  "review_paths": [],
  "rough_scope": ["x"],
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z",
  "updated_at": "2026-04-30T00:00:00Z"
}`

// asObj asserts a decoded value is a JSON object and returns it (fails the test
// otherwise). Used to reach nested structures (history[0], last_review,
// recurrence) for per-fixture mutation.
func asObj(t *testing.T, v interface{}) map[string]interface{} {
	t.Helper()
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T", v)
	}
	return m
}

// hist0 returns history[0] as an object (every fixture card has one entry).
func hist0(t *testing.T, c map[string]interface{}) map[string]interface{} {
	t.Helper()
	arr := c["history"].([]interface{})
	return asObj(t, arr[0])
}

// verdict classes mirror the Python helpers.
const (
	verdictOK       = "ok"          // validate_ok:  schema passes AND ack-pair holds.
	verdictFailJSON = "fail_schema" // validate_fail: schema rejects.
	verdictFailAck  = "fail_ack"    // validate_fail_ack: schema passes, ack-pair guard rejects.
)

// fixture is one ported assertion: a name, a builder for the card, and the
// expected verdict.
type fixture struct {
	name    string
	build   func(t *testing.T) map[string]interface{}
	verdict string
}

// portedFixtures are the 20 assertions from verify-task-card-schema.py main().
func portedFixtures() []fixture {
	// Shared builders mirroring the Python base + status variants.
	ready := func(t *testing.T) map[string]interface{} { return mustCard(t, baseReadyJSON) }

	draft := func(t *testing.T) map[string]interface{} {
		c := ready(t)
		c["status"] = "draft"
		c["rough_scope"] = []interface{}{"Map the remaining coordinator edge cases."}
		c["files_in_scope"] = []interface{}{}
		c["success_criteria"] = []interface{}{}
		c["validation_plan"] = []interface{}{}
		c["active_session_alias"] = nil
		c["claimed_at"] = nil
		hist0(t, c)["status"] = "draft"
		return c
	}

	working := func(t *testing.T) map[string]interface{} {
		c := ready(t)
		c["status"] = "working"
		c["active_session_alias"] = "verify-subagent"
		c["claimed_at"] = "2026-04-30T00:05:00Z"
		c["session_aliases"] = []interface{}{"verify-schema", "verify-subagent"}
		hist0(t, c)["status"] = "working"
		return c
	}

	reviewed := func(t *testing.T) map[string]interface{} {
		c := ready(t)
		c["status"] = "completed"
		c["review_paths"] = []interface{}{
			".local/{{COORDINATOR_DIR}}/reports/task-verify-schema/2026-04-30T00-15-00Z-review.md",
		}
		c["last_review"] = map[string]interface{}{
			"path":         ".local/{{COORDINATOR_DIR}}/reports/task-verify-schema/2026-04-30T00-15-00Z-review.md",
			"reviewed_at":  "2026-04-30T00:15:00Z",
			"session_name": "verify-coordinator",
			"title":        "Coordinator review",
			"status":       "ready",
			"summary":      "Return the task to ready for one follow-up pass.",
			"next_action":  "Resume the task in a bound subagent session.",
		}
		return c
	}

	// withBlock attaches a fresh recurrence block to a ready card.
	withBlock := func(t *testing.T) map[string]interface{} {
		c := ready(t)
		c["recurrence"] = mustBlock(t)
		return c
	}

	return []fixture{
		// validate_ok (schema passes + ack-pair holds) ----------------------
		{"ready_task", ready, verdictOK},
		{"draft_task", draft, verdictOK},
		{"working_task", working, verdictOK},
		{"reviewed_task", reviewed, verdictOK},
		{"recurrence_valid", withBlock, verdictOK},
		{"legacy_no_recurrence", ready, verdictOK}, // backward-compat: no block

		// validate_fail (schema rejects) -------------------------------------
		{"invalid_draft", func(t *testing.T) map[string]interface{} {
			c := draft(t) // draft with rough_scope set
			c["rough_scope"] = []interface{}{}
			c["open_questions"] = []interface{}{}
			c["ready_criteria"] = []interface{}{}
			return c
		}, verdictFailJSON},
		{"invalid_research_missing_question", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["research_question"] = ""
			return c
		}, verdictFailJSON},
		{"invalid_research_missing_policy", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["source_policy"] = nil
			return c
		}, verdictFailJSON},
		{"invalid_research_missing_artifact_type", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["desired_artifact_type"] = nil
			return c
		}, verdictFailJSON},
		{"invalid_research_missing_artifact_path", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["target_artifact_path"] = nil
			return c
		}, verdictFailJSON},
		{"invalid_ready", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["files_in_scope"] = []interface{}{}
			return c
		}, verdictFailJSON},
		{"invalid_working", func(t *testing.T) map[string]interface{} {
			c := working(t)
			c["active_session_alias"] = nil
			c["claimed_at"] = nil
			return c
		}, verdictFailJSON},
		{"invalid_reviewed", func(t *testing.T) map[string]interface{} {
			c := reviewed(t)
			delete(asObj(t, c["last_review"]), "path")
			return c
		}, verdictFailJSON},
		{"recurrence_empty_id", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			asObj(t, c["recurrence"])["recurrence_id"] = ""
			return c
		}, verdictFailJSON},
		{"recurrence_bad_symptom_class", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			asObj(t, c["recurrence"])["symptom_class_id"] = "bare-class-name"
			return c
		}, verdictFailJSON},
		{"recurrence_negative_count", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			asObj(t, c["recurrence"])["recurrence_count"] = -1
			return c
		}, verdictFailJSON},
		{"recurrence_missing_ack", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			r := asObj(t, c["recurrence"])
			delete(r, "recurrence_count")
			delete(r, "last_acknowledged_count")
			return c
		}, verdictFailJSON},
		{"recurrence_missing_one_ack", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			delete(asObj(t, c["recurrence"]), "last_acknowledged_count")
			return c
		}, verdictFailJSON},

		// validate_fail_ack (schema passes, cross-field ack guard rejects) ---
		{"recurrence_count_lt_ack", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			r := asObj(t, c["recurrence"])
			r["recurrence_count"] = 1
			r["last_acknowledged_count"] = 2
			return c
		}, verdictFailAck},
	}
}

// extraFixtures cover additionalProperties + nested evidence/aliases shape
// violations that the Python harness implied (additionalProperties:false at
// three levels; evidence items require kind+ref; aliases require recurrence_id)
// but never asserted explicitly. The generic validator handles them; these rows
// make the gate prove it.
func extraFixtures() []fixture {
	ready := func(t *testing.T) map[string]interface{} { return mustCard(t, baseReadyJSON) }
	withBlock := func(t *testing.T) map[string]interface{} {
		c := ready(t)
		c["recurrence"] = mustBlock(t)
		return c
	}
	return []fixture{
		{"additional_property_top_level", func(t *testing.T) map[string]interface{} {
			c := ready(t)
			c["bogus_top_level_field"] = "not allowed"
			return c
		}, verdictFailJSON},
		{"additional_property_recurrence", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			asObj(t, c["recurrence"])["bogus_rec_field"] = 7
			return c
		}, verdictFailJSON},
		{"additional_property_evidence", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			ev := asObj(t, c["recurrence"])["evidence"].([]interface{})[0]
			asObj(t, ev)["bogus_ev_field"] = true
			return c
		}, verdictFailJSON},
		{"evidence_missing_ref", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			ev := asObj(t, c["recurrence"])["evidence"].([]interface{})[0]
			delete(asObj(t, ev), "ref")
			return c
		}, verdictFailJSON},
		{"evidence_empty_kind", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			ev := asObj(t, c["recurrence"])["evidence"].([]interface{})[0]
			asObj(t, ev)["kind"] = ""
			return c
		}, verdictFailJSON},
		{"alias_missing_id", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			al := asObj(t, c["recurrence"])["aliases"].([]interface{})[0]
			delete(asObj(t, al), "recurrence_id")
			return c
		}, verdictFailJSON},
		{"alias_empty_id", func(t *testing.T) map[string]interface{} {
			c := withBlock(t)
			al := asObj(t, c["recurrence"])["aliases"].([]interface{})[0]
			asObj(t, al)["recurrence_id"] = ""
			return c
		}, verdictFailJSON},
	}
}

func TestValidateCard_Fixtures(t *testing.T) {
	schemaBytes, err := SchemaBytes()
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	schema, err := parseSchema(schemaBytes)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	var rows []fixture
	rows = append(rows, portedFixtures()...)
	rows = append(rows, extraFixtures()...)

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			card := tc.build(t)
			schemaErrs := Validate(schema, card)
			ackErr := ""
			if len(schemaErrs) == 0 {
				if e := AckPairError(card); e != nil {
					ackErr = e.Error()
				}
			}

			switch tc.verdict {
			case verdictOK:
				if len(schemaErrs) != 0 {
					t.Fatalf("expected VALID, schema rejected with %d errors:\n%s",
						len(schemaErrs), formatErrs(schemaErrs))
				}
				if ackErr != "" {
					t.Fatalf("expected VALID, ack-pair guard rejected: %s", ackErr)
				}
			case verdictFailJSON:
				if len(schemaErrs) == 0 {
					// The ack guard must NOT be the rejection when schema is the
					// intended gate: a schema-pass here for a verdictFailJSON row
					// is a real miss.
					t.Fatalf("expected SCHEMA rejection, schema passed (ackErr=%q)", ackErr)
				}
			case verdictFailAck:
				if len(schemaErrs) != 0 {
					t.Fatalf("expected schema to PASS (ack-pair is the intended rejection), schema rejected:\n%s",
						formatErrs(schemaErrs))
				}
				if ackErr == "" {
					t.Fatalf("expected ack-pair guard to REJECT, but it passed")
				}
			}
		})
	}
}

// TestValidateCard_ByteRoundTrip exercises the byte-level convenience entry
// (used by the CLI): a valid card decodes, validates, and reports Valid=true; a
// deliberately ack-inverted card reports the ack violation.
func TestValidateCard_ByteRoundTrip(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		res, err := ValidateCard([]byte(baseReadyJSON))
		if err != nil {
			t.Fatalf("ValidateCard: %v", err)
		}
		if !res.Valid {
			t.Fatalf("expected valid, got schema errors:\n%s", formatErrs(res.SchemaErrors))
		}
	})
	t.Run("ack_inverted", func(t *testing.T) {
		card := mustCard(t, baseReadyJSON)
		card["recurrence"] = mustBlock(t)
		r := asObj(t, card["recurrence"])
		r["recurrence_count"] = 1
		r["last_acknowledged_count"] = 2
		raw, _ := json.Marshal(card)
		res, err := ValidateCard(raw)
		if err != nil {
			t.Fatalf("ValidateCard: %v", err)
		}
		if res.Valid {
			t.Fatalf("expected ack-pair rejection, got valid")
		}
		if res.AckPairError == "" {
			t.Fatalf("expected AckPairError set, got empty")
		}
		if !strings.Contains(res.AckPairError, "recurrence_count") {
			t.Fatalf("ack error should name recurrence_count, got %q", res.AckPairError)
		}
	})
	t.Run("not_json", func(t *testing.T) {
		if _, err := ValidateCard([]byte("{not json")); err == nil {
			t.Fatalf("expected decode error for invalid JSON")
		}
	})
}

// TestValidateCard_IntegerPrecision pins the defer-018 review BLOCK (M1): the
// `integer` type check must reject a fractional JSON literal even when its
// float64 coercion lands on an integral value at magnitude >= 2^53 (where a
// naive float64+Trunc check would silently accept it). ValidateCard decodes
// with json.Decoder.UseNumber so the check is lexical. The control proves the
// fix is in the safe direction: a large INTEGER literal (no fractional point)
// is still accepted.
func TestValidateCard_IntegerPrecision(t *testing.T) {
	const tmpl = `{
  "schema_version": %s,
  "task_id": "t1", "title": "T", "task_type": "implementation", "coordination_mode": "short",
  "primary_lane": "repo", "report_envelope": "standard", "status": "draft",
  "session_aliases": [], "active_session_alias": null, "claimed_at": null,
  "report_paths": [], "review_paths": [], "rough_scope": ["x"],
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z", "updated_at": "2026-04-30T00:00:00Z"
}`
	// draft-07 parity: "integer" = a number with zero fractional part. These
	// forms MUST validate — the retired Python jsonschema used float.is_integer,
	// which accepts them; an earlier lexical check that rejected '.', 'e', 'E'
	// was an over-strict narrowing (it wrongly rejected "1.0"/"1e2"). The check
	// now routes through big.Rat.IsInt (denominator == 1), restoring parity.
	integerForms := []string{
		"1",
		"1.0",                // decimal-zero fractional part: integer under draft-07
		"1e2",                // exponent form with zero fractional part: integer under draft-07
		"9007199254740993",   // 2^53+1: large but genuine integer literal
		"9007199254740993.0", // .0 suffix, still zero fractional part
	}
	for _, n := range integerForms {
		n := n
		t.Run("accept_"+n, func(t *testing.T) {
			res, err := ValidateCard([]byte(fmt.Sprintf(tmpl, n)))
			if err != nil {
				t.Fatalf("ValidateCard: %v", err)
			}
			if !res.Valid {
				t.Fatalf("expected integer form %q accepted, got schema errors:\n%s", n, formatErrs(res.SchemaErrors))
			}
		})
	}
	// Fractional forms are NOT integers (reduced denominator != 1). The large
	// case 9007199254740993.5 = 2^53+1.5 is the float64-precision crux: its
	// float64 coercion rounds to an integral value, so a float64+Trunc check
	// would WRONGLY accept it. The exact big.Rat comparison rejects it.
	fractionalForms := []string{"1.5", "2.5", "9007199254740993.5"}
	for _, n := range fractionalForms {
		n := n
		t.Run("reject_"+n, func(t *testing.T) {
			res, err := ValidateCard([]byte(fmt.Sprintf(tmpl, n)))
			if err != nil {
				t.Fatalf("ValidateCard: %v", err)
			}
			if res.Valid {
				t.Fatalf("expected schema rejection of fractional integer %q, got valid", n)
			}
			if len(res.SchemaErrors) == 0 {
				t.Fatalf("expected a schema error for fractional integer %q, got none (ackErr=%q)", n, res.AckPairError)
			}
		})
	}
}

// TestValidateCard_AckPairPrecision pins the round-2 review BLOCK (F1): the
// cross-field ack guard must compare counts with EXACT integer arithmetic. A
// float64 comparison would round both 2^53 and 2^53+1 to 2^53 and slip the
// impossible-state invariant (count < ack). Both counts are lexical integers so
// the schema passes; only the ack guard catches it.
func TestValidateCard_AckPairPrecision(t *testing.T) {
	const card = `{
  "task_id": "t1", "title": "T", "task_type": "implementation", "coordination_mode": "short",
  "primary_lane": "repo", "report_envelope": "standard", "status": "draft",
  "session_aliases": [], "active_session_alias": null, "claimed_at": null,
  "report_paths": [], "review_paths": [], "rough_scope": ["x"],
  "recurrence": {
    "recurrence_id": "exact-defect-canonical-example",
    "symptom_class_id": "recurrence.v1/example-symptom-class",
    "recurrence_count": 9007199254740992,
    "last_acknowledged_count": 9007199254740993
  },
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z", "updated_at": "2026-04-30T00:00:00Z"
}`
	res, err := ValidateCard([]byte(card))
	if err != nil {
		t.Fatalf("ValidateCard: %v", err)
	}
	if len(res.SchemaErrors) != 0 {
		t.Fatalf("schema should PASS (both counts are lexical integers), got:\n%s", formatErrs(res.SchemaErrors))
	}
	if res.Valid {
		t.Fatalf("expected ack-pair rejection (count 2^53 < ack 2^53+1 in exact arithmetic), got valid")
	}
	if res.AckPairError == "" {
		t.Fatalf("expected AckPairError set")
	}
	// Both counts must render in their exact authored form (not float64-rounded).
	if !strings.Contains(res.AckPairError, "9007199254740992") || !strings.Contains(res.AckPairError, "9007199254740993") {
		t.Fatalf("ack error should name both exact counts, got %q", res.AckPairError)
	}
}

// TestValidateCard_AckPairPrecision_DecimalExponent pins the cmpInteger
// float64-fallback gap that widening isIntegerValue introduced. isIntegerValue
// accepts draft-07 integer forms that bigInt (base-10 SetString) CANNOT parse —
// decimal-zero ("...0") and exponent ("e0"). The OLD cmpInteger used a
// bigInt-then-float64 ladder: for these forms bigInt returned ok=false and it
// fell back to float64, where adjacent values at magnitude >= 2^53 collapse
// (2^53+1 -> 2^53), so cmpInteger returned 0 (equal) and the ack-pair invariant
// slipped. cmpInteger now routes through cmpNumber (big.Rat.Cmp), which is exact
// across every form isIntegerValue accepts. Both counts pass the schema (they
// are integers via big.Rat.IsInt); only the ack guard catches the violation.
func TestValidateCard_AckPairPrecision_DecimalExponent(t *testing.T) {
	const tmpl = `{
  "task_id": "t1", "title": "T", "task_type": "implementation", "coordination_mode": "short",
  "primary_lane": "repo", "report_envelope": "standard", "status": "draft",
  "session_aliases": [], "active_session_alias": null, "claimed_at": null,
  "report_paths": [], "review_paths": [], "rough_scope": ["x"],
  "recurrence": {
    "recurrence_id": "exact-defect-canonical-example",
    "symptom_class_id": "recurrence.v1/example-symptom-class",
    "recurrence_count": %s,
    "last_acknowledged_count": %s
  },
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z", "updated_at": "2026-04-30T00:00:00Z"
}`
	cases := []struct{ name, count, ack string }{
		{"decimal_zero_count", "9007199254740992.0", "9007199254740993"},
		{"decimal_zero_ack", "9007199254740992", "9007199254740993.0"},
		{"decimal_zero_both", "9007199254740992.0", "9007199254740993.0"},
		{"exponent_ack", "9007199254740992", "9007199254740993e0"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			res, err := ValidateCard([]byte(fmt.Sprintf(tmpl, c.count, c.ack)))
			if err != nil {
				t.Fatalf("ValidateCard: %v", err)
			}
			if len(res.SchemaErrors) != 0 {
				t.Fatalf("schema should PASS (both counts are draft-07 integers via big.Rat.IsInt), got:\n%s", formatErrs(res.SchemaErrors))
			}
			if res.Valid {
				t.Fatalf("expected ack-pair rejection (count %s < ack %s in exact arithmetic), got valid", c.count, c.ack)
			}
			if res.AckPairError == "" {
				t.Fatalf("expected AckPairError set for count %s < ack %s", c.count, c.ack)
			}
		})
	}
}

// TestValidateCard_TrailingData pins the round-2 review BLOCK (F2): a JSON
// document is ONE value (RFC 8259). ValidateCard must reject a valid card
// followed by a second value or trailing non-whitespace, rather than silently
// accepting the first and discarding the rest.
func TestValidateCard_TrailingData(t *testing.T) {
	cases := map[string]string{
		"second_object":  validCardMinimalJSON + "\n{}",
		"garbage_suffix": validCardMinimalJSON + " trailing",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateCard([]byte(input)); err == nil {
				t.Fatalf("expected trailing-data rejection, got nil error")
			}
		})
	}
	// Control: the clean card still validates (no false rejection from the EOF check).
	res, err := ValidateCard([]byte(validCardMinimalJSON))
	if err != nil {
		t.Fatalf("clean card should validate, got %v", err)
	}
	if !res.Valid {
		t.Fatalf("clean card should be valid, got:\n%s", formatErrs(res.SchemaErrors))
	}
}

func formatErrs(errs []VError) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString("  - ")
		b.WriteString(e.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// TestValidateCard_DateTimeCaseParity pins RFC 3339 / retired-Python parity for
// the `format: date-time` check. RFC 3339 (section 5.6 note) permits a
// lower-case separator and zone designator ([Tt]/[Zz]); the retired Python
// gate's FormatChecker (rfc3339-validator) accepted them. Go's time.Parse treats
// T/Z as case-sensitive literals and would reject them — validRFC3339DateTime
// normalizes those two positions so the port does not narrow the contract.
func TestValidateCard_DateTimeCaseParity(t *testing.T) {
	// validRFC3339DateTime table: upper-case already accepted; lower-case t/z
	// (with/without fractional seconds, with offset) MUST now also pass; garbage
	// MUST still fail (no over-loosening).
	accept := []string{
		"2026-04-30T12:00:00Z",
		"2026-04-30t12:00:00z",
		"2026-04-30T12:00:00.123Z",
		"2026-04-30t12:00:00.123z",
		"2026-04-30T12:00:00+07:00",
		"2026-04-30t12:00:00+07:00",
		"2026-04-30t12:00:00-05:00",
	}
	for _, s := range accept {
		if !validRFC3339DateTime(s) {
			t.Errorf("validRFC3339DateTime(%q) = false, want true (RFC3339 lower-case parity)", s)
		}
	}
	reject := []string{
		"2026-04-30",           // date only, no time
		"2026-04-30 12:00:00Z", // space separator (not RFC3339 date-time form)
		"2026-13-40T12:00:00Z", // out-of-range month/day
		"not-a-date-time",
		"",
	}
	for _, s := range reject {
		if validRFC3339DateTime(s) {
			t.Errorf("validRFC3339DateTime(%q) = true, want false (still rejected)", s)
		}
	}

	// Wire-through: a real card carrying a lower-case date-time validates (the
	// regression — Go's raw time.RFC3339 parse would have rejected this card).
	lower := strings.Replace(validCardMinimalJSON,
		"\"created_at\": \"2026-04-30T00:00:00Z\"",
		"\"created_at\": \"2026-04-30t00:00:00z\"", 1)
	res, err := ValidateCard([]byte(lower))
	if err != nil {
		t.Fatalf("lower-case date-time card should parse, got %v", err)
	}
	if !res.Valid {
		t.Fatalf("lower-case date-time card should be valid, got:\n%s", formatErrs(res.SchemaErrors))
	}

	// Wire-through: a genuinely malformed date-time is still rejected.
	bad := strings.Replace(validCardMinimalJSON,
		"\"created_at\": \"2026-04-30T00:00:00Z\"",
		"\"created_at\": \"2026-04-30 00:00:00\"", 1)
	res2, err := ValidateCard([]byte(bad))
	if err != nil {
		t.Fatalf("bad date-time card should parse, got %v", err)
	}
	if res2.Valid {
		t.Fatalf("malformed date-time card should be rejected, but was reported valid")
	}
}

// TestValidateCard_MinimumOverflow pins the exact-arith minimum check: a JSON
// integer literal whose magnitude overflows float64 must still be constrained by
// `minimum`. Previously asNumber returned ok=false on float64 range error and
// the `if n, ok := asNumber(inst); ok` guard SKIPPED the minimum compare, so a
// huge-magnitude negative schema_version (minimum: 1) was reported valid. The
// check now routes through cmpNumber (math/big.Rat) so the overflow case is
// compared exactly, matching the ack-pair guard's big.Int exactness.
func TestValidateCard_MinimumOverflow(t *testing.T) {
	cardWithVersion := func(rawNumber string) string {
		return strings.Replace(validCardMinimalJSON,
			"\"task_id\": \"t1\",",
			"\"task_id\": \"t1\", \"schema_version\": "+rawNumber+",", 1)
	}
	// Regression: a huge-magnitude negative integer overflows float64 but must
	// still violate minimum: 1.
	res, err := ValidateCard([]byte(cardWithVersion("-1000000000000000000000000000000")))
	if err != nil {
		t.Fatalf("huge-negative card should parse, got %v", err)
	}
	if res.Valid {
		t.Fatalf("huge-negative schema_version (-1e30) should be rejected by minimum:1, got valid")
	}

	// Control: a huge-magnitude positive integer overflows float64 and is >= 1,
	// so it must be accepted (no false rejection from the overflow path).
	res2, err := ValidateCard([]byte(cardWithVersion("1000000000000000000000000000000")))
	if err != nil {
		t.Fatalf("huge-positive card should parse, got %v", err)
	}
	if !res2.Valid {
		t.Fatalf("huge-positive schema_version (1e30) should be valid (>= minimum:1), got:\n%s", formatErrs(res2.SchemaErrors))
	}

	// Baseline: a normal small negative integer is rejected (guards against an
	// accidental inversion of the comparison sign).
	res3, err := ValidateCard([]byte(cardWithVersion("-5")))
	if err != nil {
		t.Fatalf("-5 card should parse, got %v", err)
	}
	if res3.Valid {
		t.Fatalf("schema_version -5 should be rejected by minimum:1, got valid")
	}
}
