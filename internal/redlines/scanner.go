package redlines

import (
	"bytes"
	"sort"
	"strings"
)

// ScanUnit is one unit of content handed to the pure matching engine. In v1 a
// unit is always a whole file (Unit="" or "file"); the engine scans the bytes
// it is given. The caller (the `redlines scan` command / the commit gate)
// slices tree-entry content into ScanUnits at file granularity: the scan
// command produces one ScanUnit per blob. Diff-level units (Unit="diff") are a
// documented future extension and are NOT yet implemented — the registry rejects
// unit: diff at load time (fail-closed) so an operator cannot silently get
// weaker file-level protection. The engine itself treats every ScanUnit
// uniformly and matches every binding subject against it.
//
// Path is repo-relative file metadata and is safe to echo in a Finding. Content
// is the unit's raw bytes (UTF-8 text expected; binary content is detected and
// skipped). The engine performs NO filesystem, git, network, or model access —
// it is a pure function of (subjects, units, scan context).
type ScanUnit struct {
	// Path is the repo-relative path of the file this unit came from. It is
	// safe metadata (a path the operator already controls) and is the ONLY
	// field besides the opaque subject id that appears in a Finding.
	Path string
	// Content is the unit's raw bytes.
	Content []byte
}

// ScanContext carries the repo-identity context the engine needs to evaluate
// the ambient predicate. It is NOT sensitive: a repo absolute path and its
// normalized git remotes. RepoRemotes is the de-duplicated normalized set from
// RepoRemotes(); empty is valid (a remoteless scratch repo is matched by path).
type ScanContext struct {
	// RepoPath is the absolute filesystem path of the repo being scanned. Used
	// only for the ambient_repos path-glob predicate.
	RepoPath string
	// Remotes are the normalized git remotes of the repo being scanned. Used
	// only for the ambient_repos remote-glob predicate.
	Remotes []string
}

// Reason codes carried by a Finding. They are generic and safe: they never
// include the matched term.
const (
	// ReasonScrubTerm is emitted when a scrub-project subject's configured label
	// (or path-fragment label) is found in a unit's content or path.
	ReasonScrubTerm = "scrub-term"
	// ReasonRelationCoOccurrence is emitted when a forbidden-relation subject's
	// SideA term AND SideB term co-occur within the SAME scan unit (non-ambient
	// case). A SideA hit in one unit and a SideB hit in a different unit is a
	// documented NON-hit (cross-unit inference is unsupported in v1).
	ReasonRelationCoOccurrence = "relation-co-occurrence"
	// ReasonRelationAmbientSideB is emitted when a forbidden-relation subject is
	// AMBIENT for the scanned repo (repo identity implies SideA) and a SideB
	// term appears alone in a unit. SideA is implied, never matched.
	ReasonRelationAmbientSideB = "relation-ambient-side-b"
)

// Finding is an opaque, paste-safe scan result. It is the ONLY result type the
// engine produces and it is structurally incapable of carrying a real term:
//
//   - SubjectID is the opaque subj-* id (the only token safe to echo anywhere).
//   - Reason is one of the generic reason codes above.
//   - Path is repo-relative file metadata (safe).
//
// There is DELIBERATELY NO matched-term field. The real term the engine matched
// is never echoed in any Finding field, in any error, or in any stringer. This
// is the load-bearing invariant that makes scan output safe to paste into
// issues, PRs, and commit messages.
type Finding struct {
	// SubjectID is the opaque subject identifier (subj-...).
	SubjectID string
	// Reason is one of ReasonScrubTerm / ReasonRelationCoOccurrence /
	// ReasonRelationAmbientSideB.
	Reason string
	// Path is the repo-relative file path of the unit that produced the finding.
	Path string
}

// MaxUnitSize bounds the cost of scanning a single unit. Units whose Content
// length exceeds this cap are SKIPPED (not matched). This is a documented
// limitation, not a conservative choice: a file larger than 1 MiB is likely
// generated/minified/binary-shaped, and lexical substring matching over it
// would be both expensive and noisy. The honesty contract covers this — a
// passing scan is not proof that no sensitive relation can be inferred. If an
// operator needs terms in very large files detected, they should structure the
// registry so the relevant content lands in smaller units.
const MaxUnitSize = 1 << 20 // 1 MiB

// HonestyContract is the verbatim v1 detection-honesty statement. It is placed
// as a comment here and is printed by the scan and guidance surfaces (and
// referenced by doctor) so no consumer of a "clean" scan mistakes it for proof
// of safety.
//
// This scan is lexical and best-effort. It detects configured terms and aliases
// within the defined scan unit, including configured ambient-side degeneration.
// It does not detect paraphrases, translations, undeclared aliases, semantic
// equivalence, or relations that require inference across scan units. A passing
// scan is not proof that no sensitive relation can be inferred.
const HonestyContract = `This scan is lexical and best-effort. It detects configured terms and aliases within the defined scan unit, including configured ambient-side degeneration. It does not detect paraphrases, translations, undeclared aliases, semantic equivalence, or relations that require inference across scan units. A passing scan is not proof that no sensitive relation can be inferred.`

