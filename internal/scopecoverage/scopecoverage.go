// Package scopecoverage implements the F4-A declared-scope coverage validator
// from the decision memo
// `researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md`.
//
// F4-A is the property that every item in a review's DECLARED scope receives a
// terminal coverage disposition before any aggregate success/approval is claimed.
// It is one of three UNION siblings in the F4 assurance/integrity-stewardship
// family:
//
//   - F4-A — declared-scope coverage        (this package)
//   - F4-B1 — canonical verifier execution  (release G0; out of scope here)
//   - F4-B2 — transition-state identity     (release G0b; out of scope here)
//
// # What this package IS and is NOT
//
// This is a STRUCTURAL coverage validator. Validate proves ONLY that every
// declared item received a terminal disposition (examined or excluded by
// contract). It does NOT — and cannot — prove review quality, attention, or
// semantic understanding. A Complete report means "every declared item was
// accounted for," never "every declared item was meaningfully examined." Callers
// MUST state this structural-only claim wherever they consume the result; the
// CoverageReport type intentionally carries no field that could be read as a
// semantic/quality verdict.
//
// # Authority
//
// This package is a pure library. It INFORMS. It applies NO transition and is
// NOT wired to any blocking gate. Per the decision memo, a deterministic
// declared-scope equality validator may become a hard gate only after canonical
// comparable representations exist (defined here), item identity and exclusion
// semantics are deterministic (defined here), the validator is attached to the
// actual approval transition (NOT done here), and fixtures prove partial/
// truncated coverage refuses approval (provided in scopecoverage_test.go).
// Until that attachment lands, the result is non-blocking / diagnostic.
//
// # Canonical representations
//
// The canonical declared-scope identity is a repo-relative path, optionally
// qualified by a concern string: "path/to/file.go" or "path/to/file.go#exports".
// This answers Open Question 1 of the decision memo: the canonical identity is
// exact path plus optional concern. Identity normalization is deterministic
// (path.Clean, forward slashes, whitespace-trimmed, case-sensitive) so two
// producers that mean the same item produce the same ID.
package scopecoverage

import (
	"path"
	"sort"
	"strings"
)

// DispositionStatus is the terminal/non-terminal state of a declared item's
// coverage. It mirrors the four-state vocabulary in the decision memo
// (§Mechanism → F4-A prevention): examined, not examined, excluded by contract,
// blocked by missing evidence.
type DispositionStatus string

const (
	// StatusExamined is a TERMINAL disposition: the item was reviewed.
	StatusExamined DispositionStatus = "examined"
	// StatusExcluded is a TERMINAL disposition: the item was intentionally
	// excluded by contract; Reason MUST explain why. An exclusion is a
	// reasoned decision to not examine, not an omission.
	StatusExcluded DispositionStatus = "excluded"
	// StatusNotExamined is a NON-TERMINAL disposition: the item is still
	// pending and has not yet been reached. Coverage is incomplete.
	StatusNotExamined DispositionStatus = "not_examined"
	// StatusBlocked is a NON-TERMINAL disposition: the item could not be
	// completed because evidence was missing. Coverage is incomplete.
	StatusBlocked DispositionStatus = "blocked"
)

// IsTerminal reports whether a disposition STATE is terminal (examined or
// excluded by contract). Non-terminal states (not_examined, blocked) mean
// coverage is incomplete and block any aggregate complete/approval claim. An
// unknown status is non-terminal (fail-safe).
//
// This is a STATUS-LEVEL predicate only: StatusExcluded is a terminal state.
// The Reason requirement for a valid exclusion (StatusExcluded WITH a
// non-blank Reason) is enforced at the DISPOSITION level by Validate via
// dispRank — a reason-less exclusion resolves to non-terminal there. Do not
// consult IsTerminal alone to decide aggregate completeness; use
// CoverageReport.Complete / HasGaps, which account for the reason requirement.
func (s DispositionStatus) IsTerminal() bool {
	return s == StatusExamined || s == StatusExcluded
}

// DeclaredScopeItem is one item a review promises to account for. Path is a
// repo-relative path; Concern optionally qualifies a sub-region of the file
// (e.g. "exports", "handler"). When Concern is empty, the item is the whole
// file. ID is the normalized canonical identity used for comparison.
type DeclaredScopeItem struct {
	Path    string
	Concern string
}

// ID returns the normalized canonical identity: "path" or "path#concern".
// Path is path.Cleaned, given forward slashes, and whitespace-trimmed. The
// separator between path and concern is "#" so a concern is never confused
// with a path segment. Identity is case-sensitive (matching case-sensitive
// filesystems); callers normalizing for case-insensitive systems must do so
// before constructing the item.
func (d DeclaredScopeItem) ID() string {
	p := normalizePath(d.Path)
	if strings.TrimSpace(d.Concern) == "" {
		return p
	}
	return p + "#" + strings.TrimSpace(d.Concern)
}

