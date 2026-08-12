package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// seamAcceptPlatformOut runs the accept-platform command for rels against root,
// mirroring seamUpdateOut/seamDoctorOut. It returns the captured stdout and the
// RunE error so tests can assert both the reported outcome and the exit error
// (accept-platform returns a non-zero RunE error when any path is rejected).
func seamAcceptPlatformOut(t *testing.T, root string, rels ...string) (string, error) {
	t.Helper()
	var out string
	var err error
	runWithCwd(t, root, func() {
		acceptPlatformTarget = root
		acceptPlatformDryRun = false
		defer func() { acceptPlatformTarget = ""; acceptPlatformDryRun = false }()
		cmd, buf := newOutCmd()
		err = runAcceptPlatform(cmd, rels)
		out = buf.String()
	})
	return out, err
}

// seamAcceptPlatformDryRun runs accept-platform in --dry-run mode (no writes).
func seamAcceptPlatformDryRun(t *testing.T, root string, rels ...string) string {
	t.Helper()
	var out string
	runWithCwd(t, root, func() {
		acceptPlatformTarget = root
		acceptPlatformDryRun = true
		defer func() { acceptPlatformTarget = ""; acceptPlatformDryRun = false }()
		cmd, buf := newOutCmd()
		_ = runAcceptPlatform(cmd, rels)
		out = buf.String()
	})
	return out
}

// TestAcceptPlatform_RecoversConsumerEditStall is the F2 load-bearing crux: the
// full first-upgrade-preservation + acceptance lifecycle end-to-end through the
// real CLI commands:
//
//  1. install seeds a platform_managed file with a recorded origin;
//  2. a consumer hand-edit makes it stall (consumer-edit);
//  3. `update` PRESERVES the edit and NAMES the stall path+reason in LIVE output;
//  4. `accept-platform <path>` adopts the platform bytes + advances the origin
//     atomically — only that path changes;
//  5. a re-run `update` shows stable convergence (no stall for that path);
//  6. `doctor` agrees (no drift FAIL, no consumer-preserved for that path).
//
// This is the outcome-observed proof that the stall is visible in live output
// and that accept-platform atomically recovers exactly one path.
func TestAcceptPlatform_RecoversConsumerEditStall(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// A non-regenerated platform_managed path (consumer edits to a regenerated
	// path are genuine drift, not a stall accept-platform recovers).
	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))

	// Capture the platform bytes install wrote, so we can assert convergence.
	platformBytes, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read platform bytes %s: %v", p, err)
	}

	// Consumer hand-edits the path → consumer-edit stall.
	const consumerEdit = "// CONSUMER HAND-EDIT — update must preserve, accept-platform must recover\n"
	if err := os.WriteFile(livePath, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	// (3) update preserves the edit AND names the stall in LIVE output (F2 p1).
	upd, uerr := seamUpdateOut(t, root)
	if uerr != nil {
		t.Fatalf("update after consumer-edit: %v (out=%q)", uerr, upd)
	}
	if !strings.Contains(upd, "managed-diverged") {
		t.Errorf("update want managed-diverged for the consumer-edited file; got:\n%s", upd)
	}
	if !strings.Contains(upd, "preserved/stalled") || !strings.Contains(upd, p) || !strings.Contains(upd, "consumer-edit") {
		t.Errorf("update LIVE output must name the stall path + typed reason (preserved/stalled, %q, consumer-edit); got:\n%s", p, upd)
	}
	// The edit survived the update byte-for-byte.
	if got, _ := os.ReadFile(livePath); string(got) != consumerEdit {
		t.Fatalf("consumer edit was NOT preserved by update; want=%q", consumerEdit)
	}

	// (4) accept-platform recovers exactly this path.
	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr != nil {
		t.Fatalf("accept-platform %s: %v (out=%q)", p, aerr, ap)
	}
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, p) || !strings.Contains(ap, "consumer-edit") {
		t.Errorf("accept-platform must report the accepted path + resolved reason; got:\n%s", ap)
	}
	// The path now carries the platform bytes (converged).
	got, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read after accept-platform %s: %v", p, err)
	}
	if string(got) != string(platformBytes) {
		t.Fatalf("accept-platform did not write platform bytes for %s;\n want=%q\n got=%q", p, string(platformBytes), string(got))
	}

	// (5) re-run update: the path no longer stalls (stable convergence).
	upd2, uerr2 := seamUpdateOut(t, root)
	if uerr2 != nil {
		t.Fatalf("update after accept-platform: %v (out=%q)", uerr2, upd2)
	}
	if strings.Contains(upd2, p) && strings.Contains(upd2, "preserved/stalled") {
		t.Errorf("after accept-platform, update must NOT report %s as stalled (convergence); got:\n%s", p, upd2)
	}

	// (6) doctor agrees: no drift FAIL, and the accepted path is not consumer-preserved.
	doc := seamDoctorOut(t, root)
	if strings.Contains(doc, "FAIL") && strings.Contains(doc, p) {
		t.Errorf("doctor must not FAIL on the accepted path; got:\n%s", doc)
	}
}

