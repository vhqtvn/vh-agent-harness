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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

	// ── defer-003: coordinator-adoption liveness (the channel-liveness tradeoff) ──
	//
	// The committed coordinator-adoption marker at
	// .vh-agent-harness/coordinator-adoption.json is git-tracked CHANNEL
	// META-STATE; the cards under .local/coordinator/tasks/ are gitignored LOSABLE
	// TRANSPORT. Before defer-003, a whole-directory transport loss on a repo that
	// HAD adopted the coordinator would silently SKIP this check on a fresh
	// checkout — a fail-open in a safety gate after adoption. Option A (operator-
	// approved) closes WHOLE-DIRECTORY loss: the adoption marker makes the gate
	// authoritative, so a valid marker with an absent transport dir FAILs instead
	// of SKIPping.
	//
	// TRADEOFF (accepted, recorded in templates/migrations/v0.16.0.md): this closes
	// whole-directory loss but NOT selective single-card deletion while the dir
	// remains (e.g. errata-v0120 deleted with the dir intact). That is accepted
	// because whole-dir loss is the structurally-forced gitignore vector, and an
	// actor who can delete one card can also delete the committed marker.
	//
	// Authority line: the claims kernel only DERIVES the marker state
	// (absent/valid/corrupt) — it does NOT choose SKIP/FAIL. This switch is the
	// SOLE transition authority that maps those states to tiers, mirroring how the
	// kernel appends to reg.CardErrors without deciding its tier.
	//
	// State matrix (all 5 rows, test-mandated):
	//   tasks absent + marker absent  → tierSkip (greenfield / never adopted)
	//   tasks present + marker absent → tierSkip (do NOT infer adoption from losable transport)
	//   tasks present + marker valid  → run the existing gate below (row 3)
	//   tasks absent + marker valid   → tierFail  (THE FIX — adopted transport dir lost)
	//   either       + marker corrupt → tierFail  (fail-closed, mirrors CardError)
	switch reg.AdoptionMarker {
	case claims.AdoptionMarkerCorrupt:
		return checkResult{name: name, tier: tierFail,
			detail: "coordinator-adoption marker is corrupt/unreadable (fail-closed — fix or remove .vh-agent-harness/coordinator-adoption.json before release): " + reg.AdoptionMarkerDetail}
	case claims.AdoptionMarkerAbsent:
		// Rows 1 & 2: no committed adoption marker → the coordinator transport is
		// not trusted to gate releases. SKIP regardless of whether stray task
		// cards exist (do not retroactively infer adoption from losable transport).
		return checkResult{name: name, tier: tierSkip,
			detail: "coordinator transport not adopted (no committed .vh-agent-harness/coordinator-adoption.json marker) — defer-liveness gate inactive"}
	case claims.AdoptionMarkerValid:
		if !reg.TasksPresent {
			// Row 4 (THE FIX): the adoption marker says this repo HAS adopted the
			// coordinator, but the transport dir is gone. FAIL rather than silently
			// SKIPping the whole check.
			return checkResult{name: name, tier: tierFail,
				detail: "coordinator-adoption marker is set but .local/coordinator/tasks/ is absent — adopted transport dir was lost; re-restore it from git history or rerun a coordinator task to repopulate before release"}
		}
		// Row 3: marker valid + tasks present → fall through to the existing
		// card/contradiction logic below.
	}
	if !reg.NotesPresent {
		// No Side B: no shipped/about-to-ship claim to contradict. Even an open
		// errata card has no released claim to contradict in this tree (e.g. a
		// core-only or pre-migration-notes checkout). Clean no-op.
		return checkResult{name: name, tier: tierSkip, detail: "no templates/migrations/v*.md notes present"}
	}

	// 3. Cross-reference both sides via the kernel's typed projection (the
	//    ORIGINAL contradiction class: an OPEN defer/errata card referencing a
	//    present released/about-to-release migration note).
	contradictions := findDeferLivenessContradictions(reg.Cards, reg.Notes)
	openCount := 0
	for _, c := range reg.Cards {
		if !claims.CardIsClosed(c) {
			openCount++
		}
	}
	relCnt, unrelCnt := countReleasedNotes(reg.Notes)

	// 4. F4-C release-diff-recurrence predicate (the NEW contradiction class).
	//    An OPEN liveness card whose path_touched target re-fires in the release
	//    diff (PRIOR_TAG..HEAD) and which lacks a fresh disposition (closed
	//    status / manifest entry / operator override). This seals the F1-D1 gap:
	//    deferred debt whose trigger re-fires in the release arc can no longer
	//    ship without a forced verdict.
	//
	//    ACTIVATION: the recurrence predicate's BLOCK escalates to FAIL only
	//    when a release is imminent (an about-to-release / untagged migration
	//    note exists) — mirroring check #13's proven pattern. When no release is
	//    imminent the findings are advisory (the predicate is dormant) so doctor
	//    stays HEALTHY during ordinary development. At tag time (G0c) the
	//    ceremony has created the about-to-release note, so the predicate is
	//    active and refuses on any undisposed fired card. Fog (no target) and
	//    COLD (glob/dir non-grammar) findings are ALWAYS advisory (INFORM, never
	//    block).
	recReport := evaluateDeferRecurrence(target, reg)
	releaseImminent := unrelCnt > 0

	hasCardErrors := len(reg.CardErrors) > 0
	hasContradictions := len(contradictions) > 0
	hasRecurrenceBlock := releaseImminent && len(recReport.blockers) > 0
	// Recurrence-ack enforcement (Slice 5): an unacknowledged recurrence
	// (derived count > committed manifest ack) or an uncollapsed duplicate
	// (producer-bypass) is a gate-consistency failure → BLOCKED at release time.
	// Dormant when no release is imminent (mirrors the F4-C predicate).
	ackReport := evaluateRecurrenceAck(target)
	hasAckBlock := releaseImminent && len(ackReport.blockers) > 0

	if !hasCardErrors && !hasContradictions && !hasRecurrenceBlock && !hasAckBlock {
		detail := fmt.Sprintf("%d card(s) (%d open), %d note(s) (%d released, %d about-to-release); no release-blocking contradiction",
			len(reg.Cards), openCount, len(reg.Notes), relCnt, unrelCnt)
		if extra := recReport.advisoryDetail(); extra != "" {
			detail += "\n" + extra
		}
		if adv := ackReport.advisoryDetail(); adv != "" {
			detail += "\n" + adv
		}
		return checkResult{name: name, tier: tierPass, detail: detail}
	}

	var b strings.Builder
	if hasCardErrors {
		fmt.Fprintf(&b, "%d unreadable/unparseable defer/errata card(s) (gate is fail-closed — fix or remove before release):",
			len(reg.CardErrors))
		for _, ce := range reg.CardErrors {
			fmt.Fprintf(&b, "\n  - %s: %v", filepath.Join(".local", "coordinator", "tasks", filepath.Base(ce.Path)), ce.Err)
		}
	}
	if hasContradictions {
		if hasCardErrors {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d open defer/errata card(s) contradict released/about-to-release claims (%d released, %d about-to-release notes):",
			len(contradictions), relCnt, unrelCnt)
		for _, cx := range contradictions {
			b.WriteString("\n  - ")
			b.WriteString(cx.String())
		}
		b.WriteString("\nresolve each card by moving its status into the closed set (completed/cancelled/staged) — e.g. inject the correction into the target migration note and stage the card, or explicitly dismiss it. Released notes are immutable; corrections ship as errata in the next release's note.")
	}
	if hasRecurrenceBlock {
		if hasCardErrors || hasContradictions {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d open defer card(s) whose path_touched target re-fired in the release diff (%s) without a fresh disposition (release-diff-recurrence, F4-C):",
			len(recReport.blockers), recReport.diffRef)
		for _, blk := range recReport.blockers {
			fmt.Fprintf(&b, "\n  - %s [status=%s]: fired target(s) %s",
				livenessCardLabel(blk.card), livenessStatusLabel(blk.card), quotedList(blk.targets))
		}
		b.WriteString("\nresolve each card by: (a) moving its status into the closed set (completed/cancelled/staged), (b) adding a disposition record for its task_id to .vh-agent-harness/release-defer-dispositions.json, or (c) supplying an operator override via the VH_HARNESS_DEFER_OVERRIDE_IDS env var.")
	}
	if hasAckBlock {
		if hasCardErrors || hasContradictions || hasRecurrenceBlock {
			b.WriteByte('\n')
		}
		b.WriteString(formatRecurrenceAckBlockers(ackReport.blockers))
	}
	// Advisory recurrence findings (fog / cold / dormant) are appended to the
	// detail whenever present, even on a FAIL, so the operator sees the full
	// recurrence surface. They never escalate the tier on their own.
	if adv := recReport.advisoryDetail(); adv != "" {
		b.WriteString("\n")
		b.WriteString(adv)
	}
	// Dormant ack blockers (no release imminent) are appended as INFORM so the
	// operator sees pending re-adjudication state. Skipped when hasAckBlock:
	// those blockers are already rendered above as the active FAIL reason.
	if !hasAckBlock {
		if adv := ackReport.advisoryDetail(); adv != "" {
			b.WriteString("\n")
			b.WriteString(adv)
		}
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

// --- F4-C release-diff-recurrence predicate (the SECOND contradiction class) ---
//
// This is the F1-D1 gap fix: 16 `draft` DEFER cards whose path_touched target
// sits in the release diff would otherwise ship without a forced verdict (the
// case study's §4.3 deferred-debt-decay failure at population scale). The
// predicate re-evaluates every OPEN liveness card's trigger against the release
// diff (PRIOR_TAG..HEAD) and refuses while any card whose target re-fired lacks
// a fresh disposition (closed status / manifest entry / operator override).
//
// Federated authority: the GATE+G0c refuse; the RELEASER/OPERATOR dispose; the
// manifest M is the releaser's (the committed disposition record rechecked by
// the release-mode evaluator in check-defer-triggers.js). Doctor #12 reads the
// manifest only for manifest_entry(c) — it does NOT validate the handshake
// (that is the release-mode evaluator's job).
//
// Silencer immunity: the predicate consults reg.LivenessCards — EVERY file under
// .local/coordinator/tasks/ (no prefix/extension filter) — not reg.Cards (the
// defer-/errata- *.json subset). A card cannot escape the gate by being named
// without the defer- prefix or by being .md instead of .json.

// recurrenceBlocker is a single release-blocking finding: an OPEN liveness card
// whose path_touched target exact-matches a path in the release diff and which
// lacks a fresh disposition.
type recurrenceBlocker struct {
	card    claims.LivenessCard
	targets []string // exact-match targets that fired (sorted)
}

// recurrenceReport carries the predicate's full output. blockers escalate to
// FAIL only when a release is imminent (checkDeferLiveness maps releaseImminent
// from about-to-release note count). fogCards and coldCards are ALWAYS advisory
// (INFORM, never block): a fog card declares no target; a cold card declares
// only non-grammar (glob/dir) targets that v1 exact-match cannot resolve.
type recurrenceReport struct {
	diffRef      string          // the PRIOR_TAG used (e.g. "v0.18.0"); "(none)" if no prior tag
	diffComputed bool            // whether the diff path set was computed
	diffPaths    map[string]bool // set of changed paths (empty if not computed)
	blockers     []recurrenceBlocker
	fogCards     []claims.LivenessCard
	coldCards    []claims.LivenessCard
}

// evaluateDeferRecurrence computes the full F4-C release-diff-recurrence
// surface. It never returns an error: an uncomputable diff (no prior tag on a
// greenfield repo, or a transient git failure) yields an empty report (no
// blockers) — fail-SAFE (the release-mode manifest evaluator in
// check-defer-triggers.js is the independent second enforcement surface). The
// release-context threading (G0c passes PRIOR_TAG via the
// VH_HARNESS_DEFER_DIFF_SINCE env var) takes precedence over the self-derived
// git describe, so the tag-time gate and the predicate agree on the boundary.
func evaluateDeferRecurrence(target string, reg claims.Registry) recurrenceReport {
	rep := recurrenceReport{}

	// Resolve the diff-since ref: honor the release-context env var (G0c
	// threading), else derive from the most recent tag.
	diffSince := os.Getenv("VH_HARNESS_DEFER_DIFF_SINCE")
	if diffSince == "" {
		out, err := exec.Command("git", "-C", target, "describe", "--tags", "--abbrev=0").Output()
		if err != nil {
			rep.diffRef = "(none)"
			return rep
		}
		diffSince = strings.TrimSpace(string(out))
	}
	rep.diffRef = diffSince

	// Compute the diff path set (PRIOR_TAG..HEAD).
	out, err := exec.Command("git", "-C", target, "diff", "--name-only", diffSince).Output()
	if err != nil {
		rep.diffRef = diffSince + " (diff-unavailable)"
		return rep
	}
	rep.diffComputed = true
	rep.diffPaths = map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.TrimSpace(line)
		if p != "" {
			rep.diffPaths[p] = true
		}
	}

	// Disposition-satisfaction surfaces.
	manifestIDs := manifestEntryIDs(target)
	overrideIDs := overrideIDSet()

	for _, c := range reg.LivenessCards {
		// disposition_satisfied(c): closed status OR manifest entry OR override.
		if claims.StatusIsClosed(c.Status) {
			continue
		}
		if manifestIDs[c.TaskID] {
			continue
		}
		if overrideIDs[c.TaskID] {
			continue
		}
		// Fog: no targets at all → INFORM only.
		if len(c.Targets) == 0 {
			rep.fogCards = append(rep.fogCards, c)
			continue
		}
		// Classify targets: exact vs non-grammar (glob/dir).
		var exactTargets, nonGrammarTargets []string
		for _, t := range c.Targets {
			if isNonGrammarTarget(t) {
				nonGrammarTargets = append(nonGrammarTargets, t)
			} else {
				exactTargets = append(exactTargets, t)
			}
		}
		// Fired: at least one exact target in the diff.
		var fired []string
		for _, t := range exactTargets {
			if rep.diffPaths[t] {
				fired = append(fired, t)
			}
		}
		if len(fired) > 0 {
			sort.Strings(fired)
			rep.blockers = append(rep.blockers, recurrenceBlocker{card: c, targets: fired})
		} else if len(exactTargets) == 0 && len(nonGrammarTargets) > 0 {
			// COLD: only non-grammar targets (v1 exact-match cannot resolve) → INFORM.
			rep.coldCards = append(rep.coldCards, c)
		}
		// else: exact targets exist but none fired → silent (card's trigger is
		// not in this release arc).
	}
	return rep
}

