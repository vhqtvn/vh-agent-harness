package cli

// Integration tests for the doctor-command freshness diagnostic
// (internal/cli/doctor.go + internal/cli/corpus_freshness.go). These exercise
// the WIRED runDoctor path against fixtures built from the test binary's REAL
// embedded corpus. They cover: dev-stale-embed WARN (no exit-code change) +
// qualified managed-drift when the corpus differs; dev-stale-embed PASS when
// fresh; consumer (no corpus.go + templates/core) sees NO new section; and the
// WARN-alone-keeps-HEALTHY property.

import (
	"strings"
	"testing"
)

// --- differs: WARN + qualified managed-drift + HEALTHY ---------------------

// TestDoctorFreshness_DevStaleWarnAndQualifiedManagedDrift: an installed source
// checkout whose templates/core differs from the embedded corpus must surface
// a dev-stale-embed WARN, qualify the managed-drift detail (no bare "in sync"),
// and stay HEALTHY (the WARN must NOT fail the command by itself — real drift
// is owned by managed-drift, which here is PASS because the live tree matches
// the embedded corpus).
func TestDoctorFreshness_DevStaleWarnAndQualifiedManagedDrift(t *testing.T) {
	abs := t.TempDir()
	// Seam-install FIRST so the live tree matches the embedded corpus
	// (managed-drift will be PASS), THEN add the source-checkout markers +
	// mutate one templates/core file (dev-stale, managed-drift still PASS).
	seamInstallInto(t, abs)
	writeCorpusGo(t, abs)
	copyEmbeddedCorpusToDisk(t, abs)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	out := seamDoctorOut(t, abs)

	// dev-stale-embed section present and WARN, naming `make update`.
	if !strings.Contains(out, "dev-stale-embed") {
		t.Errorf("want a dev-stale-embed section for a differs source checkout; got:\n%s", out)
	}
	if !strings.Contains(out, "dev-stale-embed WARN") {
		t.Errorf("dev-stale-embed must be WARN on differs; got:\n%s", out)
	}
	if !strings.Contains(out, "make update") {
		t.Errorf("dev-stale-embed WARN should point at `make update`; got:\n%s", out)
	}

	// managed-drift is PASS but its detail is qualified: no bare "in sync".
	if !strings.Contains(out, "in sync with the embedded corpus in this binary") {
		t.Errorf("managed-drift must be qualified (no bare 'in sync') when dev-stale; got:\n%s", out)
	}

	// The dev-stale WARN alone must NOT fail the command.
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("dev-stale WARN alone must keep doctor HEALTHY (it must not fail by itself); got:\n%s", out)
	}
}

// --- fresh source checkout: dev-stale-embed PASS ---------------------------

// TestDoctorFreshness_FreshSourceCheckoutPasses: a source checkout whose
// templates/core matches the embedded corpus shows dev-stale-embed PASS (the
// section is present for any source checkout, but PASS when current) and stays
// HEALTHY.
func TestDoctorFreshness_FreshSourceCheckoutPasses(t *testing.T) {
	abs := t.TempDir()
	seamInstallInto(t, abs)
	writeCorpusGo(t, abs)
	copyEmbeddedCorpusToDisk(t, abs) // fresh by construction

	out := seamDoctorOut(t, abs)
	if !strings.Contains(out, "dev-stale-embed") {
		t.Errorf("fresh source checkout should still show a dev-stale-embed section (PASS); got:\n%s", out)
	}
	if strings.Contains(out, "dev-stale-embed WARN") {
		t.Errorf("fresh source checkout must not WARN; got:\n%s", out)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("fresh source checkout must stay HEALTHY; got:\n%s", out)
	}
}

// --- consumer: no new section ----------------------------------------------

// TestDoctorFreshness_ConsumerSeesNoDevStaleSection: a consumer (no corpus.go +
// templates/core) must see NO dev-stale-embed section — the guard is structurally
// inert for every consumer. doctor behavior is byte-identical to before.
func TestDoctorFreshness_ConsumerSeesNoDevStaleSection(t *testing.T) {
	abs := t.TempDir()
	seamInstallInto(t, abs)
	// No corpus.go, no templates/core → consumer.

	out := seamDoctorOut(t, abs)
	if strings.Contains(out, "dev-stale-embed") {
		t.Errorf("consumer must see NO dev-stale-embed section; got:\n%s", out)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("consumer doctor must stay HEALTHY; got:\n%s", out)
	}
	// managed-drift detail is unqualified (no dev-stale suffix).
	if strings.Contains(out, "dev-stale:") || strings.Contains(out, "embedded corpus in this binary") {
		t.Errorf("consumer managed-drift must not carry the dev-stale qualifier; got:\n%s", out)
	}
}

// --- vendored templates/core without corpus.go is a consumer ---------------

// TestDoctorFreshness_VendoredTemplatesCoreWithoutCorpusGoIsConsumer: a target
// that vendors templates/core but lacks corpus.go is a CONSUMER for the guard.
// doctor shows no dev-stale-embed section (templates/core alone is too weak).
func TestDoctorFreshness_VendoredTemplatesCoreWithoutCorpusGoIsConsumer(t *testing.T) {
	abs := t.TempDir()
	seamInstallInto(t, abs)
	copyEmbeddedCorpusToDisk(t, abs)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")
	// Deliberately no corpus.go.

	out := seamDoctorOut(t, abs)
	if strings.Contains(out, "dev-stale-embed") {
		t.Errorf("vendored templates/core without corpus.go is a consumer; no dev-stale section expected; got:\n%s", out)
	}
}

// --- not seeded: doctor still runs (no crash on missing live tree) ---------

// TestDoctorFreshness_DevStaleOnUninitializedSourceCheckout: a source checkout
// that is NOT seam-installed still runs the freshness diagnostic (the guard
// does not depend on lineage). Other checks may WARN/FAIL (lineage absent,
// managed-drift missing), but dev-stale-embed must still appear and be WARN
// when the corpus differs. Proves the diagnostic is independent of install
// state.
func TestDoctorFreshness_DevStaleOnUninitializedSourceCheckout(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")
	// No seamInstallInto: no lineage, no live .opencode tree.

	out := seamDoctorOut(t, abs)
	if !strings.Contains(out, "dev-stale-embed WARN") {
		t.Errorf("dev-stale-embed should WARN on a differs source checkout even when not installed; got:\n%s", out)
	}
	// The managed-drift FAIL (missing files) is real and expected; dev-stale
	// just additionally qualifies it. Assert the qualifier is appended.
	if !strings.Contains(out, "dev-stale:") {
		t.Errorf("managed-drift detail should carry the dev-stale qualifier even on FAIL; got:\n%s", out)
	}
}