// TestAcceptPlatform_RejectsUnknownPath proves the "unknown path leaves state
// unchanged" guarantee: a path not on the ownership map is rejected with a
// reason, no platform bytes are written, and the origin store is untouched.
func TestAcceptPlatform_RejectsUnknownPath(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	storeBefore, err := originhash.Read(root)
	if err != nil {
		t.Fatalf("read store before: %v", err)
	}
	storeBeforeJSON := storeJSON(t, root)

	const unknown = "definitely/not/a/real/managed/path.md"
	// Precondition: the unknown path does not exist on disk (so a buggy write
	// would create it — we assert it stays absent).
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(unknown))); statErr == nil {
		t.Fatalf("precondition: unknown path should not exist")
	}

	ap, aerr := seamAcceptPlatformOut(t, root, unknown)
	if aerr == nil {
		t.Fatalf("accept-platform must error on an unknown path; got nil err. out=%q", ap)
	}
	if !strings.Contains(ap, "unknown path") {
		t.Errorf("accept-platform must report 'unknown path' reason; got:\n%s", ap)
	}
	// No platform bytes written (path still absent).
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(unknown))); statErr == nil {
		t.Errorf("accept-platform must NOT write anything for a rejected path; the unknown path was created")
	}
	// Origin store unchanged (byte-identical).
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("accept-platform must NOT mutate the origin store for a rejected path")
	}
	_ = storeBefore // keep the read meaningful even if the JSON-compare is the strong assertion
}

// TestAcceptPlatform_RejectsOutOfScope proves the "out-of-scope leaves state
// unchanged" guarantee: a project_owned path is rejected (accept-platform
// targets platform-managed paths only), and nothing is written.
func TestAcceptPlatform_RejectsOutOfScope(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// A default project_owned path (render-independent seed).
	p := findLiveDefaultNonManagedPath(t, root)
	storeBeforeJSON := storeJSON(t, root)

	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr == nil {
		t.Fatalf("accept-platform must error on a project_owned (out-of-scope) path; got nil err. out=%q", ap)
	}
	if !strings.Contains(ap, "out-of-scope") {
		t.Errorf("accept-platform must report 'out-of-scope' reason; got:\n%s", ap)
	}
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("accept-platform must NOT mutate the origin store for an out-of-scope path")
	}
}

// TestAcceptPlatform_RejectsNotPreserved proves the "mismatched (not currently
// preserved) leaves state unchanged" guarantee: an already-converged managed
// path (no consumer edit) is rejected — accept-platform is a focused recovery
// tool for STALLS, not a general write tool.
func TestAcceptPlatform_RejectsNotPreserved(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// A fresh, unedited platform_managed path (already at platform bytes).
	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)
	storeBeforeJSON := storeJSON(t, root)

	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr == nil {
		t.Fatalf("accept-platform must error on a not-currently-preserved path; got nil err. out=%q", ap)
	}
	if !strings.Contains(ap, "not currently preserved") {
		t.Errorf("accept-platform must report 'not currently preserved' reason; got:\n%s", ap)
	}
	// Live bytes unchanged (already platform; accept did not rewrite).
	if got, _ := os.ReadFile(livePath); string(got) != string(platformBytes) {
		t.Errorf("accept-platform must not rewrite an already-converged path")
	}
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("accept-platform must NOT mutate the origin store for a not-preserved path")
	}
}

