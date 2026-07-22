// Package claims implements the §4.1 claim/verifier closure kernel over the
// typed-memory substrate (internal/memory/record + internal/memory/store).
//
// A "claim" is a typed assertion the release-readiness gate (and, in later
// slices, the §4.2 premise-recheck) must consult before acting. This package is
// the NON-AUTHORITATIVE source of such claims: it READS on-disk state and
// projects it into typed records; it never writes canon, never mutates
// commit/release/permission state, and never itself blocks a release. Only the
// gate (internal/cli/release_gate.go) acts / FAILs. This is the authority line
// from the claim-verifier closure-kernel memo: "coordinator state INFORMS;
// safety-layer gates ACT".
//
// Provenance split (memo S1/S2): each claim's SourceRef names whether it came
// from CANON (templates/migrations/*.md — git-tracked, the S2 invariant side)
// or from TRANSPORT (.local/coordinator/tasks/*.json — losable, the S1
// disposition side). The kernel does not persist a parallel registry file:
// store.Read returns an empty slice with a nil error on a missing file, so a
// persisted registry would make the gate PASS when lost (fail-open). Instead
// Derive re-derives the typed projection from the SAME on-disk sources at
// gate-read time, preserving the gate's fail-closed discipline by construction.
package claims

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/record"
)

// ClaimKind classifies a claim by which side of the release gate it feeds.
type ClaimKind string

const (
	// KindReleasedNote is a migration-note claim present on Side B
	// (templates/migrations/v<semver>.md). Released=true means the version is a
	// published bare-semver git tag (immutable shipped artifact); false means
	// about-to-release / staging.
	KindReleasedNote ClaimKind = "released_note"
	// KindDeferDisposition is a coordinator task-card disposition present on
	// Side A (.local/coordinator/tasks/{defer,errata}-*.json — transport).
	KindDeferDisposition ClaimKind = "defer_disposition"
)

// Claim is the S1 claim/disposition schema from the closure-kernel memo, plus
// §4.3 gate-extension fields the release gate reasons about inline. The S1 core
// (ClaimID..ValidUntil) is the stable contract; the extensions are how the gate
// classifies the claim. A Claim is carried as the JSON body of a record.Record
// so the substrate stores a typed, addressable atom per claim.
type Claim struct {
	// --- S1 core schema ---
	ClaimID        string `json:"claim_id"`
	Statement      string `json:"statement"`
	SourceRef      string `json:"source_ref"`       // canon vs transport source path (provenance)
	VerifierRef    string `json:"verifier_ref"`     // what checks this claim
	Disposition    string `json:"disposition"`      // e.g. released / about-to-release / open / closed
	LastVerifiedAt string `json:"last_verified_at"` // RFC3339
	ValidUntil     string `json:"valid_until,omitempty"`

	// --- §4.3 gate extensions ---
	Kind               ClaimKind `json:"kind"`
	NoteVersion        string    `json:"note_version,omitempty"`
	Released           bool      `json:"released,omitempty"`
	IsErrata           bool      `json:"is_errata,omitempty"`
	FilesInScope       []string  `json:"files_in_scope,omitempty"`
	RoughScope         []string  `json:"rough_scope,omitempty"`
	ReferencedVersions []string  `json:"referenced_versions,omitempty"`
}

// NoteClaim is a migration note present on disk (Side B). Version is bare
// semver (e.g. "v0.12.0"). Released means the version is a published bare-semver
// git tag (immutable shipped artifact); otherwise it is about-to-release.
type NoteClaim struct {
	Version  string
	Path     string
	Released bool
}

