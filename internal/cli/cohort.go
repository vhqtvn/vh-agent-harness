package cli

// cohort.go implements the O4 Slice-2 pre-write cohort-link check: an
// eslink.Scan over the EFFECTIVE .opencode/ JS tree a seam apply is about to
// produce, partitioned by the ratified O3 boundary predicate (Decision 2,
// researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-scope.md §3).
//
// The effective tree is what the live runtime would load AFTER the proposed
// apply: advancing/no-op staged bytes, ACTUAL retained live bytes for
// preserved/diverged files (staged bytes are NOT what a preserved path keeps),
// adopter/unmanaged live files swept in for whole-tree graph continuity, and
// consumer-deleted paths omitted. Scanning staged bytes alone (or live bytes
// alone) would assess a tree nobody will ever run — the mixed tree is the
// adopter-representable reality the state-lib skew family demonstrated.
//
// Boundary predicate (O3): a PROVEN broken edge refuses iff EITHER endpoint is
// in the active platform set (the staged render is the membership oracle —
// "currently-would-render" semantics, not ever-rendered). Adopter-internal
// proven edges and every unknown finding are WARN-side only (U6: unknown never
// refuses). Diverged-managed endpoints sit on the FAIL side: a preserved
// platform file is still a platform file.
//
// Refusal contract (6 elements, verbatim from the ratified record): broken
// edges + symbols; SHA-256 of the assessed bytes; preservation confirmation;
// docs pointer + safeguard-first reconciliation (backup/diff first, sequential
// accept-platform repair WITH the flag — never a promise that plain
// accept-platform always resolves); doctor-not-suppressed caveat; and the
// mandatory literal rider below. A refusal writes ZERO files and exits
// non-zero; --allow-broken-cohort converts the refusal into a full WARN
// printout and proceeds (one-shot, never persisted, never doctor-suppressing).

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/eslink"
	"github.com/vhqtvn/vh-agent-harness/internal/managedfile"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// cohortRider is the MANDATORY literal scope rider (ratified element 6). The
// cohort check proves link integrity ONLY; it never claims behavior,
// semantics, or logic of preserved files was verified. Every refusal and
// warning message this file emits carries it verbatim.
const cohortRider = "link integrity only; behavior of preserved files is not verified"

// cohortDryRunDisclaimer is the Decision-1B disclaimer printed under --dry-run
// when a live run would refuse: preview completion is NOT live-readiness
// certification.
const cohortDryRunDisclaimer = "Notice: dry-run exit 0 indicates preview completion only and does not certify live update readiness."

// cohortDocsPointer names the durable recovery documentation (element 4).
const cohortDocsPointer = "vh-agent-harness docs preserved-managed-reconciliation"

// errCohortRefusal is the sentinel wrapped by the pre-write refusal error so
// callers/tests can distinguish a cohort-link refusal (deliberate, zero-write,
// contract-printed) from an ordinary runtime failure. It never aborts output
// by itself: the full 6-element diagnostic has already been printed to stderr
// before the wrapping error is returned.
var errCohortRefusal = fmt.Errorf("cohort-link refusal")

// ActivePlatformPaths returns the ACTIVE platform set for the O3 boundary
// predicate: every path in the staged render (slash-clean repo-relative keys,
// the walkStaged precedent) that is NOT inactive-capability residue and whose
// effective ownership class is platform_managed or overlay_extension. The
// staged render is the membership oracle — this is the "currently-would-render"
// set, not the ever-rendered set — so diverged-managed endpoints (a preserved
// platform file IS still a platform file) and consumer-deleted platform paths
// both remain members; a deleted path simply cannot endpoint a proven broken
// edge (the scanner's provider-presence precondition).
//
// Shared by the seamApply preflight, the acceptOnePath gate, and doctor's
// cohort-links check (#28), so all three sites partition edges against the
// SAME active set derived from the SAME staging walk.
func ActivePlatformPaths(stagingDir string, inactive map[string]bool, classifier *substrate.Classifier) map[string]bool {
	out := map[string]bool{}
	if classifier == nil {
		return out
	}
	for rel := range walkStagedLivePaths(stagingDir) {
		if inactive != nil && inactive[rel] {
			continue
		}
		co, ok := classifier.Classify(rel)
		if !ok {
			continue
		}
		switch co.Class {
		case ownership.ClassPlatformManaged, ownership.ClassOverlayExtension:
			out[rel] = true
		}
	}
	return out
}

