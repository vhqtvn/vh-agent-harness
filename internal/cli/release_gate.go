package cli

// release_gate.go — the §4.3 GENERIC release-readiness defer-liveness gate.
//
// This is the SAFETY LAYER acting (per the claim-verifier closure-kernel memo's
// authority line: "coordinator state INFORMS; safety-layer gates ACT"). It is a
// release-readiness GATE, not a registry: it reads two on-disk sides and FAILs
// the build/release when they contradict.
//
//   Side A — open coordinator task cards: .local/coordinator/tasks/defer-*.json
//            and errata-*.json (transport state, NOT git-tracked).
//   Side B — released/about-to-release claims: templates/migrations/v<semver>.md
//            (every released version is a published git tag; an untagged note is
//            "about to release").
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
	"strings"
)

// semverVersionRe matches a bare release version token vX.Y.Z (no pre-release
// suffix). It MIRRORS semverTagRe in migration_release_test.go — that variable
// lives in a _test.go file and is therefore not visible to non-test code, so
// this file carries its own copy. Keep the two in sync.
var semverVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// migrationNotePathRe matches a migration-note path or filename embedded in a
// free-form scope string, e.g. "templates/migrations/v0.12.0.md" or the bare
// "v0.12.0.md". Used to detect when a defer card references a specific note.
// cardReferencedNoteVersions extracts ALL matches from a string (not just the
// first) so a scope entry naming two notes is fully evaluated.
var migrationNotePathRe = regexp.MustCompile(`v\d+\.\d+\.\d+\.md`)

// deferLivenessClosedStatuses are the task-card statuses that are no longer an
// OPEN contradiction against a released/about-to-release claim:
//   - completed: the correction landed / the defer was satisfied.
//   - cancelled: the defer was dismissed as no longer applicable.
//   - staged: a correction has been staged for the next release cut (e.g. an
//     erratum written to templates/migrations/errata/) — no longer an open
//     contradiction, it rides the next release.
//
// This generalizes the old erratum gate's "non-draft passes" rule.
var deferLivenessClosedStatuses = map[string]bool{
	"completed": true,
	"cancelled": true,
	"staged":    true,
}