// manifestEntryIDs reads the committed disposition manifest at
// .vh-agent-harness/release-defer-dispositions.json and returns the set of
// defer_id values that carry a releaser/operator disposition. Doctor #12 uses
// this for manifest_entry(c). It does NOT validate the handshake (schema,
// evaluated_commit-vs-HEAD) — that is the release-mode evaluator's job; doctor
// only needs to know WHICH cards are accounted for. A missing/unparseable
// manifest yields an empty set: no manifest_entry is satisfied, so the closed-
// status check still carries the disposition burden (fail-safe, not silent).
func manifestEntryIDs(repoRoot string) map[string]bool {
	ids := map[string]bool{}
	path := filepath.Join(repoRoot, ".vh-agent-harness", "release-defer-dispositions.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return ids
	}
	var obj struct {
		Records []struct {
			DeferID string `json:"defer_id"`
		} `json:"records"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return ids
	}
	for _, r := range obj.Records {
		if r.DeferID != "" {
			ids[r.DeferID] = true
		}
	}
	return ids
}

// overrideIDSet returns the operator live-intent override set from the
// VH_HARNESS_DEFER_OVERRIDE_IDS env var (comma-separated task_ids). This is the
// escape hatch for an operator to dispose a card at doctor time without a
// manifest commit (the federated authority: releaser/operator dispose).
func overrideIDSet() map[string]bool {
	ids := map[string]bool{}
	for _, raw := range strings.Split(os.Getenv("VH_HARNESS_DEFER_OVERRIDE_IDS"), ",") {
		id := strings.TrimSpace(raw)
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

// isNonGrammarTarget reports whether a path_touched arg is a non-exact-match
// (glob/dir-prefix) operand. v1 recurrence resolution is EXACT-match only; such
// targets are COLD (logged, never block). A target ending in '/' is a directory
// operand; a target containing glob metacharacters (* ? [ {) is a glob. Both
// are non-grammar for v1 exact-match. glob/dir-prefix path_touched semantics
// are deferred to v2 (defer-glob-trigger-v2).
func isNonGrammarTarget(t string) bool {
	if strings.HasSuffix(t, "/") {
		return true
	}
	return strings.ContainsAny(t, "*?[{")
}

// livenessCardLabel renders a LivenessCard for the gate detail: the task_id,
// title (if any), and the source filename so the operator can locate the card
// file under .local/coordinator/tasks/.
func livenessCardLabel(c claims.LivenessCard) string {
	base := filepath.Base(c.Path)
	if c.Title != "" {
		return fmt.Sprintf("%s (%s) [%s]", c.TaskID, c.Title, base)
	}
	return fmt.Sprintf("%s [%s]", c.TaskID, base)
}

// livenessStatusLabel renders a card's status for the gate detail, distinguishing
// a JSON-parsed status from a non-JSON card's implicit OPEN default.
func livenessStatusLabel(c claims.LivenessCard) string {
	if c.Status == "" {
		return "open (implicit, non-JSON card)"
	}
	return c.Status
}

// quotedList renders a string slice as a quoted, comma-separated list for the
// gate detail.
func quotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = strconv.Quote(s)
	}
	return strings.Join(quoted, ", ")
}

// advisoryDetail renders the fog/cold/dormant advisory findings as a detail
// block. Returns "" when there is nothing advisory to report. These never
// escalate the tier: they INFORM the operator of the recurrence surface. Fog
// and cold ID lists are capped (first 8 shown, remainder summarized) so a large
// task dir does not flood the gate detail.
func (r recurrenceReport) advisoryDetail() string {
	if !r.diffComputed && len(r.fogCards) == 0 && len(r.coldCards) == 0 {
		return "" // nothing computed, nothing advisory
	}
	var b strings.Builder
	fmt.Fprintf(&b, "release-diff-recurrence (F4-C): diff-since=%s", r.diffRef)
	if !r.diffComputed {
		b.WriteString("; diff not computable (predicate dormant — manifest evaluator is the independent second surface)")
		if len(r.fogCards) == 0 && len(r.coldCards) == 0 {
			return b.String()
		}
	}
	if len(r.fogCards) > 0 {
		b.WriteString("; ")
		b.WriteString(summarizeIDs("fog", "no trigger target, INFORM only", r.fogCards))
	}
	if len(r.coldCards) > 0 {
		b.WriteString("; ")
		b.WriteString(summarizeIDs("cold", "glob/dir target, INFORM only until v2", r.coldCards))
	}
	return b.String()
}

// summarizeIDs renders a labeled, capped list of card IDs: the first 8 IDs,
// then "+N more" if the list is longer. Keeps the gate detail readable on a
// large task dir.
func summarizeIDs(kind, note string, cards []claims.LivenessCard) string {
	const cap = 8
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s card(s) (%s):", len(cards), kind, note)
	rest := cards
	if len(rest) > cap {
		rest = rest[:cap]
	}
	for _, c := range rest {
		fmt.Fprintf(&b, " %s", c.TaskID)
	}
	if len(cards) > cap {
		fmt.Fprintf(&b, " (+%d more)", len(cards)-cap)
	}
	return b.String()
}

// --- doctor check #13: staged-errata-content (the THIRD failure mode) ---
//
// This check closes the hole that let errata-v0120 ship uncorrected across four
// releases (v0.12.0 → v0.13.0 → v0.13.1 → v0.14.0). Doctor check #12
// (defer-liveness) treats `staged` as a CLOSED (passing) status — it only fails
// on OPEN cards. But "staged" only means "correction queued for the next
// release"; it says nothing about whether the correction was actually INJECTED
// into the about-to-release note. A staged erratum whose content is missing from
// the note that is about to ship is the exact defect this check catches.
//
// The check is the mechanism half of the auto-inject closure: once the inject
// subcommand (`release inject-errata`) copies the erratum body into the note AND
// flips the card staged→completed, this check sees no staged cards and passes.
// If the inject was forgotten, the card stays staged and the about-to-release
// note lacks the content → FAIL, naming the offending card.

// erratumTitleVersionRe extracts the target version from an erratum file's
// title line, e.g. "# Erratum: v0.12.0 — media-perception rendering claims".
var erratumTitleVersionRe = regexp.MustCompile(`(?i)^#\s*Erratum:\s*(v\d+\.\d+\.\d+)`)

// stagedStatus is the card status that marks an erratum as "correction queued
// for the next release" — the status defer-liveness (#12) treats as closed but
// this check (#13) holds open against the note content.
const stagedStatus = "staged"

// checkStagedErrataContent is the 13th doctor check. It FAILs when an
// about-to-release (untagged) migration note exists AND any errata card with
// status "staged" has correction content NOT present in that note — the third
// failure mode from the reconciliation matrix. It composes with check #12
// (defer-liveness): #12 holds open cards, #13 holds staged cards accountable to
// the note they claim to correct.
//
// TIERING:
//   - SKIP when no about-to-release notes exist (no imminent release — the
//     current steady-state of this repo, which has no untagged migration note),
//     or when the gate cannot run honestly (git absent / not a work tree / no
//     tasks dir / no notes).
//   - PASS when there are no staged errata cards (the common post-inject
//     steady state — inject already flipped them all to completed), or when
//     every staged errata card's content is present in the highest-version
//     about-to-release note.
//   - FAIL when a staged errata card's correction content is absent from the
//     HIGHEST-version about-to-release note, naming the offending card + the
//     target note that was checked. Also FAIL (fail-closed) when a staged card's
//     staged_path is unreadable — a staged card pointing at a missing erratum
//     file is itself a release blocker.
//
// TARGET SELECTION: the gate targets ONLY the highest-version untagged note —
// the SAME note that `release inject-errata` injects into (about[len-1] after
// claims.Derive's ascending NUMERIC semver sort). This alignment is
// load-bearing: checking against ANY note (the round-2 design) would let a
// correction present only in a stale/lower note PASS while the note that
// actually ships lacks it. The semver sort (not lexicographic) ensures
// v0.10.0 ranks above v0.9.0. If there are no staged cards, the target
// selection is irrelevant (PASS on empty staged set — the common post-inject
// steady state).
//
// The content check is a REAL signature match (not card status, not a bare
// "## Errata" header): the erratum file's corrective body — everything after the
// title line and the status blockquote, whitespace-normalized — must appear as a
// substring of the highest about-to-release note's normalized text. This is
// exactly what `release inject-errata` produces (verbatim copy under a section
// heading), so the check passes deterministically after inject and fails when a
// human wrote only a stub header.
//
// READ-ONLY: never mutates a card or note.
func checkStagedErrataContent(target string) checkResult {
	const name = "staged-errata-content"

	// 1. Git availability + work tree (same preconditions as #12: git classifies
	//    notes as released vs about-to-release).
	if _, err := exec.LookPath("git"); err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "git not on PATH"}
	}
	wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(wt)) != "true" {
		return checkResult{name: name, tier: tierSkip, detail: "not a git work tree"}
	}

	reg, err := claims.Derive(target)
	if err != nil {
		return checkResult{name: name, tier: tierFail, detail: "could not read claim sources: " + err.Error()}
	}
	if !reg.TasksPresent || !reg.NotesPresent {
		return checkResult{name: name, tier: tierSkip, detail: "no tasks/notes state to verify"}
	}

	// 2. Only about-to-release notes are in scope (an imminent release). No
	//    untagged note means no release is imminent → SKIP (nothing to inject
	//    into). This is why the current steady-state repo (no v0.15.0 note)
	//    passes this check.
	aboutToRelease := aboutToReleaseNotes(reg.Notes)
	if len(aboutToRelease) == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: "no about-to-release (untagged) migration note — no imminent release to verify against"}
	}

	// 3. Only staged errata cards are in scope. A staged card whose staged_path
	//    is empty cannot be content-checked → fail-closed (a staged card with no
	//    staged erratum file is malformed). No staged errata cards at all →
	//    nothing to verify → PASS (the common post-inject steady state).
	staged := stagedErrataCards(reg.Cards)
	if len(staged) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d about-to-release note(s), no staged errata cards — nothing to verify", len(aboutToRelease))}
	}

	// 4. Target ONLY the highest-version about-to-release note — the same note
	//    that `release inject-errata` injects into (about[len-1] after ascending
	//    sort). Checking against ANY untagged note would let an erratum present
	//    only in a stale/older note PASS while the note that will actually ship
	//    lacks the correction. Aligning the gate's target with inject's target
	//    closes that gap (commit-review tier1b-F1).
	targetNote := aboutToRelease[len(aboutToRelease)-1]
	noteRaw, rerr := os.ReadFile(targetNote.Path)
	if rerr != nil {
		return checkResult{name: name, tier: tierFail,
			detail: fmt.Sprintf("unreadable about-to-release note %s: %v", targetNote.Version, rerr)}
	}
	noteNorm := normalizeForContentMatch(string(noteRaw))

	// 5. For each staged errata card, verify its correction body is present in
	//    the target (highest-version) about-to-release note.
	var missing []string
	var unreadable []string
	checked := 0
	for _, c := range staged {
		sp := strings.TrimSpace(c.StagedPath)
		if sp == "" {
			unreadable = append(unreadable, fmt.Sprintf("%s [status=staged]: no staged_path set (card is malformed)", cardLabel(c)))
			continue
		}
		// staged_path is repo-relative in the card; resolve against target.
		erratumPath := sp
		if !filepath.IsAbs(erratumPath) {
			erratumPath = filepath.Join(target, filepath.FromSlash(sp))
		}
		raw, rerr := os.ReadFile(erratumPath)
		if rerr != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s [status=staged]: staged_path %s unreadable: %v", cardLabel(c), sp, rerr))
			continue
		}
		targetVer, body := extractErratumSignature(string(raw))
		if body == "" {
			unreadable = append(unreadable, fmt.Sprintf("%s [status=staged]: staged_path %s has no corrective body (title/status only)", cardLabel(c), sp))
			continue
		}
		checked++
		if !strings.Contains(noteNorm, body) {
			ver := targetVer
			if ver == "" {
				ver = "(unknown version)"
			}
			missing = append(missing, fmt.Sprintf("%s [status=staged]: correction for %s absent from about-to-release note %s — run `vh-agent-harness release inject-errata` to inject it",
				cardLabel(c), ver, targetNote.Version))
		}
	}

	if len(unreadable) > 0 || len(missing) > 0 {
		var b strings.Builder
		if len(unreadable) > 0 {
			fmt.Fprintf(&b, "%d staged errata card(s) with unreadable/empty staged_path (fail-closed):", len(unreadable))
			for _, m := range unreadable {
				b.WriteString("\n  - ")
				b.WriteString(m)
			}
		}
		if len(missing) > 0 {
			if len(unreadable) > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%d staged errata card(s) whose correction is missing from the about-to-release note:", len(missing))
			for _, m := range missing {
				b.WriteString("\n  - ")
				b.WriteString(m)
			}
			b.WriteString("\nrun `vh-agent-harness release inject-errata` to inject each staged erratum and flip its card to completed, or stage the correction manually.")
		}
		return checkResult{name: name, tier: tierFail, detail: b.String()}
	}
	return checkResult{name: name, tier: tierPass,
		detail: fmt.Sprintf("%d staged errata card(s) verified injected into about-to-release note %s", checked, targetNote.Version)}
}

