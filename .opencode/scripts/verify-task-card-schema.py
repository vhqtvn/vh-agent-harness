from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path

from jsonschema import Draft7Validator, FormatChecker


REPO_ROOT = Path(__file__).resolve().parents[2]
SCHEMA_PATH = (
    REPO_ROOT / "docs" / "coordination" / "schemas" / "task-card.schema.json"
)


def load_schema() -> dict:
    return json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))


def build_base_task() -> dict:
    return {
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
        "validation_plan": ["Run verify-task-card-schema.py."],
        "report_envelope": "standard",
        "backlog_id": "P0-REPO-062",
        "workstream_slug": None,
        "dependencies": [],
        "owner_notes": [],
        "status": "ready",
        "session_aliases": ["verify-schema"],
        "active_session_alias": None,
        "claimed_at": None,
        "report_paths": [],
        "review_paths": [],
        "latest_report": None,
        "next_action": "Resume when needed.",
        "last_review": None,
        "history": [
            {
                "at": "2026-04-30T00:00:00Z",
                "event": "task_created",
                "session_name": "verify-schema",
                "status": "ready",
                "note": "Fixture created for schema validation.",
            }
        ],
        "created_at": "2026-04-30T00:00:00Z",
        "updated_at": "2026-04-30T00:00:00Z",
    }


# ---- Recurrence-signature contract ----------------------------------------
#
# The recurrence block is OPTIONAL on every task-card (backward-compat: legacy
# cards carry no block). When present, the block carries the two-level identity
# (recurrence_id = exact-defect collapse key; symptom_class_id = immutable/
# versioned taxonomy id `recurrence.v1/<class>`), the acknowledgement pair
# (recurrence_count / last_acknowledged_count), non-identity evidence[], and a
# bounded aliases[]/supersession list. JSON Schema draft-07 expresses per-field
# shape (type, minLength, pattern, minimum:0, required, additionalProperties);
# it CANNOT express the cross-field invariant recurrence_count >=
# last_acknowledged_count, so that guard lives here in the validator script
# (assert_ack_pair) and runs after schema validation passes.


def build_recurrence_block() -> dict:
    """A well-formed recurrence block (all four logical parts populated).

    Example values are deliberately generic/domain-free so the embedded corpus
    does not leak any one project's specifics into consumer repos.
    """
    return {
        "recurrence_id": "exact-defect-canonical-example",
        "symptom_class_id": "recurrence.v1/example-symptom-class",
        "recurrence_count": 2,
        "last_acknowledged_count": 1,
        "evidence": [
            {
                "kind": "path",
                "ref": "pkg/example/handler.go",
                "note": "Recurring loop observed in the example handler.",
            },
            {
                "kind": "outcome",
                "ref": "review-observation-001",
                "note": "Second observation of the same symptom class.",
            },
        ],
        "aliases": [
            {
                "recurrence_id": "exact-defect-prior-alias",
                "superseded": True,
                "note": "Re-pointed after later evidence.",
            },
        ],
    }


def assert_ack_pair(name: str, payload: dict) -> None:
    """Cross-field guard: recurrence_count >= last_acknowledged_count.

    Runs AFTER schema validation passes. The schema REQUIRES both counts
    whenever a recurrence block is present, so by the time this runs both are
    guaranteed present integers >= 0 (type+minimum enforced by draft-07). Only
    the relational >= is left, which draft-07 cannot express — hence this guard.
    A card with no recurrence block is a no-op.
    """
    rec = payload.get("recurrence") if isinstance(payload, dict) else None
    if not isinstance(rec, dict):
        return
    count = rec["recurrence_count"]
    ack = rec["last_acknowledged_count"]
    if count < ack:
        raise AssertionError(
            f"{name}: recurrence_count ({count}) must be >= "
            f"last_acknowledged_count ({ack})"
        )


def validate_ok(name: str, payload: dict, validator: Draft7Validator) -> None:
    errors = sorted(validator.iter_errors(payload), key=lambda error: list(error.path))
    if errors:
        details = "; ".join(error.message for error in errors)
        raise AssertionError(f"{name} should be valid, but failed: {details}")
    # A valid card must ALSO satisfy the cross-field ack-pair invariant.
    assert_ack_pair(name, payload)


def validate_fail(name: str, payload: dict, validator: Draft7Validator) -> None:
    errors = sorted(validator.iter_errors(payload), key=lambda error: list(error.path))
    if not errors:
        raise AssertionError(f"{name} should be invalid, but passed validation")


