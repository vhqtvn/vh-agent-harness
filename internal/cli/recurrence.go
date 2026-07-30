package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/recurrence"
)

// recurrence.go — the CLI bridge for the WRITE-LAYER dedup (P1-MEMORY-001
// Slice 3). The task-card producer is JS
// (templates/core/.opencode/scripts/state-lib.js:saveCoordinationTask); it owns
// the filesystem scan of existing cards but does NOT duplicate the derivation's
// effective-identity + alias-reconciliation logic (duplication hazard). Instead
// it shells out to `vh-agent-harness recurrence dedup`, passing a JSON request
// on stdin (incoming card + the existing recurrence-bearing population) and
// reading a JSON decision on stdout.
//
// This command is a STATELESS bridge: it maps the wire JSON to/from the pure
// recurrence.Card / recurrence.Decision types, calls
// recurrence.ResolveRecurrence, and emits the decision. No filesystem access,
// no store, no side effects — all state lives in the JS producer. The producer
// APPLIES the decision; this command (and the derivation) INFORM only.
//
// Authority line (memo efa53fb): the producer provides synchronous merge
// CONVENIENCE and APPLIES the decision at the write boundary; neither the
// derivation nor this bridge is transition authority. The release gate (Slice 5)
// ACTs / fails closed on an unadjudicated recurrence.

// recurrenceCmd is the parent for recurrence-related diagnostics. Currently
// exposes one subcommand (dedup); future slices may add `doctor`-style linting
// or derivation previews here.
var recurrenceCmd = &cobra.Command{
	Use:          "recurrence",
	Short:        "Recurrence-signature diagnostics (producer dedup bridge)",
	SilenceUsage: true,
	Long: `Recurrence-signature diagnostics for the coordination task-card producer.

Subcommands provide stateless bridges between the JS task-card producer and the
pure Go recurrence derivation. The producer (saveCoordinationTask in
state-lib.js) consults these at the task-writing boundary to decide whether an
incoming card is a repeat of a known canonical (merge) or a new defect (new
card).

These commands INFORM the producer only; they are NOT transition authority. The
release gate (Slice 5) enforces acknowledgement of unadjudicated recurrences.`,
	Args: cobra.NoArgs,
}

// recurrenceDedupCmd reads a JSON dedup request from stdin and prints the dedup
// decision as JSON on stdout. Request shape:
//
//	{
//	  "incoming": { "task_id": "...", "recurrence": { ... } },
//	  "existing": [ { "task_id": "...", "recurrence": { ... } }, ... ]
//	}
//
// Response shape:
//
//	{
//	  "action": "new_card" | "merge",
//	  "effective_id": "...",
//	  "canonical_task_id": "...",          // non-empty on merge
//	  "merged": { ... },                   // present on merge only
//	  "incoming_observation": { ... }
//	}
var recurrenceDedupCmd = &cobra.Command{
	Use:          "dedup",
	Short:        "Decide whether an incoming task card is a repeat (merge) or new (new_card)",
	SilenceUsage: true,
	Long: `Read a JSON dedup request from stdin and print the recurrence dedup decision
as JSON on stdout.

The request carries the incoming card (the card being written) and the existing
recurrence-bearing population (scanned by the JS producer). The decision is
computed by the pure recurrence.ResolveRecurrence, which resolves effective
identity, reconciles aliases, and — on a match — produces the merged canonical
block (count bumped, observation appended, last_acknowledged_count held so the
disposition becomes unacknowledged).

On action=merge the producer must update the canonical card (canonical_task_id)
with the merged block instead of spawning a new card. On action=new_card the
producer writes the incoming card normally.`,
	Args: cobra.NoArgs,
	RunE: runRecurrenceDedup,
}

// dedupRequest is the JSON wire shape read from stdin. It maps directly to the
// pure recurrence types; the existing[] slice carries only the recurrence-bearing
// population (the JS adapter filters out legacy cards before sending, but
// ResolveRecurrence also ignores legacy existing cards defensively).
type dedupRequest struct {
	Incoming recurrence.Card   `json:"incoming"`
	Existing []recurrence.Card `json:"existing,omitempty"`
}

// dedupResponse is the JSON wire shape printed on stdout.
type dedupResponse struct {
	Action              string                 `json:"action"` // "new_card" | "merge"
	EffectiveID         string                 `json:"effective_id"`
	CanonicalTaskID     string                 `json:"canonical_task_id"`
	Merged              *recurrence.Block      `json:"merged,omitempty"`
	IncomingObservation recurrence.Observation `json:"incoming_observation"`
}

func init() {
	recurrenceCmd.AddCommand(recurrenceDedupCmd)
	rootCmd.AddCommand(recurrenceCmd)
	assignGroup(groupHealth, recurrenceCmd)
}

// runRecurrenceDedup is the in-process entry point for the dedup subcommand. It
// reads the JSON request from the command's stdin, runs the pure
// recurrence.ResolveRecurrence, and prints the decision as JSON on stdout.
// Exported via closure so tests exercise it directly without spawning a process.
func runRecurrenceDedup(cmd *cobra.Command, _ []string) error {
	in := cmd.InOrStdin()
	out := cmd.OutOrStdout()

	body, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var req dedupRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse dedup request: %w", err)
	}

	dec := recurrence.ResolveRecurrence(req.Existing, req.Incoming)

	resp := dedupResponse{
		Action:              dec.Action.String(),
		EffectiveID:         dec.EffectiveID,
		CanonicalTaskID:     dec.CanonicalTaskID,
		Merged:              dec.Merged,
		IncomingObservation: dec.IncomingObservation,
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
