package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/recurrence"
)

// TestRecurrenceDedup_Merge is the WRITE-LAYER crux proof for the Go bridge:
// an incoming card whose recurrence_id matches an existing canonical must
// yield action=merge, canonical_task_id pointing at the existing card, and a
// Merged block with recurrence_count N→N+1, last_acknowledged_count held
// (→ unacknowledged), and a recurrence_observation evidence entry referencing
// the incoming task_id.
func TestRecurrenceDedup_Merge(t *testing.T) {
	req := dedupRequest{
		Incoming: recurrence.Card{
			TaskID: "T-repeat",
			Recurrence: &recurrence.Block{
				RecurrenceID:          "R1",
				SymptomClassID:        "CLASS-A",
				RecurrenceCount:       1,
				LastAcknowledgedCount: 1,
				Evidence: []recurrence.Evidence{
					{Kind: "path", Ref: "src/repeat.go"},
				},
			},
		},
		Existing: []recurrence.Card{
			{
				TaskID: "T-canonical",
				Recurrence: &recurrence.Block{
					RecurrenceID:          "R1",
					SymptomClassID:        "CLASS-A",
					RecurrenceCount:       1,
					LastAcknowledgedCount: 1,
					Evidence: []recurrence.Evidence{
						{Kind: "path", Ref: "src/canonical.go"},
					},
				},
			},
		},
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	cmd, out := newOutCmd()
	cmd.SetIn(bytes.NewReader(reqBytes))

	if err := runRecurrenceDedup(cmd, nil); err != nil {
		t.Fatalf("runRecurrenceDedup: %v", err)
	}

	var resp dedupResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}

	if resp.Action != "merge" {
		t.Fatalf("action: want merge, got %q", resp.Action)
	}
	if resp.CanonicalTaskID != "T-canonical" {
		t.Fatalf("canonical_task_id: want T-canonical, got %q", resp.CanonicalTaskID)
	}
	if resp.Merged == nil {
		t.Fatal("merged block is nil on merge")
	}
	if resp.Merged.RecurrenceCount != 2 {
		t.Fatalf("recurrence_count: want 2 (N→N+1), got %d", resp.Merged.RecurrenceCount)
	}
	if resp.Merged.LastAcknowledgedCount != 1 {
		t.Fatalf("last_acknowledged_count: want 1 (held → unacknowledged), got %d",
			resp.Merged.LastAcknowledgedCount)
	}
	// The recurrence observation must be attributable to the incoming task_id.
	foundObs := false
	for _, ev := range resp.Merged.Evidence {
		if ev.Kind == recurrence.EvidenceKindRecurrenceObservation && ev.Ref == "T-repeat" {
			foundObs = true
		}
	}
	if !foundObs {
		t.Fatalf("merged evidence missing recurrence_observation ref=T-repeat; got %+v",
			resp.Merged.Evidence)
	}
	// Canonical recurrence_id is retained; the repeat's identity folds in.
	if resp.Merged.RecurrenceID != "R1" {
		t.Fatalf("merged recurrence_id: want R1 (canonical retained), got %q",
			resp.Merged.RecurrenceID)
	}
}

// TestRecurrenceDedup_NewCard: an incoming card with a recurrence_id that does
// NOT match any existing canonical → action=new_card, no merged block, no
// canonical_task_id. The producer writes a fresh card normally.
func TestRecurrenceDedup_NewCard(t *testing.T) {
	req := dedupRequest{
		Incoming: recurrence.Card{
			TaskID: "T-new",
			Recurrence: &recurrence.Block{
				RecurrenceID:   "R-fresh",
				SymptomClassID: "CLASS-B",
			},
		},
		Existing: []recurrence.Card{
			{
				TaskID: "T-canonical",
				Recurrence: &recurrence.Block{
					RecurrenceID:   "R1",
					SymptomClassID: "CLASS-A",
				},
			},
		},
	}
	reqBytes, _ := json.Marshal(req)

	cmd, out := newOutCmd()
	cmd.SetIn(bytes.NewReader(reqBytes))

	if err := runRecurrenceDedup(cmd, nil); err != nil {
		t.Fatalf("runRecurrenceDedup: %v", err)
	}

	var resp dedupResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}

	if resp.Action != "new_card" {
		t.Fatalf("action: want new_card, got %q", resp.Action)
	}
	if resp.CanonicalTaskID != "" {
		t.Fatalf("canonical_task_id: want empty on new_card, got %q", resp.CanonicalTaskID)
	}
	if resp.Merged != nil {
		t.Fatalf("merged block: want nil on new_card, got %+v", resp.Merged)
	}
	if resp.EffectiveID != "R-fresh" {
		t.Fatalf("effective_id: want R-fresh, got %q", resp.EffectiveID)
	}
}

// TestRecurrenceDedup_LegacyIncoming: an incoming card with NO recurrence block
// → always new_card (its effective id is its own unique task_id, which no
// recurrence canonical shares).
func TestRecurrenceDedup_LegacyIncoming(t *testing.T) {
	req := dedupRequest{
		Incoming: recurrence.Card{
			TaskID: "T-legacy",
		},
		Existing: []recurrence.Card{
			{
				TaskID: "T-canonical",
				Recurrence: &recurrence.Block{
					RecurrenceID: "R1",
				},
			},
		},
	}
	reqBytes, _ := json.Marshal(req)

	cmd, out := newOutCmd()
	cmd.SetIn(bytes.NewReader(reqBytes))

	if err := runRecurrenceDedup(cmd, nil); err != nil {
		t.Fatalf("runRecurrenceDedup: %v", err)
	}

	var resp dedupResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}

	if resp.Action != "new_card" {
		t.Fatalf("action: want new_card for legacy incoming, got %q", resp.Action)
	}
	if resp.EffectiveID != "T-legacy" {
		t.Fatalf("effective_id: want T-legacy (task_id fallback), got %q", resp.EffectiveID)
	}
}

// TestRecurrenceDedup_Registered verifies the subcommand is wired into the
// cobra command tree so the JS producer can invoke it via the binary.
func TestRecurrenceDedup_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "recurrence" {
			for _, sub := range cmd.Commands() {
				if sub.Use == "dedup" {
					return // found
				}
			}
			t.Fatalf("recurrence command found but has no 'dedup' subcommand; children: %v",
				subcommandUses(cmd))
		}
	}
	t.Fatalf("recurrence command not registered in rootCmd")
}

func subcommandUses(cmd *cobra.Command) []string {
	var uses []string
	for _, c := range cmd.Commands() {
		uses = append(uses, c.Use)
	}
	return uses
}

// TestRecurrenceDedup_EmptyStdin guards the degenerate input: empty stdin
// (no existing cards, no incoming) must not panic — it resolves to new_card
// with empty effective id (the producer writes a legacy card normally).
func TestRecurrenceDedup_EmptyStdin(t *testing.T) {
	cmd, out := newOutCmd()
	cmd.SetIn(strings.NewReader("{}"))

	if err := runRecurrenceDedup(cmd, nil); err != nil {
		t.Fatalf("runRecurrenceDedup on empty: %v", err)
	}

	var resp dedupResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}
	if resp.Action != "new_card" {
		t.Fatalf("action on empty: want new_card, got %q", resp.Action)
	}
}
