package cli

import (
	"strings"
	"testing"
)

// f2_tamper_test.go — supplementary spoofed-emit-digest regression for the F2
// ingest gate. This covers the MORE SOPHISTICATED attack vector than
// TestIngestF1EmitForF2_SpoofedEmitDigestRejected: an attacker who tampers the
// canonical content AND re-binds the envelope's digest (so the envelope
// internally re-validates via ValidateF1Envelope) but leaves the emit's
// SemanticDigest as the stale, pre-tamper value. The reconciliation check
// (emit.SemanticDigest == env.SemanticDigest) catches this: the envelope's
// verified digest no longer matches the emit's claimed digest.
//
// This test was identified by the commit-reviewer during the Slice 1 blocked
// review (critical/data_integrity finding) and is retained as a complementary
// regression: it proves the fix holds even when the attacker keeps the
// envelope internally consistent.

// TestIngestF1EmitForF2_SpoofedEmitDigestAfterContentTamperRejected verifies
// that tampering the canonical content + re-binding the envelope's digest
// (so the envelope re-validates) but leaving emit.SemanticDigest stale is
// REJECTED. Without the emit-vs-envelope reconciliation check, this attack
// would pass: ValidateF1Envelope would accept the envelope (its rebound
// digest binds the tampered content), and F2 would capture the stale
// emit.SemanticDigest as the binding digest — a digest that no longer binds
// the shipped (tampered) content.
func TestIngestF1EmitForF2_SpoofedEmitDigestAfterContentTamperRejected(t *testing.T) {
	emit := f2EmitFromFixture(t)
	staleDigest := emit.SemanticDigest

	// Attack: tamper the canonical content.
	emit.CanonicalEnvelope.SynthesisCycleID = "tampered-after-emit"
	// Re-bind the envelope's digest so ValidateF1Envelope PASSES on the
	// tampered content (the envelope is now internally consistent again).
	rebound, err := emit.CanonicalEnvelope.ComputeDigest()
	if err != nil {
		t.Fatalf("recompute envelope digest (setup): %v", err)
	}
	emit.CanonicalEnvelope.SemanticDigest = rebound
	// Leave emit.SemanticDigest as the stale, pre-tamper digest. Now
	// emit.SemanticDigest != env.SemanticDigest.

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("the content-tamper + rebound-envelope-digest + stale-emit-digest attack was ingested (result non-nil) — F2 would carry a binding digest that does not bind the shipped (tampered) content")
	}
	if len(errs) == 0 {
		t.Fatalf("the spoofed-emit-digest attack produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "emit.SemanticDigest") || !strings.Contains(joined, "canonical envelope") {
		t.Fatalf("rejection did not name the emit-vs-envelope digest mismatch; got:\n  %s", joined)
	}
	// The stale digest is the original; the envelope now carries the rebound
	// digest. Confirm they actually differ (otherwise the test setup is wrong).
	if staleDigest == rebound {
		t.Fatalf("test setup invariant: stale digest == rebound digest (the tamper did not actually change the digest)")
	}
}
