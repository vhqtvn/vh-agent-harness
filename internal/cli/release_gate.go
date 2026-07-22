package cli

// release_gate.go — the §4.3 GENERIC release-readiness defer-liveness gate.
//
// This is the SAFETY LAYER acting (per the claim-verifier closure-kernel memo's
// authority line: "coordinator state INFORMS; safety-layer gates ACT"). It is a
// release-readiness GATE, not a registry: it consumes the §4.1 claim/verifier
// closure kernel (internal/memory/claims) — a NON-AUTHORITATIVE typed
// projection of the two on-disk sides — and FAILs the build/release when they
// contradict.
//
//   Side A — open coordinator task cards: .local/coordinator/tasks/defer-*.json
//            and errata-*.json (transport state, NOT git-tracked).
//   Side B — released/about-to-release claims: templates/migrations/v<semver>.md
//            (every released version is a published git tag; an untagged note is
//            "about to release").
//
// The gate NO LONGER reads those sources inline. The claims kernel
// (claims.Derive) is the single derivation path that reads both sides and
// classifies them; this gate is its one consumer. That closes the dual-derivation
// risk (two code paths reading the same sources and drifting apart). The kernel
// is strictly read/inform: it never writes canon, never persists, never blocks a
// release — ONLY this gate acts / FAILs.
//
// A card is a RELEASE-BLOCKING CONTRADICTION iff ALL hold:
//  1. its status is OPEN (not in the closed set {completed, cancelled, staged}),
//     AND
//  2. it references a claim present on Side B, where "references" means EITHER
//     (a) the card is an errata card (filename prefix "errata-") — errata cards
//     ARE released-claim contradiction records by definition, so the existence
//     of the contradicted released note is implied by the card's provenance
//     (side B confirms the released corpus exists); OR
//     (b) the card's files_in_scope / rough_scope mentions a path of the form
//     templates/migrations/v<semver>.md whose note actually exists on disk.
//
// LINEAGE: this gate ABSORBS the former internal/cli/erratum_gate_test.go. The
// old gate was a narrow one-sided proxy ("any draft errata-*.json card fails").
// This gate is a strict superset: it preserves "open errata card → FAIL"
// exactly (the errata subset) and generalizes to "open defer card that targets
// an existing migration note → FAIL", reading BOTH sides instead of status
// alone. The old standalone test was deleted; its behavior is now exercised as
// a fixture subset of this gate (see release_gate_test.go).
//
// FAIL-CLOSED invariant (preserved through the registry path): the kernel reads
// the SAME on-disk sources the gate used to read inline and surfaces the SAME
// errors — a malformed defer/errata card becomes a CardError (→ FAIL), an
// unreadable dir becomes a Derive error (→ FAIL). Moving the read seam into the
// kernel introduces NO fail-open path: there is no new persistent file that can
// silently disappear (the kernel re-derives from sources each call; a persisted
// registry would, by contrast, PASS when lost because store.Read returns empty
// with nil error — see the claims package doc for why the kernel deliberately
// does not persist).
//
// symptom_signature is intentionally NOT used here (deferred, parked). Open
// defers are keyed by task_id + path for diagnostics only; recurrence identity
// is out of scope for this slice.

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/claims"
)

// deferLivenessContradiction is a single release-blocking finding: an open card
// that references a present released/about-to-release claim.
type deferLivenessContradiction struct {
	card          claims.DeferCard
	versions      []string // existing notes the card explicitly names (sorted)
	errataImplied bool     // errata card: contradicts a released claim by definition
}

func (cx deferLivenessContradiction) String() string {
	id := cx.card.TaskID
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(cx.card.Path), ".json")
	}
	title := cx.card.Title
	if title == "" {
		title = "(no title)"
	}
	status := strings.TrimSpace(cx.card.Status)
	if status == "" {
		status = "(no status)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s [status=%s]: %s", id, status, title)
	if len(cx.versions) > 0 {
		fmt.Fprintf(&b, " — references existing note(s): %s", strings.Join(cx.versions, ", "))
	}
	if cx.errataImplied {
		b.WriteString(" [errata card: records an uncorrected false claim in a released note]")
	}
	return b.String()
}