// CoverageDisposition is the coverage record for one item. ItemID MUST match a
// DeclaredScopeItem.ID for a recognized item; a disposition whose ItemID does
// not match any declared item is reported as Extra (out-of-scope). Reason is
// free text explaining an exclusion or block. Reason is REQUIRED for
// StatusExcluded and ENFORCED: a StatusExcluded with a blank (empty or
// whitespace-only) Reason is treated as a non-terminal "unexplained exclusion"
// (reported in UnexplainedExclusions and NonTerminal) and cannot bless an item
// as covered — "excluded by contract" implies a contract/reason.
type CoverageDisposition struct {
	ItemID string
	Status DispositionStatus
	Reason string
}

// CoverageReport is the structural coverage result. Complete is true ONLY when
// every declared item received exactly one VALID terminal disposition, no
// declared ID is ambiguous, no disposition is duplicated, and no disposition
// targets an unrecognized (out-of-scope) item. Complete means structural
// coverage; it is NOT a semantic or quality verdict.
//
// A VALID terminal disposition is either StatusExamined, or StatusExcluded WITH
// a non-blank Reason. An exclusion is "excluded BY CONTRACT" — it implies a
// contract/reason. A StatusExcluded with a blank Reason is an omission in
// disguise, not a valid terminal disposition: it is treated as non-terminal
// (reported in UnexplainedExclusions and NonTerminal) and cannot bless an item
// as covered.
type CoverageReport struct {
	// Complete is true iff Missing, NonTerminal, Extra, AmbiguousDeclared,
	// DuplicateDispositions, and UnexplainedExclusions are ALL empty. An Extra
	// (out-of-scope) disposition blocks Complete because it means the coverage
	// set and the declared set do not correspond — representations are not
	// cleanly comparable, so a clean complete approval cannot be claimed. An
	// UnexplainedExclusion blocks Complete because an unexplained exclusion is
	// not a valid terminal disposition. FailFastTerminated is reported
	// separately; it does not independently force incomplete (a fail-fast
	// review that nonetheless accounted for every declared item is structurally
	// complete), but fail-fast combined with any gap is, by construction, not
	// complete.
	Complete bool

	// Missing lists declared IDs that received NO disposition entry at all.
	Missing []string

	// NonTerminal lists declared IDs whose resolved disposition is non-terminal
	// (not_examined, blocked, or an unexplained exclusion). Coverage for these
	// items is incomplete.
	NonTerminal []string

	// Extra lists disposition ItemIDs that do not match any declared item
	// (reported items outside the declared scope / mismatch).
	Extra []string

	// AmbiguousDeclared lists IDs that appear more than once in the declared
	// scope (two items normalized to the same identity). Identity is
	// ambiguous and the comparison cannot be trusted while this is non-empty.
	AmbiguousDeclared []string

	// DuplicateDispositions lists IDs that have more than one disposition
	// entry. When an ID has duplicates, its resolved status is the WORST
	// (least terminal) among them and the ID is flagged here so a caller can
	// detect the inconsistency rather than silently picking one.
	DuplicateDispositions []string

	// UnexplainedExclusions lists declared IDs whose resolved disposition is a
	// StatusExcluded with a blank Reason. Such an item is also in NonTerminal
	// (an unexplained exclusion is not a valid terminal disposition). This
	// field exists separately so a caller can distinguish "blocked / pending"
	// from "claimed covered via an invalid exclusion" — different remedies.
	UnexplainedExclusions []string

	// FailFastTerminated echoes the input flag: the review process was cut
	// short (e.g. a tiered cascade stopped at the first block/split). This is
	// informational; completeness is derived from the disposition gaps above.
	// A fail-fast review that still has gaps cannot be Complete by
	// construction.
	FailFastTerminated bool
}

// HasGaps reports whether any structural defect was found. It is the negation
// of Complete minus the informational FailFastTerminated flag, and is the
// single predicate a future approval gate would consult. Extra (out-of-scope
// dispositions) and UnexplainedExclusions (excluded-without-reason) count as
// gaps: a coverage set that does not correspond to the declared set, or that
// claims coverage via an invalid exclusion, is not a clean complete approval.
func (r CoverageReport) HasGaps() bool {
	return len(r.Missing) > 0 ||
		len(r.NonTerminal) > 0 ||
		len(r.Extra) > 0 ||
		len(r.AmbiguousDeclared) > 0 ||
		len(r.DuplicateDispositions) > 0 ||
		len(r.UnexplainedExclusions) > 0
}