// isCohortScannedPath reports whether rel (slash-clean repo-relative) belongs
// to the cohort graph: a .js/.mjs module under .opencode/. Everything else
// (markdown, json, templates) is outside the link graph by design.
func isCohortScannedPath(rel string) bool {
	if !strings.HasPrefix(rel, ".opencode/") {
		return false
	}
	return strings.HasSuffix(rel, ".js") || strings.HasSuffix(rel, ".mjs")
}

// sweepLiveCohortTree reads every .js/.mjs file under <target>/.opencode/ into
// a tree keyed by slash-clean repo-relative path. This is the RETAINED-reality
// half of the effective tree: preserved/diverged files keep exactly these
// bytes, and adopter/unmanaged files enter the graph only through this sweep.
// Unreadable files are omitted (a read failure never fabricates bytes and
// never aborts the check — the scanner treats the absent module honestly).
func sweepLiveCohortTree(target string) (map[string][]byte, error) {
	tree := map[string][]byte{}
	root := filepath.Join(target, ".opencode")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // no .opencode tree (greenfield): empty sweep
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(target, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !isCohortScannedPath(rel) {
			return nil
		}
		data, derr := os.ReadFile(p)
		if derr != nil {
			return nil // unreadable: omit (honest absence), never fail the check
		}
		tree[rel] = data
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cohort: walk live .opencode tree: %w", err)
	}
	return tree, nil
}

// assembleEffectiveCohortTree builds the .opencode/ JS tree this apply would
// leave behind (the adopter-representable mixed tree):
//
//   - advancing/seeded outcomes overlay their STAGED bytes (what the apply will
//     write, or seed into an absent slot);
//   - no-op outcomes need no overlay — live already equals staged byte-for-byte
//     (that is the no-op definition), so the live sweep already carries the
//     right bytes;
//   - preserved/diverged and project-preserved outcomes keep the ACTUAL
//     retained live bytes from the sweep (staged bytes are NOT what a preserved
//     path keeps — assessing staged bytes there would scan a tree nobody runs);
//     a consumer-deleted preserved path is absent from the sweep and therefore
//     omitted from the effective tree;
//   - armed outcomes are never .js/.mjs in the corpus (the armed set is the
//     profile yaml + opencode.jsonc); if one ever appears, the live sweep's
//     bytes stand — the retained-reality choice for a value not writable from
//     staging.
func assembleEffectiveCohortTree(target, stagingDir string, outcomes []substrate.FileOutcome) (map[string][]byte, error) {
	tree, err := sweepLiveCohortTree(target)
	if err != nil {
		return nil, err
	}
	for _, o := range outcomes {
		if !isCohortScannedPath(o.Path) {
			continue
		}
		switch o.Action {
		case substrate.ActionManagedOverwrite, substrate.ActionProjectSeeded:
			b, rerr := os.ReadFile(filepath.Join(stagingDir, filepath.FromSlash(o.Path)))
			if rerr != nil {
				return nil, fmt.Errorf("cohort: read staged bytes for %s: %w", o.Path, rerr)
			}
			tree[o.Path] = b
		}
	}
	return tree, nil
}

// cohortFileHash is one SHA-256 receipt over the assessed prospective bytes
// (element 2 of the refusal contract).
type cohortFileHash struct {
	Path string
	SHA  string // originhash.Digest form: "sha256:<hex>"
}