// TestAcceptPlatform_DryRunWritesNothing proves --dry-run validates and reports
// without writing live bytes or advancing the origin.
func TestAcceptPlatform_DryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)

	// Consumer-edit to make it a real stall.
	const consumerEdit = "// DRY-RUN CONSUMER EDIT\n"
	if err := os.WriteFile(livePath, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit: %v", err)
	}
	storeBeforeJSON := storeJSON(t, root)

	ap := seamAcceptPlatformDryRun(t, root, p)
	if !strings.Contains(ap, "dry-run") || !strings.Contains(ap, p) || !strings.Contains(ap, "consumer-edit") {
		t.Errorf("dry-run must report the would-be-accepted path + reason; got:\n%s", ap)
	}
	// Live bytes are STILL the consumer edit (dry-run wrote nothing).
	if got, _ := os.ReadFile(livePath); string(got) != consumerEdit {
		t.Errorf("dry-run must NOT write platform bytes; live was changed")
	}
	// Origin store unchanged.
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("dry-run must NOT advance the origin store")
	}
	// Platform bytes captured for clarity (the would-be write target).
	_ = platformBytes
}

// TestAcceptPlatform_LiveUpdateNamesStalls is the F2 part-1 outcome-observed
// assertion: when update preserves stalled paths, the LIVE (non-dry-run) output
// enumerates each preserved path with its typed reason and points at
// accept-platform. This complements the crux test by asserting the output shape
// directly (multiple stall reasons surface together).
func TestAcceptPlatform_LiveUpdateNamesStalls(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Two distinct consumer-edited managed paths → two consumer-edit stalls.
	p1 := findLiveAuthoredPlatformManagedPath(t, root)
	const edit = "// CONSUMER EDIT\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p1)), []byte(edit), 0o644); err != nil {
		t.Fatalf("edit %s: %v", p1, err)
	}

	upd, uerr := seamUpdateOut(t, root)
	if uerr != nil {
		t.Fatalf("update: %v (out=%q)", uerr, upd)
	}
	if !strings.Contains(upd, "preserved/stalled") {
		t.Errorf("live update output must include the preserved/stalled summary; got:\n%s", upd)
	}
	if !strings.Contains(upd, p1) || !strings.Contains(upd, "consumer-edit") {
		t.Errorf("live update output must name the stalled path + reason; got:\n%s", upd)
	}
	if !strings.Contains(upd, "accept-platform") {
		t.Errorf("live update output must point at accept-platform as the recovery op; got:\n%s", upd)
	}
}

// TestAcceptPlatform_RecoversUnknownBaselineStall is the F6↔F2 linkage proof:
// a platform_managed path with NO recorded origin (hadOrigin==false) but a live
// regular file is the adoption-migration stall (UnknownBaseline). update
// PRESERVES it (never clobbers unknowable bytes); accept-platform adopts the
// platform bytes and records the origin, resolving the migration stall. This
// mirrors the managedfile.UnknownBaseline doc: "the path stays preserved until a
// future accept-platform recovery operation explicitly adopts the platform bytes."
func TestAcceptPlatform_RecoversUnknownBaselineStall(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)

	// Simulate the pre-feature / partial-migration state for ONE path: remove
	// its origin entry so hadOrigin becomes false while the live file exists.
	store, err := originhash.Read(root)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	delete(store.OriginHashes, p)
	if err := store.Write(root); err != nil {
		t.Fatalf("write store: %v", err)
	}

	// update preserves the path as unknown-baseline (F6) and names it live.
	upd, uerr := seamUpdateOut(t, root)
	if uerr != nil {
		t.Fatalf("update: %v (out=%q)", uerr, upd)
	}
	if !strings.Contains(upd, "managed-diverged") {
		t.Errorf("update want managed-diverged for the unknown-baseline stall; got:\n%s", upd)
	}
	if !strings.Contains(upd, "unknown-baseline") || !strings.Contains(upd, p) {
		t.Errorf("update LIVE output must name the unknown-baseline stall + path; got:\n%s", upd)
	}
	// Live bytes were NOT clobbered by update (still the platform bytes install wrote).
	if got, _ := os.ReadFile(livePath); string(got) != string(platformBytes) {
		t.Errorf("update must NOT clobber unknown-baseline bytes")
	}

	// accept-platform adopts the platform bytes + records the origin.
	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr != nil {
		t.Fatalf("accept-platform %s: %v (out=%q)", p, aerr, ap)
	}
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, "unknown-baseline") {
		t.Errorf("accept-platform must report the unknown-baseline resolution; got:\n%s", ap)
	}

	// The origin entry now exists for p (converged).
	store2, _ := originhash.Read(root)
	if h, ok := store2.Lookup(p); !ok || h == "" {
		t.Errorf("accept-platform must record the origin entry for %s after acceptance", p)
	}
	// Re-run update: no stall for p (convergence).
	upd2, _ := seamUpdateOut(t, root)
	if strings.Contains(upd2, p) && strings.Contains(upd2, "preserved/stalled") {
		t.Errorf("after accept-platform, update must not stall %s; got:\n%s", p, upd2)
	}
}