// Validate compares a declared scope against the coverage dispositions actually
// produced and returns a structural CoverageReport. It is deterministic and
// pure: identical inputs always yield identical outputs.
//
// failFastTerminated indicates the producing review was truncated (e.g. a
// tiered cascade with fail_fast stopped at the first block/split, so later
// tiers never ran). It is echoed into the report and informs a caller that
// coverage evidence was gathered under truncation; it does not change Complete
// on its own.
func Validate(declared []DeclaredScopeItem, dispositions []CoverageDisposition, failFastTerminated bool) CoverageReport {
	// Declared-side accounting.
	declaredIDs := make([]string, 0, len(declared))
	declaredSet := make(map[string]int, len(declared)) // id -> count
	for _, d := range declared {
		id := d.ID()
		declaredIDs = append(declaredIDs, id)
		declaredSet[id]++
	}

	// Ambiguity: declared IDs appearing more than once.
	var ambiguous []string
	for id, n := range declaredSet {
		if n > 1 {
			ambiguous = append(ambiguous, id)
		}
	}

	// Disposition-side accounting.
	dispByItem := make(map[string][]CoverageDisposition, len(dispositions))
	for _, dp := range dispositions {
		id := strings.TrimSpace(dp.ItemID)
		dispByItem[id] = append(dispByItem[id], dp)
	}

	// Duplicates: IDs with more than one disposition entry.
	var duplicates []string
	for id, dps := range dispByItem {
		if len(dps) > 1 {
			duplicates = append(duplicates, id)
		}
	}

	// Extra: disposition IDs not present in the declared scope. A declared ID
	// with count >= 1 is in scope even if ambiguous; ambiguity is reported
	// separately and does not turn a matching disposition into an extra.
	var extra []string
	for id := range dispByItem {
		if _, ok := declaredSet[id]; !ok {
			extra = append(extra, id)
		}
	}

	// Missing (no disposition at all) and NonTerminal (resolved disposition
	// is non-terminal). For an ID with duplicate dispositions, the resolved
	// rank is the WORST (lowest) among the entries; the duplicate is also
	// flagged separately above so a caller can detect the inconsistency rather
	// than silently trusting the worst-case resolution. An unexplained
	// exclusion (StatusExcluded with blank Reason) resolves to a non-terminal
	// rank and is reported in BOTH NonTerminal and UnexplainedExclusions.
	var missing, nonTerminal, unexplained []string
	for _, id := range declaredIDs {
		// Each occurrence of an ambiguous declared ID is evaluated; record the
		// ID once per distinct gap class (deduplicated after the loop).
		dps, ok := dispByItem[id]
		if !ok || len(dps) == 0 {
			missing = append(missing, id)
			continue
		}
		wr := worstRank(dps)
		if wr < minTerminalRank {
			nonTerminal = append(nonTerminal, id)
			if wr == rankUnexplainedExclusion {
				unexplained = append(unexplained, id)
			}
		}
	}
	missing = dedupSorted(missing)
	nonTerminal = dedupSorted(nonTerminal)
	unexplained = dedupSorted(unexplained)

	rep := CoverageReport{
		Missing:               missing,
		NonTerminal:           nonTerminal,
		Extra:                 dedupSorted(extra),
		AmbiguousDeclared:     dedupSorted(ambiguous),
		DuplicateDispositions: dedupSorted(duplicates),
		UnexplainedExclusions: unexplained,
		FailFastTerminated:    failFastTerminated,
	}
	rep.Complete = !rep.HasGaps()
	return rep
}

// Rank thresholds for resolved dispositions. Higher rank = more terminal.
const (
	rankUnknown              = 0 // unknown status -> non-terminal, worst (fail-safe)
	rankNotExamined          = 1
	rankUnexplainedExclusion = 2 // StatusExcluded with blank Reason -> non-terminal
	rankBlocked              = 3
	rankExcluded             = 4 // StatusExcluded WITH a non-blank Reason -> terminal
	rankExamined             = 5 // StatusExamined -> terminal

	// minTerminalRank is the lowest rank that counts as a valid terminal
	// disposition (examined, or excluded-with-reason).
	minTerminalRank = rankExcluded
)

// dispRank returns the terminal rank of a single disposition. An excluded
// disposition WITHOUT a non-blank Reason is demoted to rankUnexplainedExclusion
// (non-terminal): "excluded by contract" implies a contract/reason, so a
// reason-less exclusion is an omission in disguise and cannot bless an item as
// covered. Unknown statuses rank lowest (non-terminal, fail-safe).
func dispRank(dp CoverageDisposition) int {
	switch dp.Status {
	case StatusExamined:
		return rankExamined
	case StatusExcluded:
		if strings.TrimSpace(dp.Reason) == "" {
			return rankUnexplainedExclusion
		}
		return rankExcluded
	case StatusBlocked:
		return rankBlocked
	case StatusNotExamined:
		return rankNotExamined
	default:
		return rankUnknown
	}
}

// worstRank returns the WORST (lowest) rank across dps. If any entry is
// non-terminal, the resolved rank is non-terminal (a single blocked or
// unexplained-exclusion entry makes the item's coverage incomplete even if
// another entry says examined). Returns rankNotExamined for empty input
// (no dispositions -> non-terminal).
func worstRank(dps []CoverageDisposition) int {
	if len(dps) == 0 {
		return rankNotExamined
	}
	worst := dispRank(dps[0])
	for _, dp := range dps[1:] {
		if r := dispRank(dp); r < worst {
			worst = r
		}
	}
	return worst
}

// normalizePath cleans and canonicalizes a repo-relative path for identity
// comparison: trim surrounding whitespace, convert backslashes to forward
// slashes, and path.Clean. A "." or "" result is returned as-is (an empty path
// is a malformed declaration a caller should avoid, but the validator does not
// panic on it).
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Clean(p)
}

// dedupSorted returns a sorted, deduplicated copy of in. Deterministic output
// makes reports stable across runs and easy to assert on in fixtures.
func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