// DeferCard is the minimal slice of a coordinator task card the gate reads
// (Side A). Only Status, FilesInScope, and RoughScope affect the verdict; the
// rest is carried to name the offending card. Path, IsErrata, and
// ReferencedVersions are DERIVED from the on-disk filename and the scope strings
// (not present in the card JSON), so they are excluded from JSON decoding.
type DeferCard struct {
	TaskID       string   `json:"task_id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	FilesInScope []string `json:"files_in_scope"`
	RoughScope   []string `json:"rough_scope"`
	// Derived (not decoded). json:"-" so a stray same-named key never touches them.
	Path               string   `json:"-"`
	IsErrata           bool     `json:"-"`
	ReferencedVersions []string `json:"-"`
}

// CardError records a defer/errata card that could not be read or parsed. The
// gate treats a non-empty CardErrors set as a hard FAIL (fail-closed): an
// unparseable card cannot be classified open-or-closed, and SKIPping it would
// let a lexically-earlier malformed card mask a later valid open contradiction.
type CardError struct {
	Path string
	Err  error
}

// Registry is the in-memory typed projection of every claim the gate consults,
// derived synchronously from on-disk state by Derive. It is the SINGLE source
// the gate consumes, closing dual-derivation (one kernel, one consumer).
// Records is the substrate-level (record.Record) view carrying the S1 claim
// schema in each body; the gate does not need it but it proves the kernel lives
// over internal/memory and feeds future §4.2 consumers.
type Registry struct {
	Notes        []NoteClaim
	Cards        []DeferCard
	CardErrors   []CardError
	Records      []record.Record
	TasksPresent bool
	NotesPresent bool
}

// semverVersionRe matches a bare release version token vX.Y.Z (no pre-release
// suffix). It MIRRORS semverTagRe in internal/cli/migration_release_test.go —
// that variable lives in a _test.go file and is therefore not visible to
// non-test code, so this file carries its own copy. Keep the two in sync.
var semverVersionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// migrationNotePathRe matches a migration-note path or filename embedded in a
// free-form scope string, e.g. "templates/migrations/v0.12.0.md" or the bare
// "v0.12.0.md". extractReferencedVersions pulls ALL matches from a string (not
// just the first) so a scope entry naming two notes is fully evaluated.
var migrationNotePathRe = regexp.MustCompile(`v\d+\.\d+\.\d+\.md`)

// closedStatuses are the task-card statuses no longer an OPEN contradiction
// against a released/about-to-release claim: completed (landed), cancelled
// (dismissed), staged (correction queued for next release).
var closedStatuses = map[string]bool{
	"completed": true,
	"cancelled": true,
	"staged":    true,
}

// CardIsClosed reports whether the card's status is in the closed set
// (case-insensitive, whitespace-trimmed). Exported so the gate can classify
// cards without re-importing the closed-set definition.
func CardIsClosed(c DeferCard) bool {
	return closedStatuses[strings.ToLower(strings.TrimSpace(c.Status))]
}

// Derive is the §4.1 closure kernel: it reads BOTH on-disk sides the gate needs
// and returns a single typed projection. It is STRICTLY NON-AUTHORITATIVE —
// read/inform side-effects only (no writes, no persistence, no gating).
//
// Error semantics mirror the gate's pre-refactor contract so fail-closed
// behavior is preserved exactly:
//   - A directory-level I/O failure (other than IsNotExist) on EITHER side
//     returns a non-nil error → the caller (gate) FAILs.
//   - A missing tasks dir → TasksPresent=false, no error → caller SKIPs Side A.
//   - A missing/empty migrations dir → NotesPresent=false, no error → caller
//     SKIPs Side B.
//
// Per-card read/parse failures are NOT fatal here: they are collected in
// CardErrors and the scan continues (so a malformed card can never mask a later
// valid open contradiction). The caller treats a non-empty CardErrors as FAIL.
func Derive(repoRoot string) (Registry, error) {
	var reg Registry
	now := time.Now().UTC()

	// Side A: coordinator transport task cards.
	cards, tasksPresent, cardErrs, err := loadDeferCards(repoRoot)
	if err != nil {
		return reg, fmt.Errorf("read coordinator tasks dir: %w", err)
	}
	reg.TasksPresent = tasksPresent
	reg.Cards = cards
	reg.CardErrors = cardErrs

	// Side B: released/about-to-release migration notes (canon).
	notes, notesPresent, err := migrationNotes(repoRoot)
	if err != nil {
		return reg, fmt.Errorf("read migration notes: %w", err)
	}
	reg.Notes = notes
	reg.NotesPresent = notesPresent

	// Substrate-level typed projection (S1 schema carried in each Record body).
	// NOT persisted here — see package doc for the fail-open rationale. Building
	// it proves the kernel exists over internal/memory and gives future §4.2
	// consumers a typed view without re-deriving.
	reg.Records = buildRecords(reg, now)
	return reg, nil
}

// loadDeferCards reads Side A: every defer-*.json and errata-*.json under
// <repoRoot>/.local/coordinator/tasks/. present=false (no error) when the tasks
// dir is absent — a clean no-op, not a failure.
func loadDeferCards(repoRoot string) (cards []DeferCard, present bool, cardErrs []CardError, err error) {
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
			cardErrs = append(cardErrs, CardError{Path: path, Err: fmt.Errorf("read card: %w", e)})
			continue
		}
		var c DeferCard
		if e := json.Unmarshal(raw, &c); e != nil {
			cardErrs = append(cardErrs, CardError{Path: path, Err: fmt.Errorf("parse card: %w", e)})
			continue
		}
		c.Path = path
		c.IsErrata = isErrata
		c.ReferencedVersions = extractReferencedVersions(c)
		cards = append(cards, c)
	}
	return cards, true, cardErrs, nil
}

// migrationNotes reads Side B: migration notes under
// <repoRoot>/templates/migrations/v<semver>.md (top-level only — the errata/
// subdir holds staged erratum text, not released claims). present=false (no
// error) when the dir is absent or yields no version notes. Tag classification
// silently ignores git errors (treats all notes as about-to-release in that
// case), mirroring the pre-refactor behavior.
func migrationNotes(repoRoot string) (notes []NoteClaim, present bool, err error) {
	dir := filepath.Join(repoRoot, "templates", "migrations")
	entries, e := os.ReadDir(dir)
	if e != nil {
		if os.IsNotExist(e) {
			return nil, false, nil
		}
		return nil, false, e
	}
	// Build the released-version set from git tags (bare vX.Y.Z semver only).
	released := map[string]bool{}
	if out, ge := exec.Command("git", "-C", repoRoot, "tag", "-l", "v[0-9]*.[0-9]*.[0-9]*").Output(); ge == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			tag := strings.TrimSpace(line)
			if tag != "" && semverVersionRe.MatchString(tag) {
				released[tag] = true
			}
		}
	}
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
		notes = append(notes, NoteClaim{
			Version:  ver,
			Path:     filepath.Join(dir, name),
			Released: released[ver],
		})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Version < notes[j].Version })
	return notes, len(notes) > 0, nil
}

// extractReferencedVersions returns the de-duplicated list of bare migration
// versions (e.g. "v0.12.0") the card mentions in files_in_scope or rough_scope,
// in first-seen order. Every migrationNotePathRe match in a scope string is
// extracted (not just the first), so a single scope entry naming two notes is
// fully evaluated.
func extractReferencedVersions(c DeferCard) []string {
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

// buildRecords projects the typed Registry into substrate-level record.Record
// atoms, each carrying the S1 Claim schema in its body. IDs are stable and
// deterministic across re-derivations (one per claim) so the projection is
// idempotent.
func buildRecords(reg Registry, now time.Time) []record.Record {
	var out []record.Record
	for _, n := range reg.Notes {
		c := Claim{
			ClaimID:        "released_note:" + n.Version,
			Statement:      fmt.Sprintf("migration note %s is present on disk", n.Version),
			SourceRef:      n.Path,
			VerifierRef:    "release_gate:defer_liveness (side B)",
			Disposition:    noteDisposition(n.Released),
			LastVerifiedAt: now.Format(time.RFC3339),
			Kind:           KindReleasedNote,
			NoteVersion:    n.Version,
			Released:       n.Released,
		}
		out = append(out, claimRecord(c, now))
	}
	for _, card := range reg.Cards {
		c := Claim{
			ClaimID:            "defer_disposition:" + card.TaskID,
			Statement:          cardDispositionStatement(card),
			SourceRef:          card.Path,
			VerifierRef:        "release_gate:defer_liveness (side A)",
			Disposition:        cardDisposition(card),
			LastVerifiedAt:     now.Format(time.RFC3339),
			Kind:               KindDeferDisposition,
			IsErrata:           card.IsErrata,
			FilesInScope:       card.FilesInScope,
			RoughScope:         card.RoughScope,
			ReferencedVersions: card.ReferencedVersions,
		}
		out = append(out, claimRecord(c, now))
	}
	return out
}

// claimRecord wraps a Claim as a validating record.Record atom.
func claimRecord(c Claim, now time.Time) record.Record {
	body, err := json.Marshal(c)
	if err != nil {
		// Should not happen for this struct; fall back so the record still
		// validates (Body is required) rather than dropping the claim silently.
		body = []byte(`{"claim_id":"` + c.ClaimID + `","statement":"marshal error"}`)
	}
	scene := string(c.Kind)
	src := c.SourceRef
	return record.Record{
		ID:        c.ClaimID,
		Type:      record.TypeInstruction,
		Priority:  record.PriorityNormal,
		Scope:     record.ScopeWorkstream,
		Scene:     &scene,
		SourceRef: &src,
		Body:      string(body),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func noteDisposition(released bool) string {
	if released {
		return "released"
	}
	return "about-to-release"
}

func cardDisposition(c DeferCard) string {
	if CardIsClosed(c) {
		return "closed"
	}
	return "open"
}

func cardDispositionStatement(c DeferCard) string {
	kind := "defer"
	if c.IsErrata {
		kind = "errata"
	}
	return fmt.Sprintf("%s card %s is %s", kind, c.TaskID, cardDisposition(c))
}