// checkDeferLiveness is the 12th doctor check and the §4.3 generic
// release-readiness gate. It consumes the claims kernel's typed projection
// (claims.Derive) of Side A (open coordinator task cards) and Side B
// (released/about-to-release migration notes), then FAILs when an open card
// references a claim present on Side B — the class of miss that left
// errata-v0120 unresolved across v0.12.0 → v0.13.0 → v0.13.1 → v0.14.0 despite
// its trigger firing repeatedly.
//
// TIERING:
//   - FAIL when one or more open cards contradict present claims (the release
//     blocker). Names every offending card and the note(s) it references.
//   - FAIL when a selected defer/errata card is unreadable or unparseable: such
//     a card cannot be classified open-or-closed, and SKIPping it would let a
//     lexically-earlier malformed card mask a later valid open contradiction
//     (fail-open). The gate is FAIL-CLOSED here and names the offending card(s).
//   - PASS when both sides are present, all cards parse, and no contradiction
//     exists.
//   - SKIP when the gate cannot run honestly: git absent / target not a git
//     work tree (cannot classify notes as released vs about-to-release), the
//     transport tasks dir is absent (no Side A), or no migration notes are
//     present (no Side B to contradict — even an open errata card has no
//     shipped claim to contradict in such a tree).
//
// The check is READ-ONLY: it never mutates a card, never edits a migration
// note, and never shells out mutatingly. Git is used only for `tag -l`
// (classify notes) and the work-tree probe. The claims kernel it calls is
// equally non-authoritative (read/inform only).
func checkDeferLiveness(target string) checkResult {
	const name = "defer-liveness"

	// 1. Git availability + work tree (mirror checkRuntimeStateGitignored /
	//    checkAutoGateGitignored). Git classifies notes as released vs
	//    about-to-release via tags; without it the gate cannot read Side B
	//    honestly.
	if _, err := exec.LookPath("git"); err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "git not on PATH"}
	}
	wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(wt)) != "true" {
		return checkResult{name: name, tier: tierSkip, detail: "not a git work tree"}
	}

	// 2. The single source of truth: the claims kernel derives BOTH sides.
	reg, err := claims.Derive(target)
	if err != nil {
		// Directory-level I/O failure on a source the gate needs → FAIL (not SKIP).
		return checkResult{name: name, tier: tierFail, detail: "could not read claim sources: " + err.Error()}
	}
	if !reg.TasksPresent {
		return checkResult{name: name, tier: tierSkip, detail: "no .local/coordinator/tasks/ state (transport dir absent)"}
	}
	if !reg.NotesPresent {
		// No Side B: no shipped/about-to-ship claim to contradict. Even an open
		// errata card has no released claim to contradict in this tree (e.g. a
		// core-only or pre-migration-notes checkout). Clean no-op.
		return checkResult{name: name, tier: tierSkip, detail: "no templates/migrations/v*.md notes present"}
	}

	// 3. Cross-reference both sides via the kernel's typed projection.
	contradictions := findDeferLivenessContradictions(reg.Cards, reg.Notes)
	openCount := 0
	for _, c := range reg.Cards {
		if !claims.CardIsClosed(c) {
			openCount++
		}
	}
	relCnt, unrelCnt := countReleasedNotes(reg.Notes)

	// Fail-closed: a malformed/unreadable card is itself a release blocker,
	// reported alongside any open-claim contradictions.
	if len(reg.CardErrors) == 0 && len(contradictions) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d card(s) (%d open), %d note(s) (%d released, %d about-to-release); no release-blocking contradiction",
				len(reg.Cards), openCount, len(reg.Notes), relCnt, unrelCnt)}
	}

	var b strings.Builder
	if len(reg.CardErrors) > 0 {
		fmt.Fprintf(&b, "%d unreadable/unparseable defer/errata card(s) (gate is fail-closed — fix or remove before release):",
			len(reg.CardErrors))
		for _, ce := range reg.CardErrors {
			fmt.Fprintf(&b, "\n  - %s: %v", filepath.Join(".local", "coordinator", "tasks", filepath.Base(ce.Path)), ce.Err)
		}
		if len(contradictions) > 0 {
			b.WriteByte('\n')
		}
	}
	if len(contradictions) > 0 {
		fmt.Fprintf(&b, "%d open defer/errata card(s) contradict released/about-to-release claims (%d released, %d about-to-release notes):",
			len(contradictions), relCnt, unrelCnt)
		for _, cx := range contradictions {
			b.WriteString("\n  - ")
			b.WriteString(cx.String())
		}
		b.WriteString("\nresolve each card by moving its status into the closed set (completed/cancelled/staged) — e.g. inject the correction into the target migration note and stage the card, or explicitly dismiss it. Released notes are immutable; corrections ship as errata in the next release's note.")
	}
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// findDeferLivenessContradictions cross-references Side A (cards) against Side B
// (notes). A card is a contradiction iff it is OPEN and either (a) it is an
// errata card (released-claim contradiction by definition — Side B is already
// confirmed non-empty by the caller), or (b) its pre-computed
// ReferencedVersions includes a note version that exists on disk.
func findDeferLivenessContradictions(cards []claims.DeferCard, notes []claims.NoteClaim) []deferLivenessContradiction {
	exists := map[string]bool{}
	for _, n := range notes {
		exists[n.Version] = true
	}
	var out []deferLivenessContradiction
	for _, c := range cards {
		if claims.CardIsClosed(c) {
			continue
		}
		var hits []string
		for _, v := range c.ReferencedVersions {
			if exists[v] {
				hits = append(hits, v)
			}
		}
		if len(hits) == 0 && !c.IsErrata {
			// Open card that references no present migration note — not a
			// release-blocking contradiction (e.g. a code-level defer with no
			// released-claim surface).
			continue
		}
		sort.Strings(hits)
		out = append(out, deferLivenessContradiction{card: c, versions: hits, errataImplied: c.IsErrata})
	}
	return out
}

// countReleasedNotes splits a note set into released vs about-to-release counts.
func countReleasedNotes(notes []claims.NoteClaim) (released, aboutToRelease int) {
	for _, n := range notes {
		if n.Released {
			released++
		} else {
			aboutToRelease++
		}
	}
	return
}