// aboutToReleaseNotes filters a note set to untagged (about-to-release) notes.
func aboutToReleaseNotes(notes []claims.NoteClaim) []claims.NoteClaim {
	var out []claims.NoteClaim
	for _, n := range notes {
		if !n.Released {
			out = append(out, n)
		}
	}
	return out
}

// stagedErrataCards filters a card set to errata cards whose status is "staged".
func stagedErrataCards(cards []claims.DeferCard) []claims.DeferCard {
	var out []claims.DeferCard
	for _, c := range cards {
		if c.IsErrata && strings.ToLower(strings.TrimSpace(c.Status)) == stagedStatus {
			out = append(out, c)
		}
	}
	return out
}

// cardLabel renders a short human-readable label for a card (task_id or
// filename-derived id, plus title).
func cardLabel(c claims.DeferCard) string {
	id := c.TaskID
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(c.Path), ".json")
	}
	title := c.Title
	if title == "" {
		title = "(no title)"
	}
	return fmt.Sprintf("%s: %s", id, title)
}

// extractErratumSignature parses an erratum markdown file into (targetVersion,
// normalizedBody). The title line "# Erratum: vX.Y.Z — ..." yields the target
// version; the corrective body is everything after the title line and the
// leading status blockquote (lines starting with '>'), whitespace-normalized
// (lowercase, runs collapsed to single spaces, trimmed). Returns ("", "") when
// the file has no recognizable title or no corrective body.
//
// The status blockquote is the instructional meta-text ("When creating the next
// migration note, include..."); stripping it leaves the actionable correction
// (the "## What was wrong" / "### N." specifics), which is exactly what inject
// copies under a "## Errata for vX.Y.Z" section and what this check looks for
// in the note.
func extractErratumSignature(erratumText string) (targetVersion, normalizedBody string) {
	ver, rawBody := splitErratumBody(erratumText)
	if rawBody == "" {
		return ver, ""
	}
	return ver, normalizeForContentMatch(rawBody)
}