// cohortPreservedFile is one preservation-confirmation row (element 3): a
// path this run will NOT touch, with the typed reason it is preserved.
type cohortPreservedFile struct {
	Path   string
	Reason string
}

// cohortCheck is the complete verdict of one pre-write cohort scan: the O3
// partition of proven broken edges, the unknown findings (never a refusal
// basis), the SHA-256 receipts over the assessed tree, and the preservation
// confirmation rows.
type cohortCheck struct {
	Refuse    []eslink.BrokenEdge // proven broken, either endpoint active-platform
	Warn      []eslink.BrokenEdge // proven broken, both endpoints non-platform (adopter-internal)
	Unknown   []eslink.UnknownFinding
	Hashes    []cohortFileHash      // sorted by path
	Preserved []cohortPreservedFile // sorted by path
	Scanned   int                   // modules in the assessed tree
}

// HasWarnSide reports whether any WARN-side finding exists (adopter-internal
// proven broken edges or unknowns) that should be surfaced even when nothing
// refuses.
func (c *cohortCheck) HasWarnSide() bool {
	return c != nil && (len(c.Warn) > 0 || len(c.Unknown) > 0)
}

// scanCohortTree runs eslink.Scan over tree, partitions proven broken edges by
// the O3 boundary predicate against the active platform set, and attaches the
// SHA-256 receipts for every assessed module.
func scanCohortTree(tree map[string][]byte, active map[string]bool) *cohortCheck {
	rep := eslink.Scan(tree)
	c := &cohortCheck{Scanned: rep.Modules}
	for _, e := range rep.Broken {
		if active[e.Importer] || active[e.Provider] {
			c.Refuse = append(c.Refuse, e)
		} else {
			c.Warn = append(c.Warn, e)
		}
	}
	c.Unknown = rep.Unknown
	for _, p := range sortedTreeKeys(tree) {
		c.Hashes = append(c.Hashes, cohortFileHash{Path: p, SHA: originhash.Digest(tree[p])})
	}
	return c
}