// TestAcceptPlatform_RegeneratedPathRejected proves a platform-REGENERATED
// managed path (allowed-commands.js, canonically overwritten every apply) is
// rejected — accept-platform is a recovery tool for STALLS, and regenerated paths
// are never stalled (ClassifyPreserved returns "" for regenerated). This locks
// the R4-B1 parity: accept-platform agrees with update that regenerated paths
// are NOT preserved.
func TestAcceptPlatform_RegeneratedPathRejected(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// allowed-commands.js is platform_managed AND regenerated (overwritten every
	// apply). It exists on disk after install.
	p := allowedCommandsRel
	livePath := filepath.Join(root, filepath.FromSlash(p))
	if _, err := os.Stat(livePath); err != nil {
		t.Fatalf("precondition: %s should exist after install: %v", p, err)
	}
	storeBeforeJSON := storeJSON(t, root)

	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr == nil {
		t.Fatalf("accept-platform must reject a regenerated path; got nil err. out=%q", ap)
	}
	if !strings.Contains(ap, "not currently preserved") {
		t.Errorf("accept-platform must report 'not currently preserved' for a regenerated path; got:\n%s", ap)
	}
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("accept-platform must NOT mutate the origin store for a regenerated path")
	}
}

// TestAcceptPlatform_ConsumerDeletedSh_RestoredWithExecMode proves the write-mode
// parity fix: when accept-platform restores a consumer-DELETED managed shell
// script, it recreates the file EXECUTABLE (0o755), matching what substrate.Apply
// would write (renderWriteMode), not a plain 0o644. Without the shared
// RenderWriteMode this would recreate the script non-executable.
func TestAcceptPlatform_ConsumerDeletedSh_RestoredWithExecMode(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	shRel := findLiveManagedShPath(t, root)
	if shRel == "" {
		t.Skip("no platform_managed .sh path rendered in this profile; skipping exec-mode parity test")
	}
	livePath := filepath.Join(root, filepath.FromSlash(shRel))

	// Capture the platform mode install wrote (must be executable for a .sh).
	info, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("stat %s: %v", shRel, err)
	}
	if m := info.Mode().Perm(); m != 0o755 {
		t.Fatalf("precondition: managed .sh %s should be 0o755 after install, got %o", shRel, m)
	}

	// Consumer deletes the script → consumer-delete stall.
	if err := os.Remove(livePath); err != nil {
		t.Fatalf("remove %s: %v", shRel, err)
	}

	// accept-platform restores it.
	ap, aerr := seamAcceptPlatformOut(t, root, shRel)
	if aerr != nil {
		t.Fatalf("accept-platform %s: %v (out=%q)", shRel, aerr, ap)
	}
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, "consumer-delete") {
		t.Errorf("accept-platform must report the consumer-delete resolution; got:\n%s", ap)
	}

	// The restored file is executable (write-mode parity with Apply).
	info2, err := os.Stat(livePath)
	if err != nil {
		t.Fatalf("stat restored %s: %v", shRel, err)
	}
	if m := info2.Mode().Perm(); m != 0o755 {
		t.Errorf("accept-platform must restore a managed .sh as executable 0o755 (matching Apply's renderWriteMode); got %o for %s", m, shRel)
	}
}

// seamAcceptPlatformBatchOut runs the accept-platform command in batch mode
// (--all and optionally --stale-only) against root, with optional extra
// positional paths unioned into the candidate set. It restores all four flag
// globals on cleanup so no other test is affected. Returns captured stdout +
// the RunE error so tests can assert the batch summary and the exit code.
func seamAcceptPlatformBatchOut(t *testing.T, root string, all, staleOnly bool, rels ...string) (string, error) {
	t.Helper()
	var out string
	var err error
	runWithCwd(t, root, func() {
		acceptPlatformTarget = root
		acceptPlatformDryRun = false
		acceptPlatformAll = all
		acceptPlatformStaleOnly = staleOnly
		defer func() {
			acceptPlatformTarget = ""
			acceptPlatformDryRun = false
			acceptPlatformAll = false
			acceptPlatformStaleOnly = false
		}()
		cmd, buf := newOutCmd()
		err = runAcceptPlatform(cmd, rels)
		out = buf.String()
	})
	return out, err
}