def validate_fail_ack(name: str, payload: dict, validator: Draft7Validator) -> None:
    """Schema PASSES but the cross-field ack-pair guard REJECTS.

    This is the impossible-state case: recurrence_count and
    last_acknowledged_count are individually well-formed (integers >= 0) so
    draft-07 accepts them, but recurrence_count < last_acknowledged_count is
    an impossible state the relational guard must reject.
    """
    errors = sorted(validator.iter_errors(payload), key=lambda error: list(error.path))
    if errors:
        details = "; ".join(error.message for error in errors)
        raise AssertionError(
            f"{name} should pass schema (ack-pair is the intended rejection), "
            f"but schema failed: {details}"
        )
    try:
        assert_ack_pair(name, payload)
    except AssertionError:
        return  # expected: the relational guard rejected the impossible state
    raise AssertionError(
        f"{name} should fail the ack-pair guard, but passed both schema and guard"
    )


def main() -> None:
    schema = load_schema()
    validator = Draft7Validator(schema, format_checker=FormatChecker())

    ready_task = build_base_task()

    draft_task = deepcopy(ready_task)
    draft_task["status"] = "draft"
    draft_task["rough_scope"] = ["Map the remaining coordinator edge cases."]
    draft_task["files_in_scope"] = []
    draft_task["success_criteria"] = []
    draft_task["validation_plan"] = []
    draft_task["active_session_alias"] = None
    draft_task["claimed_at"] = None
    draft_task["history"][0]["status"] = "draft"

    working_task = deepcopy(ready_task)
    working_task["status"] = "working"
    working_task["active_session_alias"] = "verify-subagent"
    working_task["claimed_at"] = "2026-04-30T00:05:00Z"
    working_task["session_aliases"] = ["verify-schema", "verify-subagent"]
    working_task["history"][0]["status"] = "working"

    reviewed_task = deepcopy(ready_task)
    reviewed_task["status"] = "completed"
    reviewed_task["review_paths"] = [
        ".local/coordinator/reports/task-verify-schema/2026-04-30T00-15-00Z-review.md"
    ]
    reviewed_task["last_review"] = {
        "path": ".local/coordinator/reports/task-verify-schema/2026-04-30T00-15-00Z-review.md",
        "reviewed_at": "2026-04-30T00:15:00Z",
        "session_name": "verify-coordinator",
        "title": "Coordinator review",
        "status": "ready",
        "summary": "Return the task to ready for one follow-up pass.",
        "next_action": "Resume the task in a bound subagent session.",
    }

    invalid_draft = deepcopy(draft_task)
    invalid_draft["rough_scope"] = []
    invalid_draft["open_questions"] = []
    invalid_draft["ready_criteria"] = []

    invalid_research_missing_question = deepcopy(ready_task)
    invalid_research_missing_question["research_question"] = ""

    invalid_research_missing_policy = deepcopy(ready_task)
    invalid_research_missing_policy["source_policy"] = None

    invalid_research_missing_artifact_type = deepcopy(ready_task)
    invalid_research_missing_artifact_type["desired_artifact_type"] = None

    invalid_research_missing_artifact_path = deepcopy(ready_task)
    invalid_research_missing_artifact_path["target_artifact_path"] = None

    invalid_ready = deepcopy(ready_task)
    invalid_ready["files_in_scope"] = []

    invalid_working = deepcopy(working_task)
    invalid_working["active_session_alias"] = None
    invalid_working["claimed_at"] = None

    invalid_reviewed = deepcopy(reviewed_task)
    del invalid_reviewed["last_review"]["path"]

    # ---- Recurrence-signature contract fixtures ----------------------------
    #
    # recurrence block is OPTIONAL: a card with no block must still validate
    # (backward-compat — already covered by ready_task/draft_task/working_task/
    # reviewed_task above; legacy_no_recurrence makes that assertion explicit).

    # VALID: a recurrence-bearing card (identity + ack pair + evidence + alias).
    recurrence_valid = deepcopy(ready_task)
    recurrence_valid["recurrence"] = build_recurrence_block()

    # A legacy card with NO recurrence block (backward-compat).
    legacy_no_recurrence = deepcopy(ready_task)

    # MALFORMED IDENTITY (schema-level rejection):
    # empty recurrence_id when the block is present.
    recurrence_empty_id = deepcopy(ready_task)
    recurrence_empty_id["recurrence"] = build_recurrence_block()
    recurrence_empty_id["recurrence"]["recurrence_id"] = ""

    # symptom_class_id not matching recurrence.v1/<class>.
    recurrence_bad_symptom_class = deepcopy(ready_task)
    recurrence_bad_symptom_class["recurrence"] = build_recurrence_block()
    recurrence_bad_symptom_class["recurrence"]["symptom_class_id"] = "bare-class-name"

    # negative recurrence_count.
    recurrence_negative_count = deepcopy(ready_task)
    recurrence_negative_count["recurrence"] = build_recurrence_block()
    recurrence_negative_count["recurrence"]["recurrence_count"] = -1

    # IMPOSSIBLE STATE (cross-field guard rejection, schema passes):
    # recurrence_count < last_acknowledged_count.
    recurrence_count_lt_ack = deepcopy(ready_task)
    recurrence_count_lt_ack["recurrence"] = build_recurrence_block()
    recurrence_count_lt_ack["recurrence"]["recurrence_count"] = 1
    recurrence_count_lt_ack["recurrence"]["last_acknowledged_count"] = 2

    # MISSING ACKNOWLEDGEMENT PAIR (schema-level rejection): the block carries
    # identity but NO acknowledgement state at all — the contract requires the
    # pair whenever a block is present.
    recurrence_missing_ack = deepcopy(ready_task)
    recurrence_missing_ack["recurrence"] = build_recurrence_block()
    del recurrence_missing_ack["recurrence"]["recurrence_count"]
    del recurrence_missing_ack["recurrence"]["last_acknowledged_count"]

    # MISSING ONE HALF of the ack pair (schema-level rejection): the counts are
    # a pair — one present without the other is also malformed.
    recurrence_missing_one_ack = deepcopy(ready_task)
    recurrence_missing_one_ack["recurrence"] = build_recurrence_block()
    del recurrence_missing_one_ack["recurrence"]["last_acknowledged_count"]

    validate_ok("draft_task", draft_task, validator)
    validate_ok("ready_task", ready_task, validator)
    validate_ok("working_task", working_task, validator)
    validate_ok("reviewed_task", reviewed_task, validator)
    validate_fail("invalid_draft", invalid_draft, validator)
    validate_fail(
        "invalid_research_missing_question",
        invalid_research_missing_question,
        validator,
    )
    validate_fail(
        "invalid_research_missing_policy",
        invalid_research_missing_policy,
        validator,
    )
    validate_fail(
        "invalid_research_missing_artifact_type",
        invalid_research_missing_artifact_type,
        validator,
    )
    validate_fail(
        "invalid_research_missing_artifact_path",
        invalid_research_missing_artifact_path,
        validator,
    )
    validate_fail("invalid_ready", invalid_ready, validator)
    validate_fail("invalid_working", invalid_working, validator)
    validate_fail("invalid_reviewed", invalid_reviewed, validator)

    # Recurrence-signature contract assertions.
    validate_ok("recurrence_valid", recurrence_valid, validator)
    validate_ok("legacy_no_recurrence", legacy_no_recurrence, validator)
    validate_fail("recurrence_empty_id", recurrence_empty_id, validator)
    validate_fail(
        "recurrence_bad_symptom_class",
        recurrence_bad_symptom_class,
        validator,
    )
    validate_fail(
        "recurrence_negative_count",
        recurrence_negative_count,
        validator,
    )
    validate_fail_ack(
        "recurrence_count_lt_ack",
        recurrence_count_lt_ack,
        validator,
    )
    validate_fail("recurrence_missing_ack", recurrence_missing_ack, validator)
    validate_fail(
        "recurrence_missing_one_ack",
        recurrence_missing_one_ack,
        validator,
    )

    print("schema_verification: ok")
    print(
        "validated_examples: draft ready working reviewed invalid_draft invalid_research_missing_question invalid_research_missing_policy invalid_research_missing_artifact_type invalid_research_missing_artifact_path invalid_ready invalid_working invalid_reviewed recurrence_valid legacy_no_recurrence recurrence_empty_id recurrence_bad_symptom_class recurrence_negative_count recurrence_count_lt_ack recurrence_missing_ack recurrence_missing_one_ack"
    )


if __name__ == "__main__":
    main()