func sortedTreeKeys(tree map[string][]byte) []string {
	out := make([]string, 0, len(tree))
	for k := range tree {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cohortPreservedFromOutcomes derives the preservation-confirmation rows from
// a seam apply PLAN: every diverged platform path (typed PreservedReason) and
// every preserved project-owned path. All preserved files are enumerated —
// not only scanned JS — because the confirmation promises no file anywhere was
// modified, clobbered, or deleted.
func cohortPreservedFromOutcomes(outcomes []substrate.FileOutcome) []cohortPreservedFile {
	var out []cohortPreservedFile
	for _, o := range outcomes {
		switch o.Action {
		case substrate.ActionManagedDiverged:
			reason := string(o.PreservedReason)
			if reason == "" {
				reason = "diverged"
			}
			out = append(out, cohortPreservedFile{Path: o.Path, Reason: reason})
		case substrate.ActionProjectPreserved:
			out = append(out, cohortPreservedFile{Path: o.Path, Reason: "project-owned"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// scanSeamCohort is the seamApply site: assemble the effective tree from the
// plan outcomes + live sweep, scan it, partition by the O3 predicate, and
// attach the preservation rows from the same plan.
func scanSeamCohort(target string, ps *seamStaging, outcomes []substrate.FileOutcome) (*cohortCheck, error) {
	tree, err := assembleEffectiveCohortTree(target, ps.staging, outcomes)
	if err != nil {
		return nil, err
	}
	active := ActivePlatformPaths(ps.staging, ps.inactiveLive, ps.cls)
	c := scanCohortTree(tree, active)
	c.Preserved = cohortPreservedFromOutcomes(outcomes)
	return c, nil
}

// scanProspectiveCohort is the acceptOnePath site (the single-substitution
// scan): the prospective tree is the CURRENT disk bytes with ONLY the proposed
// path's staged bytes substituted. There is no batch mechanism — each
// acceptance is gated on the tree IT alone would produce; sequential repair
// with the escape flag is the ratified path when one substitution cannot heal
// every platform-bound broken link.
//
// The preservation rows are derived from the origin store via the SAME shared
// classifier acceptOnePath itself uses (managedfile.ClassifyPreserved), so the
// gate's notion of "preserved" matches the accept verb's own. They enumerate
// ALL staged paths (not only scanned JS — element 3 requires confirmation for
// every preserved file), EXCEPT the substitute path itself: that path's
// preserved state is what this acceptance resolves, and the prospective tree
// assessed its STAGED substituted bytes, not its retained live bytes.
func scanProspectiveCohort(target string, ps *seamStaging, store *originhash.Store, substituteRel string, substituteBytes []byte) (*cohortCheck, error) {
	tree, err := sweepLiveCohortTree(target)
	if err != nil {
		return nil, err
	}
	if isCohortScannedPath(substituteRel) {
		tree[substituteRel] = substituteBytes
	}
	active := ActivePlatformPaths(ps.staging, ps.inactiveLive, ps.cls)
	c := scanCohortTree(tree, active)

	// Preservation rows over the FULL staged set (element 3 covers every
	// preserved file, not only scanned JS), excluding the substitute path.
	for _, rel := range stagedSortedPaths(ps.staging) {
		if rel == substituteRel {
			continue
		}
		regenerated := ps.effectiveRegen[rel]
		origin, hadOrigin := store.Lookup(rel) // nil-safe
		live := buildAcceptLiveState(target, rel)
		stagedBytes, serr := os.ReadFile(filepath.Join(ps.staging, filepath.FromSlash(rel)))
		if serr != nil {
			continue // no staged copy: not a preserved platform path here
		}
		if reason := managedfile.ClassifyPreserved(regenerated, hadOrigin, origin, live, originhash.Digest(stagedBytes)); reason != "" {
			c.Preserved = append(c.Preserved, cohortPreservedFile{Path: rel, Reason: string(reason)})
		}
	}
	sort.Slice(c.Preserved, func(i, j int) bool { return c.Preserved[i].Path < c.Preserved[j].Path })
	return c, nil
}

// stagedSortedPaths returns every staged path (slash-clean, sorted).
func stagedSortedPaths(stagingDir string) []string {
	var out []string
	for rel := range walkStagedLivePaths(stagingDir) {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

// --- diagnostic rendering -----------------------------------------------------

// cohortDiagMode selects the disposition framing of the full diagnostic block.
type cohortDiagMode int

const (
	// cohortModeRefuse: a live apply is refused (zero writes performed).
	cohortModeRefuse cohortDiagMode = iota
	// cohortModeWouldRefuse: dry-run preview of a refusal (nothing written,
	// Decision 1B: informational, exit 0).
	cohortModeWouldRefuse
	// cohortModeOverride: --allow-broken-cohort set; ALL diagnostics printed
	// as WARN and the apply proceeds (one-shot, not persisted).
	cohortModeOverride
)

// formatCohortDiagnostic renders the full 6-element refusal contract (or its
// would-refuse / override framings — identical evidence, different
// disposition headline). preservationClaim carries the SITE- and
// mode-accurate element-3 zero-write statement: the formatter itself never
// claims invocation-wide zero writes, because the override framing proceeds
// into a live write and the accept-platform batch is per-path (earlier
// accepted paths in the same invocation may already have landed). Every
// variant carries the literal rider.
func formatCohortDiagnostic(c *cohortCheck, mode cohortDiagMode, subject, preservationClaim string) string {
	var b strings.Builder
	headline := ""
	switch mode {
	case cohortModeRefuse:
		headline = fmt.Sprintf("REFUSING%s: %d proven broken link(s) touch platform-managed files — zero files were written by the refused operation", subject, len(c.Refuse))
	case cohortModeWouldRefuse:
		headline = fmt.Sprintf("WOULD REFUSE on a live run%s: %d proven broken link(s) touch platform-managed files (dry-run preview; nothing written)", subject, len(c.Refuse))
	case cohortModeOverride:
		headline = fmt.Sprintf("proceeding with --allow-broken-cohort%s: %d proven broken link(s) touch platform-managed files — ALL diagnostics printed as WARN (one-shot, not persisted)", subject, len(c.Refuse))
	}
	fmt.Fprintf(&b, "vh-agent-harness: cohort-link preflight: %s\n", headline)

	fmt.Fprintf(&b, "  1) proven broken link(s) touching platform-managed files (%d):\n", len(c.Refuse))
	for _, e := range c.Refuse {
		fmt.Fprintf(&b, "     - %s imports %q from %s (required symbol: %s)\n       %s\n",
			e.Importer, e.Specifier, e.Provider, e.Symbol, e.Reason)
	}
	fmt.Fprintf(&b, "  1a) adopter-internal proven broken link(s) (%d — reported, never a refusal basis):\n", len(c.Warn))
	for _, e := range c.Warn {
		fmt.Fprintf(&b, "     - %s imports %q from %s (required symbol: %s)\n       %s\n",
			e.Importer, e.Specifier, e.Provider, e.Symbol, e.Reason)
	}
	fmt.Fprintf(&b, "  1b) unknown finding(s) (%d — never a refusal basis):\n", len(c.Unknown))
	for _, u := range c.Unknown {
		fmt.Fprintf(&b, "     - %s %q: %s\n", u.Path, u.Specifier, u.Reason)
	}

	fmt.Fprintf(&b, "  2) SHA-256 of assessed prospective bytes (%d module(s) scanned, %d hashed):\n", c.Scanned, len(c.Hashes))
	for _, h := range c.Hashes {
		fmt.Fprintf(&b, "     - %s  %s\n", h.SHA, h.Path)
	}

	fmt.Fprintf(&b, "  3) preservation confirmation (%d preserved file(s)) — %s:\n", len(c.Preserved), preservationClaim)
	for _, p := range c.Preserved {
		fmt.Fprintf(&b, "     - [%s] %s\n", p.Reason, p.Path)
	}

	fmt.Fprintf(&b, "  4) recovery: docs: %s\n", cohortDocsPointer)
	fmt.Fprintln(&b, "     safeguard-first: back up and diff BEFORE acting (`vh-agent-harness update --dry-run` previews the per-file plan);")
	fmt.Fprintln(&b, "     adopting platform bytes with accept-platform DISCARDS local edits — back up preserved files first.")
	fmt.Fprintln(&b, "     repair path: adopt platform bytes ONE path at a time — `vh-agent-harness accept-platform <path> --allow-broken-cohort`;")
	fmt.Fprintln(&b, "     a plain accept-platform (no flag) proceeds only when that single acceptance alone heals every platform-bound broken link,")
	fmt.Fprintln(&b, "     so it is NOT guaranteed to resolve an update refusal. Non-destructive alternative for ADDITIONS: promote the edit into an")
	fmt.Fprintln(&b, "     overlay pack (.vh-agent-harness/overlays/<pack>/) at a NEW path — a pack cannot shadow a builtin at the same path;")
	fmt.Fprintln(&b, "     same-path customization instead requires a project_owned override in harness-ownership.yml and ongoing reconciliation.")

	fmt.Fprintln(&b, "  5) override caveat: --allow-broken-cohort proceeds for THIS run only (one-shot, never persisted in lineage, profile, or state)")
	fmt.Fprintln(&b, "     and does NOT suppress subsequent doctor failures — doctor still reports cohort-link findings.")

	fmt.Fprintf(&b, "  6) scope: %s\n", cohortRider)
	return b.String()
}

// The element-3 preservation claims, per site and mode. They exist so the
// formatter never asserts more zero-writes than the emitting operation
// actually guarantees (the override framings proceed into live writes; the
// accept-platform batch is per-path, not transactional).
const (
	// seam site, refused or would-refused: the refused operation wrote nothing.
	cohortClaimSeamRefused = "this run wrote nothing: no file was modified, clobbered, or deleted"
	// seam site, override: preservation is byte-for-byte, but the apply DOES
	// write advancing (platform-managed) files after this warning.
	cohortClaimSeamOverride = "preserved files are retained byte-for-byte — never modified, clobbered, or deleted; the proceeding apply writes only advancing (platform-managed) files, not these"
	// accept site, refused or would-refused: per-path claim — earlier
	// accepted paths in the same invocation may already have landed.
	cohortClaimAcceptRefused = "this acceptance wrote nothing: no file was modified, clobbered, or deleted by it (accept-platform is per-path, not transactional — earlier accepted path(s) in this invocation, if any, already landed)"
	// accept site, override: the acceptance writes exactly the accepted
	// path's staged platform bytes; every other preserved file stands.
	cohortClaimAcceptOverride = "the proceeding acceptance writes ONLY this path's staged platform bytes; every other preserved file is retained byte-for-byte — not modified, clobbered, or deleted (accept-platform is per-path, not transactional)"
)

// formatCohortWarnBlock renders the compact WARN-side report used when nothing
// refuses but adopter-internal proven broken edges and/or unknown findings
// exist. It is deliberately lighter than the full refusal contract (no hashes,
// no preservation rows) but carries the same literal rider.
func formatCohortWarnBlock(c *cohortCheck) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vh-agent-harness: cohort-link preflight: warnings (never a refusal basis):\n")
	fmt.Fprintf(&b, "  adopter-internal proven broken link(s) (%d):\n", len(c.Warn))
	for _, e := range c.Warn {
		fmt.Fprintf(&b, "    - %s imports %q from %s (required symbol: %s)\n      %s\n",
			e.Importer, e.Specifier, e.Provider, e.Symbol, e.Reason)
	}
	fmt.Fprintf(&b, "  unknown finding(s) (%d):\n", len(c.Unknown))
	for _, u := range c.Unknown {
		fmt.Fprintf(&b, "    - %s %q: %s\n", u.Path, u.Specifier, u.Reason)
	}
	fmt.Fprintf(&b, "  scope: %s\n", cohortRider)
	return b.String()
}

// emitSeamCohortDiagnostics applies the ratified disposition policy for the
// seamApply site and prints to w. It returns whether the apply must be
// abandoned (live refusal). On a live refusal the caller returns the
// errCohortRefusal-wrapped error AFTER this printer has emitted the full
// contract; on a dry-run refusal (Decision 1B) the caller continues so the
// normal plan preview still prints and the command exits 0.
func emitSeamCohortDiagnostics(w io.Writer, c *cohortCheck, dryRun, allowBrokenCohort bool) (refuse bool) {
	if len(c.Refuse) > 0 {
		if allowBrokenCohort {
			fmt.Fprint(w, formatCohortDiagnostic(c, cohortModeOverride, "", cohortClaimSeamOverride))
			return false
		}
		if dryRun {
			fmt.Fprint(w, formatCohortDiagnostic(c, cohortModeWouldRefuse, "", cohortClaimSeamRefused))
			// Decision 1B: the would-refuse preview is informational — the
			// command still exits 0 so preview pipelines can inspect the
			// staged diff. The verbatim disclaimer states that bound.
			fmt.Fprintf(w, "  %s\n", cohortDryRunDisclaimer)
			return false
		}
		fmt.Fprint(w, formatCohortDiagnostic(c, cohortModeRefuse, "", cohortClaimSeamRefused))
		return true
	}
	if c.HasWarnSide() {
		fmt.Fprint(w, formatCohortWarnBlock(c))
	}
	return false
}