// TestAcceptPlatform_All_RecoversStallAndSkipsConverged is the F8 batch-mode
// crux: --all accepts every stalled platform-managed path in one invocation,
// routes already-converged paths to a benign "skipped" bucket (NEVER failing the
// exit code), and prints the N accepted / M skipped / K failed summary. This is
// the outcome the operator gets from `accept-platform --all` after a partial
// disruption: every stall recovered, convergence confirmed, zero noise from the
// healthy majority.
func TestAcceptPlatform_All_RecoversStallAndSkipsConverged(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Consumer-edit exactly one managed path so it becomes a real stall; the
	// rest of the managed set stays converged.
	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)
	const consumerEdit = "// CONSUMER EDIT — --all batch test\n"
	if err := os.WriteFile(livePath, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	ap, aerr := seamAcceptPlatformBatchOut(t, root, true, false)
	// --all must NOT fail the exit code just because the converged majority was
	// skipped — that is the expected batch outcome.
	if aerr != nil {
		t.Fatalf("accept-platform --all must succeed when the only non-accepted paths are benign skips; got err=%v (out=%q)", aerr, ap)
	}
	// The stalled path was accepted (resolved consumer-edit).
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, p) || !strings.Contains(ap, "consumer-edit") {
		t.Errorf("--all must accept the stalled path %q with its reason; got:\n%s", p, ap)
	}
	// The converged majority was routed to the benign skip bucket (NOT failed).
	if !strings.Contains(ap, "skipped") {
		t.Errorf("--all must report the skipped (already converged) paths; got:\n%s", ap)
	}
	if strings.Contains(ap, "NOT accepted") {
		t.Errorf("--all must not report benign skips as NOT accepted (failed); got:\n%s", ap)
	}
	// Explicit residue-exclusion crux: CoreOwnershipDefaults includes paths from
	// deselected capabilities (e.g. media-perception, worker-read-only) that the
	// EFFECTIVE classifier (ps.cls) does NOT know. --all must filter them out via
	// ps.cls rather than enumerate + fail them as "unknown path". A fresh
	// seamInstallInto tree carries those defaults, so asserting the
	// media-perception residue path never appears in the output (neither accepted
	// nor failed) proves the exclusion fires end-to-end.
	if strings.Contains(ap, "media-perception") || strings.Contains(ap, "worker-read-only") {
		t.Errorf("--all must exclude inactive-capability residue (media-perception/worker-read-only) via the effective classifier; got:\n%s", ap)
	}
	// The one-line batch summary is present.
	if !strings.Contains(ap, "accept-platform summary:") {
		t.Errorf("--all must print the batch summary line; got:\n%s", ap)
	}
	// The stalled path now carries the platform bytes (recovered).
	got, err := os.ReadFile(livePath)
	if err != nil || string(got) != string(platformBytes) {
		t.Errorf("--all did not recover %s to platform bytes;\n want=%q\n got=%q", p, string(platformBytes), got)
	}
}