// deferLivenessCard is the minimal slice of a coordinator task card the gate
// reads. Only Status, FilesInScope, and RoughScope affect the verdict; the rest
// is carried to name the offending card in the diagnostic.
type deferLivenessCard struct {
	TaskID       string   `json:"task_id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	FilesInScope []string `json:"files_in_scope"`
	RoughScope   []string `json:"rough_scope"`
	// path and isErrata are derived from the on-disk filename, not the JSON.
	path     string
	isErrata bool
}

// deferLivenessCardError records a defer/errata card that could not be read or
// parsed. A release-readiness gate must be FAIL-CLOSED here: an unreadable card
// cannot be classified open-or-closed, so the gate treats it as a hard failure
// (never SKIP) to avoid a lexically-earlier malformed card masking a later
// valid open contradiction.
type deferLivenessCardError struct {
	path string
	err  error
}

// migrationClaimNote is a migration note present on disk that carries
// released/about-to-release claims the gate reads. Released=true means the
// note's version is a published git tag (immutable shipped artifact); false
// means it is untagged (about-to-release / staging).
type migrationClaimNote struct {
	version  string
	path     string
	released bool
}

// deferLivenessContradiction is a single release-blocking finding: an open card
// that references a present released/about-to-release claim.
type deferLivenessContradiction struct {
	card          deferLivenessCard
	versions      []string // existing notes the card explicitly names (sorted)
	errataImplied bool     // errata card: contradicts a released claim by definition
}

func (cx deferLivenessContradiction) String() string {
	id := cx.card.TaskID
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(cx.card.path), ".json")
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
// release-readiness gate. It reads open coordinator task cards (Side A) and
// released/about-to-release migration notes (Side B), then FAILs when an open
// card references a claim present on Side B — the class of miss that left
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
// (classify notes) and the work-tree probe.
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

	// 2. Side A: open coordinator task cards (transport state). Fail-closed on
	//    directory-level errors and on any malformed card (see comment above).
	cards, cardsPresent, cardErrs, err := loadDeferLivenessCards(target)
	if err != nil {
		return checkResult{name: name, tier: tierFail, detail: "could not read coordinator tasks dir: " + err.Error()}
	}
	if !cardsPresent {
		return checkResult{name: name, tier: tierSkip, detail: "no .local/coordinator/tasks/ state (transport dir absent)"}
	}

	// 3. Side B: released/about-to-release migration notes on disk.
	notes, err := migrationClaimNotes(target)
	if err != nil {
		return checkResult{name: name, tier: tierFail, detail: "could not read migration notes: " + err.Error()}
	}
	if len(notes) == 0 {
		// No Side B: no shipped/about-to-ship claim to contradict. Even an open
		// errata card has no released claim to contradict in this tree (e.g. a
		// core-only or pre-migration-notes checkout). Clean no-op.
		return checkResult{name: name, tier: tierSkip, detail: "no templates/migrations/v*.md notes present"}
	}

	// 4. Cross-reference both sides.
	contradictions := findDeferLivenessContradictions(cards, notes)
	openCount := 0
	for _, c := range cards {
		if !deferLivenessCardIsClosed(c) {
			openCount++
		}
	}
	relCnt, unrelCnt := countReleasedNotes(notes)

	// Fail-closed: a malformed/unreadable card is itself a release blocker,
	// reported alongside any open-claim contradictions.
	if len(cardErrs) == 0 && len(contradictions) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d card(s) (%d open), %d note(s) (%d released, %d about-to-release); no release-blocking contradiction",
				len(cards), openCount, len(notes), relCnt, unrelCnt)}
	}

	var b strings.Builder
	if len(cardErrs) > 0 {
		fmt.Fprintf(&b, "%d unreadable/unparseable defer/errata card(s) (gate is fail-closed — fix or remove before release):",
			len(cardErrs))
		for _, ce := range cardErrs {
			fmt.Fprintf(&b, "\n  - %s: %v", filepath.Join(".local", "coordinator", "tasks", filepath.Base(ce.path)), ce.err)
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

// loadDeferLivenessCards reads the coordinator transport task cards that the
// gate consults: every defer-*.json and errata-*.json under
// <repoRoot>/.local/coordinator/tasks/. Returns present=false (no error) when
// the tasks dir is absent — that is a clean no-op, not a failure.
//
// Per-card read/JSON-parse failures do NOT abort the scan: they are collected
// in cardErrs and the scan continues, so a malformed card can never mask a
// later valid open contradiction. The caller treats a non-empty cardErrs as a
// FAIL (fail-closed). err is reserved for directory-level failures (e.g.
// permission denied on the tasks dir itself).
func loadDeferLivenessCards(repoRoot string) (cards []deferLivenessCard, present bool, cardErrs []deferLivenessCardError, err error) {
	tasksDir := filepath.Join(repoRoot, ".local", "coordinator", "tasks")
	entries, readErr := os.ReadDir(tasksDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, false, nil, nil
		}
		return nil, false, nil, readErr
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		isDefer := strings.HasPrefix(name, "defer-")
		isErrata := strings.HasPrefix(name, "errata-")
		if !isDefer && !isErrata {
			continue // not a defer/errata card (e.g. res-, eval-, review- cards)
		}
		path := filepath.Join(tasksDir, name)
		raw, e := os.ReadFile(path)
		if e != nil {
			cardErrs = append(cardErrs, deferLivenessCardError{path: path, err: fmt.Errorf("read card: %w", e)})
			continue
		}
		var c deferLivenessCard
		if e := json.Unmarshal(raw, &c); e != nil {
			cardErrs = append(cardErrs, deferLivenessCardError{path: path, err: fmt.Errorf("parse card: %w", e)})
			continue
		}
		c.path = path
		c.isErrata = isErrata
		cards = append(cards, c)
	}
	return cards, true, cardErrs, nil
}

// migrationClaimNotes enumerates Side B: the migration notes present on disk
// under <repoRoot>/templates/migrations/v<semver>.md (top-level only — the
// errata/ subdir holds staged erratum text, not released claims). A note is
// classified released=true when its version is a published bare-semver git tag
// (immutable shipped artifact); otherwise released=false (about-to-release /
// staging). Tag classification mirrors migration_release_test.go's glob +
// semverTagRe filter.
func migrationClaimNotes(repoRoot string) ([]migrationClaimNote, error) {
	dir := filepath.Join(repoRoot, "templates", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// Build the released-version set from git tags (bare vX.Y.Z semver only).
	released := map[string]bool{}
	if out, e := exec.Command("git", "-C", repoRoot, "tag", "-l", "v[0-9]*.[0-9]*.[0-9]*").Output(); e == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			tag := strings.TrimSpace(line)
			if tag != "" && semverVersionRe.MatchString(tag) {
				released[tag] = true
			}
		}
	}
	var notes []migrationClaimNote
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		ver := strings.TrimSuffix(name, ".md")
		if !semverVersionRe.MatchString(ver) {
			continue // not a release-version note (e.g. README, index)
		}
		notes = append(notes, migrationClaimNote{
			version:  ver,
			path:     filepath.Join(dir, name),
			released: released[ver],
		})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].version < notes[j].version })
	return notes, nil
}

// findDeferLivenessContradictions cross-references Side A (cards) against Side B
// (notes). A card is a contradiction iff it is OPEN and either (a) it is an
// errata card (released-claim contradiction by definition — Side B is already
// confirmed non-empty by the caller), or (b) its files_in_scope / rough_scope
// names a migration-note version whose note exists on disk.
func findDeferLivenessContradictions(cards []deferLivenessCard, notes []migrationClaimNote) []deferLivenessContradiction {
	exists := map[string]migrationClaimNote{}
	for _, n := range notes {
		exists[n.version] = n
	}
	var out []deferLivenessContradiction
	for _, c := range cards {
		if deferLivenessCardIsClosed(c) {
			continue
		}
		var hits []string
		for _, v := range cardReferencedNoteVersions(c) {
			if _, ok := exists[v]; ok {
				hits = append(hits, v)
			}
		}
		if len(hits) == 0 && !c.isErrata {
			// Open card that references no present migration note — not a
			// release-blocking contradiction (e.g. a code-level defer with no
			// released-claim surface).
			continue
		}
		sort.Strings(hits)
		out = append(out, deferLivenessContradiction{card: c, versions: hits, errataImplied: c.isErrata})
	}
	return out
}

// cardReferencedNoteVersions returns the de-duplicated list of bare migration
// versions (e.g. "v0.12.0") that the card mentions in files_in_scope or
// rough_scope, in first-seen order. Every migrationNotePathRe match in a scope
// string is extracted (not just the first), so a single scope entry naming two
// notes is fully evaluated.
func cardReferencedNoteVersions(c deferLivenessCard) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		for _, m := range migrationNotePathRe.FindAllString(s, -1) {
			v := strings.TrimSuffix(m, ".md")
			if v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	for _, f := range c.FilesInScope {
		add(f)
	}
	for _, r := range c.RoughScope {
		add(r)
	}
	return out
}

// deferLivenessCardIsClosed reports whether the card's status is in the closed
// set (case-insensitive, whitespace-trimmed).
func deferLivenessCardIsClosed(c deferLivenessCard) bool {
	return deferLivenessClosedStatuses[strings.ToLower(strings.TrimSpace(c.Status))]
}

// countReleasedNotes splits a note set into released vs about-to-release counts.
func countReleasedNotes(notes []migrationClaimNote) (released, aboutToRelease int) {
	for _, n := range notes {
		if n.released {
			released++
		} else {
			aboutToRelease++
		}
	}
	return
}