// Scan is the pure lexical matching engine. It consumes already-loaded binding
// subjects and already-sliced scan units, and returns the deterministic,
// paste-safe set of findings. It performs NO I/O.
//
// The caller is responsible for:
//   - Pre-filtering subjects to those that Binds the scanned repo (the engine
//     trusts this; it does not re-check binding).
//   - Slicing tree-entry content into ScanUnits. In v1 the scan command slices
//     at file-level only, producing one ScanUnit per blob; diff-level units are
//     a future extension not yet implemented (and rejected at registry load).
//
// The engine owns:
//   - scrub-project lexical detection (labels as case-insensitive substrings in
//     content; path-fragment labels also matched against the unit path).
//   - forbidden-relation co-occurrence within a single unit (non-ambient).
//   - ambient-side degeneration (any SideB term alone, when the subject is
//     ambient for the ScanContext's repo).
//   - binary skip (NUL byte), oversized skip (> MaxUnitSize), dedup, sort.
//
// Normalization policy: matching is case-insensitive via strings.ToLower on both
// the content/path and the configured term, then strings.Contains. strings.ToLower
// is Unicode-aware lowercase (handles ASCII and most Unicode); full Unicode
// case-folding (e.g. cases.Fold, which would reconcile German ß/ss or Turkish
// dotted-i) is a known future refinement and is NOT applied in v1. This is
// documented because registry terms are matched as SUBSTRINGS, not whole
// tokens: a multi-word phrase matches as a contiguous run, and a short term may
// match inside a longer word (the conservative choice for a detector — better a
// false positive a human reviews than a missed sensitive relation).
//
// Findings are deduplicated to one per (SubjectID, Path, Reason) and sorted by
// Path then SubjectID for deterministic output.
func Scan(ctx ScanContext, subjects []Subject, units []ScanUnit) []Finding {
	// Dedup key. A single unit can match a subject's term set in multiple ways
	// (e.g. two labels both hit, or a label hits both content and path); these
	// collapse to ONE finding because the Finding carries no term — duplicates
	// would be byte-identical and noise.
	seen := make(map[findingKey]struct{})
	var findings []Finding

	for _, u := range units {
		content := u.Content

		// Binary detection: a NUL byte is the conventional heuristic (mirrors
		// git's own binary detection). Binary content is not meaningfully
		// lexical, so the unit is skipped entirely — no finding for any subject.
		if len(content) > 0 && bytes.IndexByte(content, 0) >= 0 {
			continue
		}
		// Oversized: bound cost. Skipped, not matched (documented limitation).
		if len(content) > MaxUnitSize {
			continue
		}

		// Normalize once per unit. strings.ToLower is Unicode-aware lowercase.
		// An empty unit still has its path checked for path-fragment labels, so
		// we always compute lowerPath and lowerContent (the latter may be "").
		lowerPath := strings.ToLower(u.Path)
		lowerContent := strings.ToLower(string(content))

		for _, s := range subjects {
			switch s.Kind {
			case KindScrubProject:
				if scrubUnitMatches(lowerContent, lowerPath, s.Labels) {
					findings = addFinding(findings, seen, s.ID, ReasonScrubTerm, u.Path)
				}
			case KindForbiddenRelation:
				if s.IsAmbient(ctx.RepoPath, ctx.Remotes) {
					// Ambient: repo identity implies SideA, so any SideB term
					// alone in this unit is a violation. SideA is never matched.
					if anyTermInContent(lowerContent, s.SideB) {
						findings = addFinding(findings, seen, s.ID, ReasonRelationAmbientSideB, u.Path)
					}
				} else {
					// Non-ambient: SideA AND SideB must co-occur within THIS
					// unit. Cross-unit co-occurrence is a documented non-hit.
					if anyTermInContent(lowerContent, s.SideA) && anyTermInContent(lowerContent, s.SideB) {
						findings = addFinding(findings, seen, s.ID, ReasonRelationCoOccurrence, u.Path)
					}
				}
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].SubjectID < findings[j].SubjectID
	})
	return findings
}

// findingKey is the dedup key for findings within one Scan call.
type findingKey struct {
	subjectID string
	reason    string
	path      string
}

// addFinding appends a finding unless an identical (subjectID, reason, path)
// finding was already recorded. Returns the (possibly grown) slice.
func addFinding(findings []Finding, seen map[findingKey]struct{}, subjectID, reason, path string) []Finding {
	k := findingKey{subjectID: subjectID, reason: reason, path: path}
	if _, ok := seen[k]; ok {
		return findings
	}
	seen[k] = struct{}{}
	return append(findings, Finding{SubjectID: subjectID, Reason: reason, Path: path})
}

// scrubUnitMatches reports whether any scrub-project label matches the unit.
// Every label is matched as a case-insensitive substring against the content.
// Additionally, labels that look like path fragments (contain "/") are matched
// as case-insensitive substrings against the (already-lowercased) path. This is
// the conservative v1 policy: a label that is a directory/project path fragment
// is caught whether it appears in file content or in a file path.
//
// Both lowerContent and lowerPath are already lowercased by the caller; labels
// are lowercased here per-call (they are short).
func scrubUnitMatches(lowerContent, lowerPath string, labels []string) bool {
	for _, label := range labels {
		if label == "" {
			continue
		}
		lowerLabel := strings.ToLower(label)
		if strings.Contains(lowerContent, lowerLabel) {
			return true
		}
		// Path-fragment labels (containing "/") are also matched against the
		// path. This catches a sensitive directory/project path fragment that
		// appears in the scanned file's path even if not in its content.
		if strings.Contains(lowerLabel, "/") && lowerPath != "" && strings.Contains(lowerPath, lowerLabel) {
			return true
		}
	}
	return false
}

// anyTermInContent reports whether any term in terms appears as a
// case-insensitive substring in the already-lowercased content. An empty term
// never matches (it would trivially "match" everywhere). An empty terms slice
// never matches.
func anyTermInContent(lowerContent string, terms []string) bool {
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(lowerContent, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