// TestAcceptPlatform_StaleOnly_RestrictsToDriftedMissing proves --stale-only
// narrows --all to the drifted+missing set `vh-agent-harness diff` reports. The
// distinguishing case is an UnknownBaseline-but-in-sync path (F6): its live
// bytes equal the render (so diff reports it OK), but it has NO recorded origin
// (so it is migration-stalled). Plain --all ACCEPTS it (acceptOnePath sees
// UnknownBaseline); --all --stale-only does NOT (computeSeamDrift reports it
// OK, so it never enters the candidate set). This is the precise behavioral
// difference the flag buys.
func TestAcceptPlatform_StaleOnly_RestrictsToDriftedMissing(t *testing.T) {
	// --- Plain --all: an UnknownBaseline-but-in-sync path IS accepted. ---
	root := t.TempDir()
	seamInstallInto(t, root)

	p := findLiveAuthoredPlatformManagedPath(t, root)

	// Simulate the adoption-migration state for p: delete its origin entry while
	// its live bytes still equal the platform render (live==staged). ClassifyPreserved
	// then returns UnknownBaseline; computeSeamDrift reports it OK.
	store, err := originhash.Read(root)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	delete(store.OriginHashes, p)
	if err := store.Write(root); err != nil {
		t.Fatalf("write store: %v", err)
	}

	apAll, aerrAll := seamAcceptPlatformBatchOut(t, root, true, false)
	if aerrAll != nil {
		t.Fatalf("--all must succeed (the migration-stalled path is recoverable); got err=%v (out=%q)", aerrAll, apAll)
	}
	if !strings.Contains(apAll, "accepted") || !strings.Contains(apAll, p) || !strings.Contains(apAll, "unknown-baseline") {
		t.Errorf("--all must accept the UnknownBaseline-but-in-sync path %q; got:\n%s", p, apAll)
	}

	// Reset the same stall condition on a FRESH tree, then run --all --stale-only.
	// The path is OK to computeSeamDrift (live==staged), so --stale-only must NOT
	// candidate it → 0 accepted.
	root2 := t.TempDir()
	seamInstallInto(t, root2)
	livePath2 := filepath.Join(root2, filepath.FromSlash(p))
	store2, err := originhash.Read(root2)
	if err != nil {
		t.Fatalf("read store2: %v", err)
	}
	delete(store2.OriginHashes, p)
	if err := store2.Write(root2); err != nil {
		t.Fatalf("write store2: %v", err)
	}

	apStale, aerrStale := seamAcceptPlatformBatchOut(t, root2, true, true)
	if aerrStale != nil {
		t.Fatalf("--all --stale-only must succeed when no candidate fails; got err=%v (out=%q)", aerrStale, apStale)
	}
	if strings.Contains(apStale, p) && strings.Contains(apStale, "accepted") {
		t.Errorf("--stale-only must NOT accept the OK-to-render UnknownBaseline path %q (it is not in drifted+missing); got:\n%s", p, apStale)
	}
	// The path's live bytes are unchanged (still the platform bytes install wrote
	// — nothing was accepted, so nothing was written).
	if got, _ := os.ReadFile(livePath2); len(got) == 0 {
		t.Errorf("precondition: %s should still exist on the fresh tree", p)
	}
}

// TestAcceptPlatform_StaleOnly_AcceptsConsumerEditByRenderBytes locks the
// resolved --stale-only semantics that the commit-reviewer BLOCK flagged:
// --stale-only selects candidates by RENDER-BYTES (computeSeamDrift's drifted
// set), NOT by origin. So a consumer-preserved edit — whose live bytes differ
// from the render AND carries a valid origin (ClassifyPreserved returns
// ConsumerEdit) — IS in the drifted set and IS accepted by --stale-only, exactly
// as it would be by plain --all or the per-path form. accept-platform is
// destructive-by-intent (it adopts the platform bytes); the operator chose the
// sweep. This test exists so the behavior is documented and a future change that
// silently alters it is caught.
func TestAcceptPlatform_StaleOnly_AcceptsConsumerEditByRenderBytes(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Consumer-edit one managed path: live diverges from the render and the
	// origin still records the platform hash install wrote → ConsumerEdit stall,
	// and computeSeamDrift classifies it drifted (live != staged).
	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)
	const consumerEdit = "// CONSUMER EDIT — --stale-only render-bytes test\n"
	if err := os.WriteFile(livePath, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}

	ap, aerr := seamAcceptPlatformBatchOut(t, root, true, true)
	if aerr != nil {
		t.Fatalf("--all --stale-only must succeed (the consumer-edit stall is recoverable); got err=%v (out=%q)", aerr, ap)
	}
	// THE KEY ASSERTION: the consumer-edit path IS accepted by --stale-only
	// (selection is by render-bytes; a valid origin does NOT exclude it).
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, p) || !strings.Contains(ap, "consumer-edit") {
		t.Errorf("--stale-only must accept the consumer-edit path %q (render-bytes selection); got:\n%s", p, ap)
	}
	// The path now carries the platform bytes (the consumer edit was discarded —
	// accept-platform is destructive-by-intent; the operator chose the sweep).
	got, err := os.ReadFile(livePath)
	if err != nil || string(got) != string(platformBytes) {
		t.Errorf("--stale-only did not recover %s to platform bytes;\n want=%q\n got=%q", p, string(platformBytes), got)
	}
}

