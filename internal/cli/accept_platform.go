package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/managedfile"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// acceptPlatformCmd implements `vh-agent-harness accept-platform <path>...`: the
// sanctioned recovery operation that adopts the platform's bytes for a
// preserved/stalled platform-managed path AND advances that path's recorded
// origin. It is the replacement for the former (now removed) manual
// "edit .vh-agent-harness/origin-hashes.json to re-baseline" ceremony, which
// contradicted the sidecar's binary-owned status (the docs said "do not edit"
// while the recovery REQUIRED editing it).
//
// Semantic contract (F2):
//   - EXACT paths only. Each positional arg is a repo-relative platform-managed
//     path that is currently in a preserved/stalled state (consumer-edit,
//     consumer-delete, unreadable, or migration unknown-baseline). The path is
//     normalized to forward-slashed repo-relative form; absolute or escaping
//     paths are rejected before any write.
//   - EXPLICIT operator invocation. It is never called automatically by update
//     or install; the operator names exactly which path(s) to recover.
//   - ORDERED apply + advance-origin per path (live-first, then origin-advance).
//     Validation runs BEFORE any write, so a REJECTED acceptance (unknown /
//     out-of-scope / not-currently-preserved / no-staged-bytes) touches nothing.
//     A successful acceptance writes the platform bytes to the live path and
//     records the corresponding origin hash. The two-object operation (live file
//   - JSON sidecar) is NOT transactional — see the failure model below.
//   - REJECT unknown / out-of-scope / not-preserved paths with a clear reason;
//     those leave state unchanged.
//   - NEVER requires manual deletion or editing of origin-hashes.json for
//     ordinary preserved-path re-baselining. The sidecar is binary-owned;
//     accept-platform is the sanctioned recovery for stalled paths. (A corrupt
//     or unsupported-schema sidecar has a separate removal-to-bootstrap recovery
//     documented at the storage layer, distinct from ordinary re-baselining.)
//
// The staged platform bytes come from the SAME prepareSeamStaging front-half
// install/update use, so the bytes accept-platform writes are byte-identical to
// what the next `update` would write UNDER IDENTICAL INPUTS (same binary,
// profile, overlays, target state) — the two render paths share one front-half.
//
// Failure model: writes are ordered live-first, then origin-advance. A
// live-write failure leaves the store untouched (origin stays at the prior
// value; the stall persists unchanged — re-tryable). The origin store is
// persisted once at the end of the batch via the store's atomic temp-file +
// rename (that rename IS atomic — the on-disk store is either the prior
// generation or the new one, never a half-written mix). If that persist fails,
// the live bytes already landed but the origin did NOT advance: the command
// reports the partial state and returns a NON-ZERO exit (it does not signal
// complete success). The next `update` self-heals the origin entry ONLY IF the
// rendered bytes remain identical to those just accepted (live == staged-hash →
// "" → origin advances); if the binary/profile/overlay inputs changed first,
// the path may re-stall and a fresh `accept-platform` re-adopts the current
// bytes. True cross-object atomicity (live file + JSON sidecar in one
// transaction) is not achievable without a transaction manager; the live-first
// order guarantees the failure mode is recoverable and never silently partial.
var (
	acceptPlatformTarget    string
	acceptPlatformDryRun    bool
	acceptPlatformAll       bool
	acceptPlatformStaleOnly bool
)