// splitErratumBody parses an erratum markdown file into (targetVersion,
// rawBody) using a three-phase state machine:
//   - phase 0: scan for the title line "# Erratum: vX.Y.Z" (everything before
//     it is preamble/front-matter and skipped).
//   - phase 1: skip the leading status blockquote (lines starting with '>')
//     AND blank lines, until the first real content line. This strips the
//     instructional meta-text ("When creating the next migration note, include
//     ...") without requiring it to be the very first line after the title.
//   - phase 2: capture the corrective body verbatim (including any blockquotes
//     that legitimately appear inside the correction).
//
// Returns ("", "") when the file has no recognizable title or no corrective
// body. Shared by the staged-errata gate (check #13, which normalizes a copy
// for substring matching) and `release inject-errata` (which writes it raw
// under a section heading).
func splitErratumBody(erratumText string) (targetVersion, rawBody string) {
	lines := strings.Split(erratumText, "\n")
	var bodyLines []string
	phase := 0 // 0=looking for title, 1=skipping status blockquote preamble, 2=capturing body
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		switch phase {
		case 0:
			if m := erratumTitleVersionRe.FindStringSubmatch(trimmed); m != nil {
				targetVersion = m[1]
				phase = 1
			}
		case 1:
			// Skip the leading status blockquote ('>' lines) and blank lines until
			// we hit the first real content line.
			if strings.HasPrefix(trimmed, ">") || trimmed == "" {
				continue
			}
			phase = 2
			fallthrough
		case 2:
			bodyLines = append(bodyLines, ln)
		}
	}
	return targetVersion, strings.TrimSpace(strings.Join(bodyLines, "\n"))
}

// normalizeForContentMatch collapses a markdown body into a single
// whitespace-normalized lowercase string for substring content matching. It is
// intentionally NOT a structural markdown parse: it flattens whitespace so that
// heading/paragraph reflow or indentation differences do not defeat a verbatim
// body copy (which is what the inject subcommand produces).
func normalizeForContentMatch(s string) string {
	s = strings.ToLower(s)
	// Collapse every run of whitespace (spaces, tabs, newlines) to a single space.
	var b strings.Builder
	b.Grow(len(s))
	inWS := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			if !inWS {
				b.WriteByte(' ')
				inWS = true
			}
			continue
		}
		inWS = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