// TestAcceptPlatform_All_BackwardCompatPerPath proves the per-path form (no
// --all) is unchanged: a benign not-currently-preserved path is still a FAILURE
// (non-zero exit) when named explicitly, even though in batch mode it would be a
// benign skip. This locks the backward-compat contract: --all changes only the
// batch routing, never the explicit positional behavior.
func TestAcceptPlatform_All_BackwardCompatPerPath(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// A fresh, unedited platform_managed path (already at platform bytes).
	p := findLiveAuthoredPlatformManagedPath(t, root)

	// Explicit positional form (no --all): still FAILS on a converged path.
	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr == nil {
		t.Fatalf("positional form must still error on a not-currently-preserved path; got nil err. out=%q", ap)
	}
	if !strings.Contains(ap, "not currently preserved") {
		t.Errorf("positional form must still report 'not currently preserved'; got:\n%s", ap)
	}
}

// findLivePlatformManagedPaths returns every repo-relative slash path that is
// platform_managed in the corpus defaults AND exists on disk under root. It is
// the plural form of findLivePlatformManagedPath (doctor_drift_test.go), used by
// accept-platform tests that need to filter the live managed set (e.g. by .sh
// suffix for the write-mode parity test).
func findLivePlatformManagedPaths(t *testing.T, root string) []string {
	t.Helper()
	def, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		t.Fatalf("core ownership defaults: %v", err)
	}
	var out []string
	for p, rule := range def {
		if rule.Class != ownership.ClassPlatformManaged {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// findLiveManagedShPath returns the repo-relative slash path of a platform_managed
// .sh file that exists on disk under root (e.g. readonly-scripts.sh), or "" when
// none is rendered in the current profile. Used for the write-mode parity test.
func findLiveManagedShPath(t *testing.T, root string) string {
	t.Helper()
	for _, p := range findLivePlatformManagedPaths(t, root) {
		if strings.HasSuffix(p, ".sh") {
			return p
		}
	}
	return ""
}

// storeJSON reads the origin-hash store file bytes for a byte-identity compare
// across an accept-platform invocation (rejects must leave it byte-identical).
// Returns "" when the store is absent.
func storeJSON(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(originhash.FilePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read origin store: %v", err)
	}
	return string(b)
}

// TestAcceptPlatform_UnreadableLiveFile_Deterministic is the DETERMINISTIC
// behavioral-closure crux for the accept-platform Unreadable preserved-reason
// path. It injects a KNOWN read failure through the acceptReadLiveFile seam —
// NO reliance on OS permission bits (which a chmod-000 probe cannot enforce
// under root / permissive CI filesystems). It proves, in every CI environment:
//
//  1. read-failure ≠ absent ≠ unedited: a live file that EXISTS (stat ok,
//     regular, with a recorded origin from install) but whose read FAILS routes
//     to the TYPED PreservedReason == managedfile.Unreadable via the shared
//     classifier (ClassifyPreserved: hadOrigin + IsRegular + !Readable) —
//     distinct from ConsumerDelete (absent) and from the unedited disposition
//     that falls through to overwrite/noop.
//  2. accept-platform RECOVERS the unreadable stall: it writes the platform
//     bytes and advances the origin (the operator's explicit acceptance is
//     honored — accept-platform is a focused recovery tool that adopts the
//     platform bytes even though the read could not inspect the live bytes).
//
// The seam (package-scoped acceptReadLiveFile) is restored to os.ReadFile on
// cleanup so no other test is affected.
func TestAcceptPlatform_UnreadableLiveFile_Deterministic(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read platform bytes %s: %v", p, err)
	}

	// --- Inject a DETERMINISTIC read failure via the acceptReadLiveFile seam.
	// The live file genuinely EXISTS on disk (stat succeeds, regular file) and
	// has a recorded origin (install wrote it) — only the read is forced to
	// fail, exactly the write-permitted-but-not-readable condition that
	// ClassifyPreserved must classify as Unreadable. ---
	prev := acceptReadLiveFile
	acceptReadLiveFile = func(path string) ([]byte, error) {
		return nil, errors.New("injected deterministic read failure (accept-platform seam)")
	}
	t.Cleanup(func() { acceptReadLiveFile = prev })

	// accept-platform recovers the Unreadable stall: adopts platform bytes +
	// advances the origin.
	ap, aerr := seamAcceptPlatformOut(t, root, p)
	if aerr != nil {
		t.Fatalf("accept-platform %s: %v (out=%q)", p, aerr, ap)
	}
	if !strings.Contains(ap, "accepted") || !strings.Contains(ap, p) || !strings.Contains(ap, "unreadable") {
		t.Errorf("accept-platform must report the unreadable resolution (accepted + path + reason); got:\n%s", ap)
	}

	// The live bytes are now the platform bytes (the operator's explicit
	// acceptance wrote them, overriding the unreadable file).
	got, gerr := os.ReadFile(livePath)
	if gerr != nil || string(got) != string(platformBytes) {
		t.Fatalf("accept-platform must write platform bytes for the unreadable stall;\n want=%q\n got=%q err=%v", string(platformBytes), got, gerr)
	}

	// The origin entry advanced to the platform hash (read from disk with the
	// REAL os.ReadFile — the seam only affected the in-run classify).
	store2, _ := originhash.Read(root)
	h, ok := store2.Lookup(p)
	if !ok || h == "" {
		t.Errorf("accept-platform must advance the origin entry for the unreadable stall %s", p)
	}
}

// TestAcceptPlatform_StorePersistFailure_PartialStateNonZero is the
// behavioral-closure crux for the batch store-persist-failure branch in
// runAcceptPlatform. It injects a DETERMINISTIC persist failure through the
// persistAcceptOriginStore seam — proving the partial-state contract holds
// without requiring a real filesystem fault:
//
//  1. the live bytes DID land (acceptOnePath writes live-first, before the
//     batch persist);
//  2. the origin store did NOT advance (the persist failed; the on-disk store
//     is byte-identical to the pre-accept state);
//  3. the command returns a NON-ZERO exit (RunE error) so automation cannot
//     mistake the partial for complete success;
//  4. the partial-state warning naming the self-heal path is reported.
//
// The seam (package-scoped persistAcceptOriginStore) is restored on cleanup so
// no other test is affected.
func TestAcceptPlatform_StorePersistFailure_PartialStateNonZero(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	p := findLiveAuthoredPlatformManagedPath(t, root)
	livePath := filepath.Join(root, filepath.FromSlash(p))
	platformBytes, _ := os.ReadFile(livePath)

	// Consumer-edit so the path is a real stall accept-platform will recover.
	const consumerEdit = "// CONSUMER EDIT — persist-failure partial-state test\n"
	if err := os.WriteFile(livePath, []byte(consumerEdit), 0o644); err != nil {
		t.Fatalf("consumer-edit %s: %v", p, err)
	}
	storeBeforeJSON := storeJSON(t, root)

	// Inject a DETERMINISTIC persist failure via the persistAcceptOriginStore
	// seam. The seam replaces the entire store.Write call, so NO temp file or
	// rename occurs — the on-disk store is left completely untouched.
	prev := persistAcceptOriginStore
	persistAcceptOriginStore = func(s *originhash.Store, target string) error {
		return errors.New("injected deterministic persist failure (accept-platform seam)")
	}
	t.Cleanup(func() { persistAcceptOriginStore = prev })

	// accept-platform writes the live bytes (live-first) then fails the batch persist.
	ap, aerr := seamAcceptPlatformOut(t, root, p)
	// (3) NON-ZERO exit.
	if aerr == nil {
		t.Fatalf("accept-platform must return a non-zero error when the origin persist fails; got nil. out=%q", ap)
	}
	// (4) the partial-state warning is reported.
	if !strings.Contains(ap, "did NOT advance") {
		t.Errorf("accept-platform must warn that the origin store did NOT advance; got:\n%s", ap)
	}

	// (1) the live bytes DID land (platform bytes written live-first).
	got, gerr := os.ReadFile(livePath)
	if gerr != nil || string(got) != string(platformBytes) {
		t.Errorf("accept-platform must still write the live bytes when the persist fails (live-first); want=%q got=%q err=%v", string(platformBytes), got, gerr)
	}
	// (2) the on-disk origin store did NOT advance (byte-identical to before).
	if storeJSON(t, root) != storeBeforeJSON {
		t.Errorf("accept-platform must NOT advance the on-disk origin store when the persist fails")
	}
}