var acceptPlatformCmd = &cobra.Command{
	Use:           "accept-platform <path> [<path>...]",
	Short:         "Adopt the platform's version of a preserved/stalled managed file (re-baseline without editing origin-hashes.json)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Adopt the platform's version of a preserved/stalled managed file and
advance its recorded origin — the sanctioned way to re-baseline a managed file
the platform preserved (consumer-edit, consumer-delete, unreadable, or migration
unknown-baseline) WITHOUT editing the binary-owned origin-hashes.json sidecar.

Each positional arg is an EXACT repo-relative platform-managed path that update
or doctor reported as preserved/stalled. The path's platform bytes are written
to the live tree and the origin entry is advanced (live-first, then origin
advance via the sidecar's atomic temp-file + rename); only the named path(s)
change. Unknown, out-of-scope (not platform-managed), or not-currently-preserved
paths are rejected with a reason and leave state unchanged.

The bytes accept-platform writes are byte-identical to what the next
` + "`update`" + ` would write (it shares the seam's render front-half), so accepting a
path and then running ` + "`update`" + ` converges immediately — the path no longer stalls.

To make a consumer edit canonical ACROSS updates instead of discarding it,
promote the edit into an overlay pack source at
.vh-agent-harness/overlays/<pack>/ rather than accepting the platform version.

Batch mode: --all accepts every drifted/stalled platform-managed path in one
invocation (no positional paths needed). It enumerates the full platform-managed
ownership map and accepts every path currently in a preserved state; already-
converged paths are skipped (not failed). Add --stale-only to restrict --all to
the drifted+missing set ` + "`vh-agent-harness diff`" + ` reports (the VISIBLY stale paths).
--stale-only is the narrower sweep: unlike --all it does NOT candidate the
migration-stalled-but-in-sync paths (F6 UnknownBaseline whose live bytes happen
to match the render — diff reports them OK, not drifted/missing). NOTE:
--stale-only selects by render-bytes, not by origin; a consumer-preserved edit
whose live bytes differ from the render IS in the drifted set and WILL be
accepted (accept-platform is destructive-by-intent — it adopts the platform
bytes). Positional paths are still honored alongside --all (the union is
accepted). The per-path form (no --all) is unchanged and backward-compatible.

Re-run ` + "`vh-agent-harness update`" + ` or ` + "`doctor`" + ` after accepting to confirm convergence.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if acceptPlatformAll {
			return nil // --all: positional paths optional (batch enumeration)
		}
		if len(args) < 1 {
			return errors.New("accept-platform requires at least one path (or use --all to accept every drifted/stalled path)")
		}
		return nil
	},
	RunE: runAcceptPlatform,
}

func init() {
	acceptPlatformCmd.Flags().StringVarP(&acceptPlatformTarget, "target", "o", "",
		"target directory (default: current directory)")
	acceptPlatformCmd.Flags().BoolVar(&acceptPlatformDryRun, "dry-run", false,
		"preview which path(s) would be accepted (and why) without writing anything")
	acceptPlatformCmd.Flags().BoolVar(&acceptPlatformAll, "all", false,
		"accept every drifted/stalled platform-managed path in one invocation (no positional paths needed)")
	acceptPlatformCmd.Flags().BoolVar(&acceptPlatformStaleOnly, "stale-only", false,
		"with --all, restrict to the drifted+missing set `vh-agent-harness diff` reports (the visibly stale paths; does NOT candidate the migration-stalled-but-in-sync F6 paths). Selects by render-bytes: a consumer-preserved edit IS included if its live bytes differ from the render; ignored without --all")
}

// persistAcceptOriginStore is the SEAM over the batch origin-store persist in
// runAcceptPlatform. Production calls store.Write(target); unit tests override
// it to inject a DETERMINISTIC persist failure — proving the partial-state
// branch (live bytes landed, origin did NOT advance) returns a non-zero exit
// and reports the qualified self-heal warning, without requiring a real
// filesystem fault. It is package-scoped (not exported) so ONLY the
// accept-platform batch persist goes through it. Tests that override it MUST
// restore the default on cleanup.
var persistAcceptOriginStore = func(s *originhash.Store, target string) error {
	return s.Write(target)
}

func runAcceptPlatform(cmd *cobra.Command, args []string) (err error) {
	// A rejected acceptance is surfaced to stderr exactly once (SilenceErrors
	// suppresses Cobra's duplicate "Error:" line), matching update's convention.
	defer func() { reportRunErrToStderr(cmd, err) }()

	out := cmd.OutOrStdout()

	target := acceptPlatformTarget
	if target == "" {
		cwd, gerr := os.Getwd()
		if gerr != nil {
			return fmt.Errorf("getcwd: %w", gerr)
		}
		target = cwd
	}
	abs, aerr := filepath.Abs(target)
	if aerr != nil {
		return fmt.Errorf("resolve target: %w", aerr)
	}

	// Build the candidate path set. Positional paths are always honored; --all
	// expands the set to every drifted/stalled platform-managed path (--stale-only
	// narrows --all to the drifted+missing set `diff` reports). A bad positional
	// arg fails fast before any staging read or live write. The --all expansion
	// needs the EFFECTIVE ownership classifier (to exclude inactive-capability
	// residue and respect override-raised classes), so it runs AFTER staging.
	rels, cerr := normalizeAcceptCandidates(args)
	if cerr != nil {
		return fmt.Errorf("accept-platform: %w", cerr)
	}

	// Reuse the seam staging front-half so the bytes we apply are byte-identical
	// to what the next `update` would write. installRenderAnswers recovers the
	// install identity (project_name/slug) so the render is faithful to the
	// original install (same as update).
	answers := installRenderAnswers(abs)
	ps, cleanup, perr := prepareSeamStaging(abs, answers)
	if perr != nil {
		return fmt.Errorf("accept-platform: %w", perr)
	}
	defer cleanup()

	// STRICT store read: a corrupt sidecar must fail-closed (no advance). A nil
	// store (missing file = bootstrap) is fine — acceptOnePath handles per-path
	// nil-lookup, and the batch persists a fresh store if any path is accepted.
	store, serr := originhash.Read(abs)
	if serr != nil {
		return fmt.Errorf("accept-platform: origin store unreadable: %w", serr)
	}
	// Promote a bootstrap (nil) store to a fresh empty one so acceptOnePath can
	// mutate it in place. The batch only PERSISTS if at least one path is
	// accepted, so an all-rejected run on a bootstrap target writes nothing.
	if store == nil {
		store = originhash.New()
	}

	// Expand the candidate set for batch mode (--all / --stale-only). Done AFTER
	// staging so the expansion can filter through the EFFECTIVE classifier
	// (ps.cls) — this excludes inactive-capability residue (deselected pilots'
	// paths are absent from the active ownership map) and respects override-
	// raised classes, so --all never produces spurious "unknown path" failures
	// for paths the current selection does not manage.
	if acceptPlatformAll {
		expanded, eerr := expandAcceptAllCandidates(abs, ps, rels)
		if eerr != nil {
			return fmt.Errorf("accept-platform: %w", eerr)
		}
		rels = expanded
	}

	// Each path is an INDEPENDENT acceptance: a validation/live-write failure on
	// one does not roll back another. Already-succeeded paths stay accepted; the
	// command returns a non-zero exit if any path failed. In batch mode (--all /
	// --stale-only) a path that is already converged (benignSkip) is routed to a
	// separate "skipped" bucket instead of "failed" — the whole point of --all is
	// "accept the stalled, skip the rest", so skipping a converged path is the
	// expected outcome and must not flip the exit code. In the explicit positional
	// form benignSkip stays a failure for backward compatibility (you named a path
	// that did not need accepting).
	batchMode := acceptPlatformAll
	var accepted, failed, skipped []acceptResult
	for _, rel := range rels {
		res := acceptOnePath(ps, store, abs, rel, acceptPlatformDryRun)
		switch {
		case res.ok:
			accepted = append(accepted, res)
		case batchMode && res.benignSkip:
			skipped = append(skipped, res)
		default:
			failed = append(failed, res)
		}
	}

	// Persist the origin store once at the end of the batch (live-first order:
	// every accepted path's live bytes already landed; this advances the origin
	// for all of them via the store's atomic temp-file + rename). Skipped on
	// dry-run (nothing was written) and when nothing was accepted (no spurious
	// empty-store write on an all-rejected run).
	var persistErr error
	if !acceptPlatformDryRun && len(accepted) > 0 {
		if werr := persistAcceptOriginStore(store, abs); werr != nil {
			persistErr = werr
			// The live bytes landed but the origin did NOT advance. This is the
			// benign-but-INCOMPLETE partial: the two-object operation (live file
			// + JSON sidecar) is NOT transactional; we wrote live-first and the
			// sidecar persist failed. The next `update` self-heals the origin
			// entry ONLY IF the rendered bytes remain identical to those just
			// accepted (live == staged-hash -> "" -> origin advances); if the
			// binary/profile/overlay inputs change first, the path may re-stall
			// and a fresh `accept-platform` re-adopts the then-current bytes.
			// We do NOT roll back the live bytes (that would discard the
			// operator's explicit acceptance), but we DO return a non-zero exit
			// so automation cannot mistake a partial for complete success.
			fmt.Fprintf(out, "accept-platform: WARNING: platform bytes were written for %d path(s) but the origin store did NOT advance (%v).\n", len(accepted), werr)
			fmt.Fprintln(out, "The next `update` self-heals the origin entry when the rendered bytes are still identical to those accepted; if inputs changed, re-run `accept-platform` to re-adopt the current bytes. This run exits non-zero because the two-part acceptance did not fully complete.")
		}
	}

	// Per-path + summary reporting.
	if acceptPlatformDryRun {
		fmt.Fprintf(out, "accept-platform (dry-run): nothing written.\n")
	}
	if len(accepted) > 0 {
		verb := "would accept"
		if !acceptPlatformDryRun {
			verb = "accepted"
		}
		fmt.Fprintf(out, "accept-platform: %s %d path(s):\n", verb, len(accepted))
		for _, r := range accepted {
			fmt.Fprintf(out, "  %s  [%s resolved]\n", r.rel, r.reason)
		}
	}
	if batchMode && len(skipped) > 0 {
		fmt.Fprintf(out, "accept-platform: %d path(s) skipped (already converged):\n", len(skipped))
		for _, r := range skipped {
			fmt.Fprintf(out, "  %s  — %s\n", r.rel, r.reason)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(out, "accept-platform: %d path(s) NOT accepted:\n", len(failed))
		for _, r := range failed {
			fmt.Fprintf(out, "  %s  — %s\n", r.rel, r.reason)
		}
	}
	// Batch summary: a one-line N accepted / M skipped / K failed so an operator
	// (or automation) can confirm the batch outcome at a glance.
	if batchMode {
		fmt.Fprintf(out, "accept-platform summary: %d accepted, %d skipped, %d failed\n", len(accepted), len(skipped), len(failed))
	}
	if !acceptPlatformDryRun && len(accepted) > 0 {
		fmt.Fprintln(out, "Re-run `vh-agent-harness update` or `doctor` to confirm convergence.")
	}
	if len(failed) > 0 {
		return fmt.Errorf("accept-platform: %d of %d path(s) could not be accepted (see above)", len(failed), len(rels))
	}
	if persistErr != nil {
		return fmt.Errorf("accept-platform: origin store did not advance after %d successful live write(s) (%v); live bytes landed but acceptance is incomplete — see the warning above", len(accepted), persistErr)
	}
	return nil
}

// normalizeAcceptCandidates normalizes the positional path args up front so a
// bad arg fails fast (before any staging read or live write). It returns exactly
// the positional set; the --all expansion is handled separately by
// expandAcceptAllCandidates after staging.
func normalizeAcceptCandidates(args []string) ([]string, error) {
	rels := make([]string, 0, len(args))
	for _, a := range args {
		rel, rerr := normalizeAcceptPath(a)
		if rerr != nil {
			return nil, rerr
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

// expandAcceptAllCandidates expands the candidate set for batch mode (--all, and
// --stale-only which narrows --all). Positional paths are always honored and
// unioned with the expansion (deduped, then sorted for deterministic ordering).
//
// The expansion source is filtered through the EFFECTIVE ownership classifier so
// only ACTIVE platform-managed paths are candidated:
//
//   - --all (no --stale-only): every platform-managed path on the core ownership
//     map whose effective class is STILL platform-managed. This is the full
//     stalled/preserved universe — acceptOnePath's preservation check routes the
//     already-converged ones to the benign-skip bucket. It catches not only
//     visibly-drifted paths but also migration-stalled paths whose live bytes
//     happen to match the render yet carry no recorded origin (F6). Inactive-
//     capability residue (deselected pilots) is excluded: the classifier returns
//     !ok for those, so --all never produces spurious "unknown path" failures.
//   - --all --stale-only: restrict the expansion to the drifted+missing set
//     `vh-agent-harness diff` reports, filtered to platform-managed. This is the
//     NARROWER sweep: it does NOT candidate the migration-stalled-but-in-sync F6
//     UnknownBaseline paths (computeSeamDrift reports those OK, not drifted/
//     missing). The selection is by render-bytes, NOT by origin: a consumer-
//     preserved edit whose live bytes differ from the render IS in the drifted
//     set and is candidated (accept-platform is destructive-by-intent — it adopts
//     the platform bytes; the operator chose to run the sweep).
//
// --stale-only is meaningless without --all and is not consulted here otherwise.
func expandAcceptAllCandidates(target string, ps *seamStaging, positional []string) ([]string, error) {
	seen := make(map[string]bool, len(positional))
	for _, r := range positional {
		seen[r] = true
	}
	var extra []string
	if acceptPlatformStaleOnly {
		rep, derr := computeSeamDrift(target)
		if derr != nil {
			return nil, fmt.Errorf("--stale-only enumerate drifted/missing: %w", derr)
		}
		extra = append(extra, rep.drifted...)
		extra = append(extra, rep.missing...)
	} else {
		defaults, derr := corpus.CoreOwnershipDefaults()
		if derr != nil {
			return nil, fmt.Errorf("--all enumerate platform-managed: %w", derr)
		}
		defPaths := make([]string, 0, len(defaults))
		for path := range defaults {
			defPaths = append(defPaths, path)
		}
		sort.Strings(defPaths)
		extra = defPaths
	}
	out := append([]string(nil), positional...)
	for _, e := range extra {
		if seen[e] {
			continue
		}
		// Filter through the EFFECTIVE classifier so inactive-capability residue
		// (!ok) and override-raised / non-platform-managed classes are excluded
		// from the candidate set rather than producing noisy failures.
		co, ok := ps.cls.Classify(e)
		if !ok || co.Class != ownership.ClassPlatformManaged {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	sort.Strings(out)
	return out, nil
}

// acceptResult is the per-path outcome of one accept attempt. On success,
// reason is the PreservedReason string that was resolved (e.g. "consumer-edit");
// on failure, reason is a human explanation. benignSkip is true for the
// not-currently-preserved outcome (the path is already converged or platform-
// regenerated). In batch mode (--all / --stale-only) a benign skip is routed to
// the summary's "skipped" bucket and does NOT cause a non-zero exit (the whole
// point of --all is "accept the stalled, skip the rest"); in the explicit
// positional form a benign skip stays a failure for backward compatibility (you
// named a path that did not need accepting).
type acceptResult struct {
	rel        string
	reason     string
	ok         bool
	benignSkip bool
}

// acceptOnePath validates one path, then (live) writes its platform bytes and
// advances the in-memory store entry. The store is persisted once at batch end
// by the caller. On any validation failure it returns ok=false WITHOUT touching
// the live tree or the store — the "atomic or none" guarantee for rejections.
//
// The preservation check reuses the SAME shared classifier (managedfile.
// ClassifyPreserved) and the SAME effective-regenerated set that update's apply
// and doctor's drift check use, so accept-platform and update agree on what is
// stalled. A path that is NOT currently preserved (already converged, or
// platform-regenerated and managed canonically by update) is rejected —
// accept-platform is a focused recovery tool, not a general write tool.
func acceptOnePath(ps *seamStaging, store *originhash.Store, target, rel string, dryRun bool) acceptResult {
	// 1. Classify the path (fail-closed for unknown paths).
	co, ok := ps.cls.Classify(rel)
	if !ok {
		return acceptResult{rel: rel, reason: "unknown path (not on the ownership map)", ok: false}
	}
	if co.Class != ownership.ClassPlatformManaged {
		return acceptResult{rel: rel, reason: fmt.Sprintf("out-of-scope (ownership=%s; accept-platform targets platform_managed paths only — the origin store tracks platform_managed paths, not overlay_extension or project-owned)", co.Class), ok: false}
	}

	// 2. Read the staged platform bytes for this path (byte-identical to what
	// update would write). A path the corpus does not render has no staged copy.
	stagedBytes, serr := os.ReadFile(filepath.Join(ps.staging, filepath.FromSlash(rel)))
	if serr != nil {
		if errors.Is(serr, fs.ErrNotExist) {
			return acceptResult{rel: rel, reason: "no platform bytes staged for this path (not in the rendered corpus)", ok: false}
		}
		return acceptResult{rel: rel, reason: fmt.Sprintf("read staged bytes: %v", serr), ok: false}
	}

	// 3. Compute the current preservation status via the shared classifier.
	regenerated := ps.effectiveRegen[rel]
	origin, hadOrigin := store.Lookup(rel) // nil-safe
	live := buildAcceptLiveState(target, rel)
	stagedHash := originhash.Digest(stagedBytes)
	reason := managedfile.ClassifyPreserved(regenerated, hadOrigin, origin, live, stagedHash)
	if reason == "" {
		return acceptResult{rel: rel, reason: "not currently preserved (already converged or platform-regenerated); run `update` to reconcile", ok: false, benignSkip: true}
	}

	if dryRun {
		return acceptResult{rel: rel, reason: string(reason), ok: true}
	}

	// 4. Apply: write the platform bytes to the live path (live-first), then
	// advance the in-memory origin entry (persisted once at batch end).
	livePath := filepath.Join(target, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		return acceptResult{rel: rel, reason: fmt.Sprintf("create parent dir: %v", err), ok: false}
	}
	if err := os.WriteFile(livePath, stagedBytes, substrate.RenderWriteMode(livePath)); err != nil {
		return acceptResult{rel: rel, reason: fmt.Sprintf("write platform bytes: %v", err), ok: false}
	}
	// Live write succeeded: advance the origin entry. If the batch-level store
	// persist later fails, the next `update` self-heals (live == stagedHash).
	store.OriginHashes[rel] = stagedHash
	return acceptResult{rel: rel, reason: string(reason), ok: true}
}

// acceptReadLiveFile is the SEAM over the live-tree read used by
// buildAcceptLiveState, mirroring substrate.readLiveFile. Production uses
// os.ReadFile; unit tests override it to inject a DETERMINISTIC read failure
// (the Unreadable preserved-reason acceptance path) without relying on OS
// permission bits, which skip under root / permissive filesystems (a chmod-000
// probe is root-dependent and flaky in such CI). It is package-scoped (not
// exported) so ONLY buildAcceptLiveState's live read goes through it; staged
// reads (acceptOnePath's hashSHA256 of the staged copy) keep using os.ReadFile
// directly, matching substrate's seam boundary. Tests that override it MUST
// restore the default (acceptReadLiveFile = os.ReadFile) on cleanup.
var acceptReadLiveFile = os.ReadFile

// buildAcceptLiveState observes the live file at target/<rel> and returns the
// managedfile.LiveState the shared classifier consumes. It mirrors substrate's
// internal buildLiveState exactly (stat -> Absent/regular/dir; read+hash ->
// Readable+Hash) so accept-platform and update make identical preservation
// decisions from identical on-disk state. Exported-internal so a future test
// helper can assert parity without reaching into substrate.
func buildAcceptLiveState(target, rel string) managedfile.LiveState {
	livePath := filepath.Join(target, filepath.FromSlash(rel))
	info, err := os.Stat(livePath)
	if err != nil {
		if os.IsNotExist(err) {
			return managedfile.LiveState{Absent: true}
		}
		// A stat error other than NotExist (permission / transient) is NOT a
		// preserved state — fall through to the classifier's "" branch, matching
		// substrate.buildLiveState's treatment.
		return managedfile.LiveState{}
	}
	if !info.Mode().IsRegular() {
		return managedfile.LiveState{}
	}
	data, rerr := acceptReadLiveFile(livePath)
	if rerr != nil {
		// Regular file but unreadable — the Unreadable preserved reason.
		return managedfile.LiveState{IsRegular: true, Readable: false}
	}
	return managedfile.LiveState{IsRegular: true, Readable: true, Hash: originhash.Digest(data)}
}

// normalizeAcceptPath cleans a user-supplied path arg to the forward-slashed
// repo-relative form the ownership map and origin store key on. It rejects
// absolute paths and any path that escapes the project root (a ".." component
// or a Clean result of ".."/"../..."). A leading "./" is accepted and stripped.
func normalizeAcceptPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty path argument")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("path %q is absolute; accept-platform takes repo-relative platform-managed paths (e.g. \".opencode/agents/build.md\")", p)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the project root", p)
	}
	return cleaned, nil
}
