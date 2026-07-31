package cli

// taskcard_test.go — the CLI bridge test for the task-card validate subcommand
// (defer-018). Mirrors recurrence_dedup_test.go: a registration check (the
// command is wired into rootCmd so callers can invoke it via the binary) plus
// functional coverage of stdin/file input and the valid/rejected verdicts.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validCardJSON is a minimal task card that passes the full schema + ack guard
// (task_type=implementation avoids the research-then branch; status=draft with a
// non-empty rough_scope satisfies the draft-then anyOf; every required field is
// present with valid date-times).
const validCardJSON = `{
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
  "rough_scope": ["map the boundary"],
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z",
  "updated_at": "2026-04-30T00:00:00Z"
}`

// ackInvertedCardJSON is the valid card plus a recurrence block whose
// recurrence_count (1) < last_acknowledged_count (2): the schema passes, the
// cross-field ack guard rejects (validate_fail_ack in the Python harness).
const ackInvertedCardJSON = `{
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
  "rough_scope": ["map the boundary"],
  "recurrence": {
    "recurrence_id": "exact-defect-canonical-example",
    "symptom_class_id": "recurrence.v1/example-symptom-class",
    "recurrence_count": 1,
    "last_acknowledged_count": 2
  },
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z",
  "updated_at": "2026-04-30T00:00:00Z"
}`

// TestTaskCardValidate_Registered verifies the subcommand is wired into the
// cobra command tree so callers can invoke it via the binary.
func TestTaskCardValidate_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "task-card" {
			for _, sub := range cmd.Commands() {
				if sub.Use == "validate [<file>]" {
					return // found
				}
			}
			t.Fatalf("task-card command found but has no 'validate' subcommand; children: %v",
				subcommandUses(cmd))
		}
	}
	t.Fatalf("task-card command not registered in rootCmd")
}

func TestTaskCardValidate_StdinValid(t *testing.T) {
	cmd, out := newOutCmd()
	cmd.SetIn(strings.NewReader(validCardJSON))

	if err := runTaskCardValidate(cmd, nil); err != nil {
		t.Fatalf("runTaskCardValidate valid: unexpected err %v", err)
	}
	if !strings.Contains(out.String(), "task-card: valid") {
		t.Fatalf("expected 'task-card: valid' verdict, got %q", out.String())
	}
}

func TestTaskCardValidate_StdinAckReject(t *testing.T) {
	cmd, out := newOutCmd()
	cmd.SetIn(strings.NewReader(ackInvertedCardJSON))

	if err := runTaskCardValidate(cmd, nil); err != errTaskCardReject {
		t.Fatalf("runTaskCardValidate ack-inverted: want errTaskCardReject, got %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "ack-pair") {
		t.Fatalf("expected ack-pair defect in output, got %q", body)
	}
	if !strings.Contains(body, "recurrence_count") {
		t.Fatalf("expected recurrence_count named in defect, got %q", body)
	}
}

// schemaRejectCardJSON is a card the schema rejects (top-level required
// property "task_id" missing) while remaining valid JSON, so the CLI exercises
// the schema-reject path (defects on stdout, errTaskCardReject, exit 1) distinctly from
// the ack-pair and parse paths.
const schemaRejectCardJSON = `{
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
  "history": [{"at": "2026-04-30T00:00:00Z", "event": "created"}],
  "created_at": "2026-04-30T00:00:00Z",
  "updated_at": "2026-04-30T00:00:00Z"
}`

func TestTaskCardValidate_StdinSchemaReject(t *testing.T) {
	cmd, out := newOutCmd()
	cmd.SetIn(strings.NewReader(schemaRejectCardJSON))

	if err := runTaskCardValidate(cmd, nil); err != errTaskCardReject {
		t.Fatalf("runTaskCardValidate schema-reject: want errTaskCardReject, got %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "schema rejected") {
		t.Fatalf("expected 'schema rejected' verdict, got %q", body)
	}
	if !strings.Contains(body, "task_id") {
		t.Fatalf("expected the missing task_id named in defects, got %q", body)
	}
}

func TestTaskCardValidate_FileArg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "card.json")
	if err := os.WriteFile(path, []byte(validCardJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cmd, out := newOutCmd()
	if err := runTaskCardValidate(cmd, []string{path}); err != nil {
		t.Fatalf("runTaskCardValidate file-arg valid: unexpected err %v", err)
	}
	if !strings.Contains(out.String(), "task-card: valid") {
		t.Fatalf("expected 'task-card: valid' verdict, got %q", out.String())
	}
}

func TestTaskCardValidate_NotJSON(t *testing.T) {
	cmd, _ := newOutCmd()
	cmd.SetIn(strings.NewReader("{not json"))
	if err := runTaskCardValidate(cmd, nil); err == nil {
		t.Fatalf("runTaskCardValidate bad json: expected non-nil error")
	}
}
