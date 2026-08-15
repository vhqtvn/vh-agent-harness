package substrate

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/managedfile"
	"github.com/vhqtvn/vh-agent-harness/internal/originhash"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/renderstate"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// ApplyOptions configures one seam apply (render-into-staging is already done by
// the caller via Renderer.Render; Apply operates on the staged tree).
type ApplyOptions struct {
	// ProjectRoot is the live project tree (the source of truth for owned/armed
	// files). Apply writes here, per-class, from staging.
	ProjectRoot string
	// StagingDir is where the renderer already rendered the template. Apply never
	// renders into ProjectRoot.
	StagingDir string
	// Classifier is the seam's read view over S2 ownership.
	Classifier *Classifier
	// HarnessVersion feeds lineage.UpdateID (content-addressed render id).
	HarnessVersion string
	// TemplateSource / Commit / Ref record the render origin for lineage (S1).
	TemplateSource string
	Commit         string
	Ref            string
	// Answers feed the lineage answer digest (S1) AND, in the prototype, the
	// raw install-identity values lineage.yml carries so doctor/update can
	// re-render faithfully. Pass the caller-supplied install answers
	// (project_name/slug), NOT the profile-merged render answers.
	Answers map[string]string
	// DryRun, when true, computes and returns the full per-file plan (the
	// ApplyReport) WITHOUT executing any write or touching lineage. The plan
	// phase is side-effect-free, so a dry-run is a safe preview an operator (or
	// agent) inspects before applying.
	DryRun bool
	// RegeneratedPlatformPaths is the set of platform_managed paths the platform
	// REGENERATES canonically on every apply (e.g. .opencode/repo-configs/
	// allowed-commands.js, emitted from Go tables). The origin-hash three-way
	// preservation does NOT apply to these: they are generated, not consumer-
	// authored content, and they MUST stay in sync with the platform's canonical
	// emission (preserving a stale consumer customization would desync the
	// shell-guard from the permission blocks). A nil/empty set means no
	// exemptions — every platform_managed file gets the three-way check (the
	// default for non-seam callers and unit tests).
	RegeneratedPlatformPaths map[string]bool
}

// FileAction labels what the seam did to one staged file. It is the machine
// label the ApplyReport carries per file.
type FileAction string

const (
	ActionManagedOverwrite FileAction = "managed-overwrite"   // platform_managed -> overwrite
	ActionManagedNoop      FileAction = "managed-unchanged"   // platform_managed/active overlay_extension already up to date
	ActionManagedDiverged  FileAction = "managed-diverged"    // platform_managed diverged from origin (consumer-edited) or consumer-deleted -> skip, NEVER clobber (origin-hash three-way)
	ActionProjectPreserved FileAction = "project-preserved"   // project_owned present -> skip
	ActionProjectSeeded    FileAction = "project-seeded"      // project_owned absent -> seed once
	ActionArmedMerged      FileAction = "armed-merged"        // platform_armed clean reconcile applied
	ActionArmedProposal    FileAction = "armed-proposal"      // platform_armed needs-decision; not written
	ActionArmedNoop        FileAction = "armed-noop"          // platform_armed already up to date
	ActionUnsupportedClass FileAction = "skipped-unsupported" // reserved for future off-lattice classes (overlay_extension/external_generated now implemented in Slice 4)
	ActionIgnoredLocal     FileAction = "ignored-local-only"
)

// WriteState is the additive, machine-readable execution state of one staged
// file's live write. It is the TYPED correctness signal that distinguishes a
// write that SUCCEEDED from one that FAILED (or was never attempted), independent
// of the human-readable Note string. Provenance consumers gate ONLY on this typed
// field — the Note string must NEVER become a correctness signal (the corrupt
// "ERROR ..." Note-scanning pattern is explicitly rejected by P1-LINEAGE-002 v1.1).
//
// This field is ADDITIVE: it carries the zero value "" (normalized to
// WriteNotAttempted at plan time) on outcomes produced by older code, so a
// reader built against this struct never sees a state it cannot interpret. It
// does NOT change substrate.Apply's ERROR return semantics — Apply still returns
// an error only for walk/plan/lineage-write failures; a live-write failure sets
// WriteState=WriteFailed (and a Note) and returns normally (nil error), because
// a partial application is a distinct, recoverable state from a hard failure.
// What DID change (P1-SUBSTRATE-001) is the PROVENANCE consequence: when any
// outcome is WriteFailed, Apply now gates lineage (does NOT advance it) and
// records GenerationFullyApplied=false on the report. The typed Note string
// remains diagnostics-only — never a correctness signal.
type WriteState string

const (
	// WriteNotAttempted: no live write was performed (dry-run, preserved,
	// proposal, noop, unsupported, ignored, or any non-write action). This is
	// also the plan-time default for every outcome before execution.
	WriteNotAttempted WriteState = "not_attempted"
	// WriteSucceeded: the live write completed (the destination now holds the
	// staged/reconciled value).
	WriteSucceeded WriteState = "succeeded"
	// WriteFailed: a write WAS attempted but did not complete — a staged-read,
	// mkdir, reconcile, or live-write error. The live destination was NOT
	// updated for this path.
	WriteFailed WriteState = "failed"
)

// FileOutcome is the seam's per-file result.
type FileOutcome struct {
	Path      string
	Class     ownership.Class
	Action    FileAction
	Applied   []string          // human-readable merge notes (armed-merged)
	Proposals []schema.Proposal // populated when Action == ActionArmedProposal
	Note      string            // extra context (e.g. why skipped) — human diagnostics ONLY, never a correctness signal
	// PreservedReason is the TYPED correctness signal for WHY a platform_managed
	// outcome is ActionManagedDiverged (the origin-hash three-way preservation).
	// It is set (non-empty) only for diverged outcomes and carries one of the
	// managedfile taxonomy values (ConsumerEdit / ConsumerDelete / Unreadable);
	// the human Note string carries the same reason as diagnostics only. Before
	// this field existed the reason lived solely in the Note string — which is
	// explicitly "never a correctness signal" — so any consumer (or the lint
	// path, doctor) that needed the reason had to scan Note text, the corrupt
	// pattern this typed field retires. The empty value means "not a preserved
	// divergence" (every non-diverged action).
	PreservedReason managedfile.PreservedReason
	WriteState      WriteState // typed live-write execution state (not_attempted/succeeded/failed)
}

// ApplyReport is the seam's result.
type ApplyReport struct {
	Outcomes     []FileOutcome
	StagingDir   string
	LineagePath  string // absolute path to the written lineage.yml
	Proposals    []schema.Proposal
	RendererName string
	// GenerationFullyApplied reports whether the generation completed every live
	// write it attempted. It is false when ANY FileOutcome carries
	// WriteState == WriteFailed (a live-write failure). When false, Apply does
	// NOT advance lineage: the prior lineage record is left byte-for-byte intact
	// and LineagePath stays "". Provenance consumers gate ONLY on this typed
	// signal — lineage must never claim a generation that did not fully apply
	// (P1-SUBSTRATE-001). Apply still returns a nil error in that case: a
	// partial application (some files wrote, some did not) is a distinct,
	// recoverable state from the hard failures (walk/plan/lineage-write) that
	// return an error.
	GenerationFullyApplied bool
	// Orphans are the report-only preserved-orphan findings (overlay skills
	// whose producing source has been removed but whose rendered copy is still
	// on disk). Populated by the seam (internal/cli/seam.go) after Apply; it is
	// NOT computed by Apply itself. v1 reports only — the platform never deletes
	// these files. Surfaced in both update --dry-run and normal update.
	Orphans []renderstate.OrphanFinding
}

// Apply runs the seam: it walks the staged tree, classifies every candidate via
// S2, plans all per-class outcomes (validating fail-closed BEFORE any write so a
// mis-authored ownership map aborts without touching the live tree), then
// executes the writes. When (and only when) the generation fully applied (no
// live-write failure) it writes the D3-B lineage file; a partially-failed
// generation leaves the prior lineage byte-for-byte intact and keeps
// report.LineagePath "" (P1-SUBSTRATE-001).
//
// Atomicity contract: the live tree is never churned. The render happened in
// staging (a scratch directory), never in ProjectRoot. project_owned files are
// never overwritten when present (preserved) and seeded at most once when absent.
// platform_armed files are overwritten only with a clean schema-reconciled value;
// a needs-decision conflict leaves the project instance untouched (a proposal is
// emitted instead). A fail-closed unclassified path aborts before any write.
//
// Return semantics: Apply returns an error ONLY for the hard failures that abort
// the pipeline or break provenance (walk/plan/lineage-write). A live-write
// failure is NOT a hard error — it is a partial application (a distinct,
// recoverable state): Apply returns nil, records GenerationFullyApplied=false on
// the report, and skips the lineage write. Callers that gate downstream
// side-effects on generation success MUST read report.GenerationFullyApplied,
// not just the error.
func Apply(r Renderer, opts ApplyOptions) (*ApplyReport, error) {
	// 1. Enumerate staged candidate files (sorted, deterministic).
	staged, err := walkStaged(opts.StagingDir)
	if err != nil {
		return nil, fmt.Errorf("walk staging: %w", err)
	}

	// Load the prior origin-hash store (the three-way divergence input for the
	// platform_managed branch of planOutcome). A MISSING store (Read returns
	// nil, nil) is the bootstrap case: no prior origin recorded, so every
	// platform_managed file is treated as unedited (overwritten/seeded) and
	// origin hashes are recorded fresh. A PRESENT-BUT-CORRUPT or unsupported-
	// schema store is FAIL-CLOSED: returning the error aborts the apply BEFORE
	// any live-tree write. This is deliberate — a nil store would make Lookup
	// report no prior origin for every path, skipping the three-way check for
	// ALL platform_managed files and wholesale-overwriting every consumer hand-
	// edit, which is exactly the silent-clobber data loss this feature exists
	// to prevent (and the store would then be rewritten fresh, erasing the
	// prior origins). The error carries originhash's remediation hint (remove
	// origin-hashes.json to re-bootstrap), so the operator has a clear unblock.
	priorOrigin, err := originhash.Read(opts.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("load origin-hash store: %w", err)
	}

	// 2. PLAN all outcomes before any write. A fail-closed unclassified path or a
	//    malformed armed instance aborts here, before the live tree is touched.
	planned := make([]FileOutcome, 0, len(staged))
	for _, rel := range staged {
		outcome, pErr := planOutcome(opts, rel, priorOrigin)
		if pErr != nil {
			return nil, fmt.Errorf("plan %q: %w", rel, pErr)
		}
		planned = append(planned, outcome)
	}

	report := &ApplyReport{
		StagingDir:   opts.StagingDir,
		RendererName: r.Name(),
		Outcomes:     planned,
	}
	// Proposals are determined at plan time (planArmed), so they are known
	// without executing — collect them up front so a dry-run can report them.
	for i := range planned {
		if planned[i].Action == ActionArmedProposal {
			report.Proposals = append(report.Proposals, planned[i].Proposals...)
		}
	}

	// Normalize the additive typed write-state for every planned outcome. Every
	// outcome starts at WriteNotAttempted (the plan is pre-execution; even the
	// managed-overwrite/seed/armed-merge outcomes have NOT written yet). The
	// zero value "" (from a freshly-built outcome) is normalized here so
	// provenance consumers never see an untyped state. executeOutcome and
	// writeArmedManaged flip it to WriteSucceeded / WriteFailed during EXECUTE.
	for i := range planned {
		if planned[i].WriteState == "" {
			planned[i].WriteState = WriteNotAttempted
		}
	}

	// Dry-run stops here: the plan (report.Outcomes/Proposals) is the preview;
	// nothing is written and lineage is left untouched. GenerationFullyApplied is
	// true on a dry-run because a dry-run executes no write (so none can fail):
	// the field describes the EXECUTED generation, and a dry-run executed
	// nothing. (At this point every outcome is still WriteNotAttempted, so this
	// is the constant true; spelled explicitly rather than via anyWriteFailed to
	// state the invariant directly.)
	report.GenerationFullyApplied = true
	if opts.DryRun {
		return report, nil
	}

	// 3. EXECUTE writes from the plan. Owned/armed files are only ever written
	//    with their final value (never transiently clobbered then restored).
	//    Execution CONTINUES across per-file live-write failures: a failed write
	//    is recorded as WriteState=WriteFailed on that outcome (writeArmedManaged)
	//    and execution proceeds to the next file. This makes partial application
	//    a real, observable state (some files wrote, some did not) rather than
	//    aborting the whole generation at the first failed write. The
	//    generation-level completeness check immediately below is what gates
	//    lineage on it.
	for i := range planned {
		executeOutcome(opts, &planned[i])
	}

	// 3a. Generation-level completeness (P1-SUBSTRATE-001). A generation is
	//     FULLY APPLIED iff NO outcome reports WriteState=WriteFailed. When any
	//     live write failed, the generation did not fully apply and lineage MUST
	//     NOT advance for it — this is the load-bearing provenance property: a
	//     lineage record is the authority for "the last SUCCESSFUL render", so
	//     it must never claim a generation whose writes did not all land. Apply
	//     leaves the prior lineage record byte-for-byte intact (writes nothing),
	//     keeps LineagePath "", records GenerationFullyApplied=false on the
	//     report, and returns normally (nil error). A partial application is a
	//     distinct, recoverable state from the hard failures (walk/plan/
	//     lineage-write) that return an error.
	//
	//     Downstream gating is deliberately ASYMMETRIC: lineage advance (here,
	//     in Apply) and the rendered-outputs manifest persist (in the seam, the
	//     record correlated with lineage's render id) both gate on this signal.
	//     The seam's OTHER post-apply side effects (proposal ledger, run-shape
	//     seed, AGENTS.md compose, agent-model seed) are INTENTIONALLY best-
	//     effort and run regardless: they are idempotent/diagnostic and do not
	//     claim generation-level success. Reuses the typed FileOutcome.WriteState
	//     (v1.1) — there is no parallel mechanism.
	report.GenerationFullyApplied = !anyWriteFailed(planned)
	if !report.GenerationFullyApplied {
		return report, nil
	}

	// 3b. Record origin hashes for this generation (the three-way divergence
	//     input for the NEXT apply). This is the harness's port of hermes's
	//     skills_sync origin-hash mechanism. For every platform_managed file the
	//     platform WROTE this generation (an overwrite that succeeded, or a noop
	//     where on-disk already matched staged), record origin = hash(staged).
	//     For a diverged/skip outcome the platform did NOT write, so carry
	//     forward the prior origin hash (the consumer's edit stands; the last
	//     platform-written version stays the divergence baseline). A
	//     platform_managed path NOT in this generation (upstream-removed) is
	//     dropped from the store (de-manifested) but its on-disk copy is left
	//     untouched — mirrors hermes :869-870 (update + record) and
	//     :862-867/:918-921 (skip + preserve entry / drop entry without deleting).
	//     Persisted under the same GenerationFullyApplied gate as lineage.yml so
	//     the store never claims a generation whose writes did not all land.
	newOrigin := originhash.New()
	for i := range planned {
		o := &planned[i]
		if o.Class != ownership.ClassPlatformManaged {
			continue
		}
		switch o.Action {
		case ActionManagedOverwrite:
			// WriteSucceeded is guaranteed here (the gate above returned early on
			// any WriteFailed); guard defensively regardless.
			if o.WriteState == WriteSucceeded {
				if h, err := hashSHA256(filepath.Join(opts.StagingDir, o.Path)); err == nil {
					newOrigin.OriginHashes[o.Path] = h
				}
			}
		case ActionManagedNoop:
			// On-disk already matches staged byte-for-byte; record the staged hash.
			if h, err := hashSHA256(filepath.Join(opts.StagingDir, o.Path)); err == nil {
				newOrigin.OriginHashes[o.Path] = h
			}
		case ActionManagedDiverged:
			// Platform did not write; carry forward the prior origin so the next
			// apply still detects the consumer's edit/deletion as a divergence.
			if h, ok := priorOrigin.Lookup(o.Path); ok {
				newOrigin.OriginHashes[o.Path] = h
			}
		}
	}
	if err := newOrigin.Write(opts.ProjectRoot); err != nil {
		return report, fmt.Errorf("write origin-hash store: %w", err)
	}

	// 4. WRITE lineage (D3-B). lineage.yml is the S1 authority. (The renderer
	//    records its own identity via Render.RenderedBy; the Go-native renderer
	//    carries the harness/bundled-template version in Template.Ref.)
	lin := lineage.Seed(opts.TemplateSource, opts.Answers, opts.HarnessVersion)
	lin.Template.Commit = opts.Commit
	lin.Template.Ref = opts.Ref
	lin.Render.RenderedBy = r.Name()
	// Idempotent lineage: when nothing meaningful changed (same content-addressed
	// update id = same answers + same harness version), keep the PRIOR render
	// timestamp instead of stamping time.Now(). Otherwise a no-op `update` would
	// rewrite last_successful_render_at every run and churn lineage.yml in git.
	// A new update id (answers or version changed) does stamp a fresh time.
	if prev, perr := lineage.Read(opts.ProjectRoot); perr == nil && prev != nil &&
		prev.Render.LastSuccessfulUpdateID == lin.Render.LastSuccessfulUpdateID {
		lin.Render.LastSuccessfulRenderAt = prev.Render.LastSuccessfulRenderAt
	}
	if err := lin.Write(opts.ProjectRoot); err != nil {
		return report, fmt.Errorf("write lineage: %w", err)
	}
	report.LineagePath = lineage.FilePath(opts.ProjectRoot)
	return report, nil
}

// walkStaged returns the sorted list of repo-relative staged file paths under
// staging.
func walkStaged(stagingDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(stagingDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(stagingDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out, err
}

// planOutcome computes the FileOutcome for one staged path WITHOUT writing. It
// classifies via S2 and, for armed files, runs the schema reconcile so a clean
// merge value or a proposal is known up front. A fail-closed unclassified path
// or a malformed armed instance returns an error (aborts before any write).
//
// Ownership routing: this switch is the AUTHORITATIVE overwrite decision for a
// seam apply. The overwrite route (ActionManagedOverwrite) is reserved for the
// platform-overwritable classes — platform_managed (generic force-overwrite; the
// single class ownership.IsMutableByGenericRender is true for) and overlay_extension
// (overlay-system overwrite when the pack is active). Every other class is
// preserved, seeded-once, schema-reconciled/proposed, or off-path. Only those two
// classes reach ActionManagedOverwrite, so the live ownership lattice is the
// single authority for which paths a plain apply may clobber.
// ownership.IsOverwritableBySeamApply documents this same class-set (and is pinned by
// its own test) but is NOT called here — this switch is the live gate, not the
// predicate.
func planOutcome(opts ApplyOptions, rel string, priorOrigin *originhash.Store) (FileOutcome, error) {
	cls, err := opts.Classifier.MustClassify(rel)
	if err != nil {
		return FileOutcome{}, err
	}
	stagedPath := filepath.Join(opts.StagingDir, rel)
	livePath := filepath.Join(opts.ProjectRoot, rel)

	switch cls.Class {
	case ownership.ClassPlatformManaged:
		// Three-way origin-hash divergence check (port of hermes skills_sync),
		// routed through the SHARED managedfile.ClassifyPreserved decision so
		// update (here) and doctor's lint path reach the SAME preserved-vs-
		// genuine verdict from identical inputs. SKIPPED for platform-
		// regenerated paths (ApplyOptions.RegeneratedPlatformPaths): those are
		// generated canonically each apply and must stay byte-in-sync with the
		// platform's emission, so a consumer edit is always overwritten (never
		// preserved as "diverged").
		//
		// F6 (adoption migration): when a platform_managed path has NO recorded
		// origin (hadOrigin == false) and an EXISTING live file, the disposition
		// is UnknownBaseline — preserve/stall, never clobber. Only an ABSENT live
		// file with no origin is a safe bootstrap (seed). See managedfile.UnknownBaseline.
		regenerated := opts.RegeneratedPlatformPaths[rel]
		origin, hadOrigin := priorOrigin.Lookup(rel)
		// Build the live-file observation for every path that could enter the
		// classification: non-regenerated paths always do; regenerated paths
		// with no origin also do (so the migration gate can fire if a
		// regenerated-flagged path somehow has no origin, though
		// effectiveRegeneratedPaths already excludes those). A regenerated path
		// WITH an origin skips the live read entirely (it is always overwritten,
		// so the live observation would be wasted IO).
		if !regenerated || !hadOrigin {
			live := buildLiveState(livePath)
			// stagedHash is consulted ONLY to distinguish a genuine consumer
			// edit from a partial-failure self-heal (live already == what the
			// platform would write). Compute it lazily: only when the live file
			// is a readable regular file that diverged from its origin. The
			// !hadOrigin (UnknownBaseline) branch never reaches the self-heal
			// check, so stagedHash stays "" there (no wasted IO).
			var stagedHash string
			if hadOrigin && live.IsRegular && live.Readable && live.Hash != origin {
				if h, sErr := hashSHA256(stagedPath); sErr == nil {
					stagedHash = h
				}
			}
			if reason := managedfile.ClassifyPreserved(regenerated, hadOrigin, origin, live, stagedHash); reason != "" {
				return FileOutcome{Path: rel, Class: cls.Class, Action: ActionManagedDiverged,
					PreservedReason: reason, Note: noteForPreservedReason(reason)}, nil
			}
		}
		// IsMutableByGenericRender(ClassPlatformManaged) == true: the generic
		// force-overwrite class. A plain re-render overwrites it wholesale —
		// UNLESS the live instance is already byte-identical to the freshly
		// re-rendered corpus, in which case we route to ActionManagedNoop
		// (no write, reported as managed-unchanged) so the summary can
		// distinguish real churn from a no-op refresh. An absent live file
		// with no prior origin (first install / bootstrap) still seeds/overwrites.
		if managedUpToDate(stagedPath, livePath) {
			return FileOutcome{Path: rel, Class: cls.Class, Action: ActionManagedNoop,
				Note: "platform_managed already up to date"}, nil
		}
		return FileOutcome{Path: rel, Class: cls.Class, Action: ActionManagedOverwrite}, nil

	case ownership.ClassProjectOwned:
		if fileExists(livePath) {
			return FileOutcome{Path: rel, Class: cls.Class, Action: ActionProjectPreserved,
				Note: "project_owned present; preserved (never clobbered on update)"}, nil
		}
		return FileOutcome{Path: rel, Class: cls.Class, Action: ActionProjectSeeded,
			Note: "project_owned absent; seeded once from platform default"}, nil

	case ownership.ClassPlatformArmed:
		return planArmed(rel, stagedPath, livePath, cls.Class)

	case ownership.ClassOverlayExtension:
		// Slice 4: overlay units are rendered into staging only when their pack
		// is active (selected via vh-harness-profile.yml overlays:[]). When active,
		// the unit is overwritten wholesale from staging on every update (it is
		// platform-owned content the project never hand-edits). When a pack is
		// deselected, its units are simply not staged, so Apply leaves any live
		// copy untouched (orphan-cleanup is a v0+ concern; the classifier is
		// rebuild-only, so a deselected overlay file is unclassified and would
		// fail-closed if re-introduced — acceptable v0).
		//
		// Like platform_managed, a byte-identical live instance is a no-op
		// (ActionManagedNoop / managed-unchanged) rather than a churn write.
		if managedUpToDate(stagedPath, livePath) {
			return FileOutcome{Path: rel, Class: cls.Class, Action: ActionManagedNoop,
				Note: "overlay_extension active; already up to date"}, nil
		}
		return FileOutcome{Path: rel, Class: cls.Class, Action: ActionManagedOverwrite,
			Note: "overlay_extension active; overwritten from staged overlay unit"}, nil

	case ownership.ClassExternalGenerated:
		// Slice 4: external_generated content is authored by the project (or a
		// provider), never by the platform. The harness seeds a blank skeleton
		// ONCE on first install (so the schema/contract is present), then leaves
		// the project's instance untouched on every subsequent update. This makes
		// repo-recon-data.yml seed blank then become project-owned in practice.
		if fileExists(livePath) {
			return FileOutcome{Path: rel, Class: cls.Class, Action: ActionProjectPreserved,
				Note: "external_generated present; preserved (project/provider-owned, never clobbered)"}, nil
		}
		return FileOutcome{Path: rel, Class: cls.Class, Action: ActionProjectSeeded,
			Note: "external_generated absent; blank skeleton seeded once from platform default"}, nil

	case ownership.ClassLocalOnly:
		return FileOutcome{Path: rel, Class: cls.Class, Action: ActionIgnoredLocal,
			Note: "local_only; not on the platform update path"}, nil

	default:
		return FileOutcome{}, fmt.Errorf("unsupported ownership class %q for %q", cls.Class, rel)
	}
}

// planArmed plans a platform_armed file: validate the staged default, look up the
// schema, reconcile against the project instance, and decide apply/proposal/noop.
//
// Authority split (spec: "doctor/preflight validate; update reconciles"):
//   - The STAGED DEFAULT is hard-validated. A schema-invalid platform default is a
//     platform bug -> hard error, abort before any write.
//   - The PROJECT INSTANCE is NOT hard-validated here. The reconciler is the
//     update-path decision-maker: it distinguishes genuinely malformed YAML (hard
//     error) from a clean merge (apply) from a needs-decision conflict (proposal,
//     e.g. a profile value the platform's enum has withdrawn). `doctor` is the
//     authoritative lint surface (wired in Slice 2) and uses the validator
//     directly; an enum-withdrawn value is a reconcile proposal, not an update-
//     blocking validation error.
func planArmed(rel, stagedPath, livePath string, cls ownership.Class) (FileOutcome, error) {
	sch, ok := schema.SchemaForPath(rel)
	if !ok {
		return FileOutcome{}, fmt.Errorf(
			"platform_armed file %q has no registered schema; a platform_armed path MUST be schema'd "+
				"(register it in internal/schema/registry.go)", rel)
	}
	stagedDefault, err := os.ReadFile(stagedPath)
	if err != nil {
		return FileOutcome{}, fmt.Errorf("read staged default: %w", err)
	}
	// Validate the platform's own staged default (it must be schema-conformant).
	if errs := sch.Validator.Validate(stagedDefault); len(errs) > 0 {
		return FileOutcome{}, fmt.Errorf("staged platform default for %q is schema-invalid: %v", rel, fieldErrorsString(errs))
	}
	projectInstance, _ := os.ReadFile(livePath) // absent is OK (first install/seed)
	if len(strings.TrimSpace(string(projectInstance))) == 0 {
		// First install: seed the armed file from the platform default (validated).
		return FileOutcome{Path: rel, Class: cls, Action: ActionArmedMerged,
			Applied: []string{"armed file absent; seeded from validated platform default"}}, nil
	}
	// The reconciler is the decision-maker on the update path. It returns a hard
	// error only for genuinely malformed input (unparseable YAML); everything else
	// is an apply / proposal / noop.
	res, err := sch.Reconciler.Reconcile(projectInstance, stagedDefault)
	if err != nil {
		return FileOutcome{}, fmt.Errorf("reconcile %q (run doctor to lint the project instance): %w", rel, err)
	}
	switch res.Outcome {
	case schema.OutcomeApply:
		return FileOutcome{Path: rel, Class: cls, Action: ActionArmedMerged, Applied: res.Applied}, nil
	case schema.OutcomePropose:
		return FileOutcome{Path: rel, Class: cls, Action: ActionArmedProposal, Proposals: res.Proposals,
			Note: "needs-decision; project instance left untouched"}, nil
	case schema.OutcomeNoop:
		return FileOutcome{Path: rel, Class: cls, Action: ActionArmedNoop,
			Note: "project instance already up to date"}, nil
	default:
		return FileOutcome{}, fmt.Errorf("reconcile %q returned unknown outcome %q", rel, res.Outcome)
	}
}

// executeOutcome performs the single write implied by a planned outcome. It is
// the only place the live tree is mutated, and only for managed-overwrite /
// project-seed / armed-merge actions.
func executeOutcome(opts ApplyOptions, o *FileOutcome) {
	if o.Action == ActionManagedOverwrite ||
		o.Action == ActionProjectSeeded ||
		o.Action == ActionArmedMerged {
		writeArmedManaged(opts, o)
	}
	// Preserved / diverged / proposal / noop (armed or managed) / unsupported /
	// ignored -> no write. (ActionManagedDiverged is the origin-hash three-way
	// skip: the consumer edit/deletion is preserved, never clobbered.)
}

// writeArmedManaged computes the bytes to write (copy for managed/seed; reconcile
// result for armed) and writes them exactly once into the live tree. The typed
// WriteState field is the machine-readable execution signal: WriteSucceeded on a
// completed write, WriteFailed on any staged-read/mkdir/reconcile/live-write
// error. The human-readable Note carries the error detail for diagnostics but is
// NEVER a correctness signal.
func writeArmedManaged(opts ApplyOptions, o *FileOutcome) {
	rel := o.Path
	stagedPath := filepath.Join(opts.StagingDir, rel)
	livePath := filepath.Join(opts.ProjectRoot, rel)

	var bytes []byte
	switch o.Action {
	case ActionManagedOverwrite, ActionProjectSeeded:
		b, err := os.ReadFile(stagedPath)
		if err != nil {
			o.Note = fmt.Sprintf("ERROR reading staged: %v", err)
			o.WriteState = WriteFailed
			return
		}
		bytes = b
	case ActionArmedMerged:
		// Re-derive the merged value here (plan already validated it). For the
		// absent-seed case the merged value IS the staged default.
		if len(o.Applied) == 1 && strings.Contains(o.Applied[0], "absent; seeded") {
			b, err := os.ReadFile(stagedPath)
			if err != nil {
				o.Note = fmt.Sprintf("ERROR reading staged: %v", err)
				o.WriteState = WriteFailed
				return
			}
			bytes = b
		} else {
			projectInstance, _ := os.ReadFile(livePath)
			stagedDefault, err := os.ReadFile(stagedPath)
			if err != nil {
				o.Note = fmt.Sprintf("ERROR reading staged: %v", err)
				o.WriteState = WriteFailed
				return
			}
			sch, ok := schema.SchemaForPath(rel)
			if !ok {
				// The plan side (planArmed) hard-errors on an unregistered
				// platform_armed path; the write path cannot return an error,
				// so it fails loudly through the typed signal instead.
				o.Note = fmt.Sprintf(
					"ERROR re-deriving merge: platform_armed file %q has no registered schema; "+
						"a platform_armed path MUST be schema'd (register it in internal/schema/registry.go)", rel)
				o.Action = ActionArmedProposal
				o.WriteState = WriteFailed
				return
			}
			res, err := sch.Reconciler.Reconcile(projectInstance, stagedDefault)
			if err != nil || res.Outcome != schema.OutcomeApply {
				o.Note = fmt.Sprintf("ERROR re-deriving merge: %v", err)
				o.Action = ActionArmedProposal
				o.WriteState = WriteFailed
				return
			}
			bytes = res.Merged
		}
	}

	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		o.Note = fmt.Sprintf("ERROR mkdir: %v", err)
		o.WriteState = WriteFailed
		return
	}
	if err := os.WriteFile(livePath, bytes, renderWriteMode(livePath)); err != nil {
		o.Note = fmt.Sprintf("ERROR write: %v", err)
		o.WriteState = WriteFailed
		return
	}
	o.WriteState = WriteSucceeded
}

// --- small helpers ---

// anyWriteFailed reports whether any outcome in the plan recorded a live-write
// failure (WriteState == WriteFailed). It is the generation-level completeness
// predicate Apply gates lineage on: a generation is fully applied iff no
// outcome failed. Reuses the typed FileOutcome.WriteState field (v1.1) — there
// is deliberately no parallel mechanism.
func anyWriteFailed(outcomes []FileOutcome) bool {
	for i := range outcomes {
		if outcomes[i].WriteState == WriteFailed {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// managedUpToDate reports whether the live instance of a platform_managed /
// active overlay_extension file is byte-identical to the freshly staged corpus
// copy. When true, planOutcome routes to ActionManagedNoop (no write, reported
// as managed-unchanged) instead of ActionManagedOverwrite, so the update
// summary distinguishes real churn from a no-op refresh. An absent live file is
// NOT up to date (first install still seeds/overwrites). A read error is
// treated as not-up-to-date so the safe default is to overwrite.
func managedUpToDate(stagedPath, livePath string) bool {
	if !fileExists(livePath) {
		return false
	}
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		return false
	}
	live, err := os.ReadFile(livePath)
	if err != nil {
		return false
	}
	return bytes.Equal(staged, live)
}

// hashSHA256 reads p and returns its "sha256:<hex>" digest. It is the single
// hash representation the origin-hash three-way check (planOutcome live file)
// and the post-generation origin-hash recording (staged file) use, routed
// through originhash.Digest so the format is identical to the persisted store
// and never drifts from it. A read error is surfaced to the caller: in the
// three-way check, a live-hash read error (e.g. write-permitted-but-not-read)
// with a prior origin routes to ActionManagedDiverged (preserved, never
// clobbered) rather than falling through to an overwrite — see planOutcome.
func hashSHA256(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return originhash.Digest(data), nil
}

// readLiveFile is the SEAM over the live-tree read used by the origin-hash
// three-way check (buildLiveState). Production uses os.ReadFile; unit tests
// override it to inject a DETERMINISTIC read failure (F8) — proving the
// unreadable-file safety path without relying on OS permission bits, which
// skip under root / permissive filesystems (the pre-F8 test's t.Skip hole).
// It is package-scoped (not exported) so ONLY the managed-file three-way live
// read goes through it; staged reads (hashSHA256 of stagedPath) and the
// managedUpToDate comparison keep using os.ReadFile directly. Tests that
// override it MUST restore the default (readLiveFile = os.ReadFile) on cleanup.
var readLiveFile = os.ReadFile

// buildLiveState observes a live managed file (stat + read via the readLiveFile
// seam + hash) and returns the managedfile.LiveState ClassifyPreserved consumes.
// It is the single place the origin-hash three-way check touches the live
// filesystem, kept IO-isolated so the classifier stays a pure, unit-testable
// function. See managedfile.LiveState for the field construction contract.
func buildLiveState(livePath string) managedfile.LiveState {
	info, err := os.Stat(livePath)
	if os.IsNotExist(err) {
		return managedfile.LiveState{Absent: true}
	}
	if err != nil || info.IsDir() {
		// A directory at a managed path, or a stat error other than NotExist:
		// the zero LiveState (NOT a preserved state). The caller falls through
		// to the write path, where a blocked write surfaces as WriteFailed
		// rather than being hidden as a "respected deletion" — preserving the
		// partial-failure semantics that a blocked write is reported.
		return managedfile.LiveState{}
	}
	ls := managedfile.LiveState{IsRegular: true}
	if data, rerr := readLiveFile(livePath); rerr == nil {
		ls.Readable = true
		ls.Hash = originhash.Digest(data)
	}
	// rerr != nil (e.g. write-permitted-but-not-readable, or an injected F8
	// failure): Readable stays false, Hash "". ClassifyPreserved classifies a
	// stat-able regular file it cannot read as Unreadable (preserve, never
	// clobber a possible edit the read could not inspect).
	return ls
}

// noteForPreservedReason maps a typed preserved reason to the human-readable
// diagnostics Note carried on the FileOutcome. The Note is diagnostics ONLY
// (never a correctness signal — the typed FileOutcome.PreservedReason field is
// the correctness signal). The strings are kept byte-stable with the pre-refactor
// values so existing Note-substring assertions and human readers are unaffected.
func noteForPreservedReason(r managedfile.PreservedReason) string {
	switch r {
	case managedfile.ConsumerDelete:
		return "platform_managed previously rendered; absent on disk (consumer-deleted); not re-seeded"
	case managedfile.ConsumerEdit:
		return "platform_managed diverged from origin hash (consumer-modified); preserved (not clobbered)"
	case managedfile.Unreadable:
		return "platform_managed live file unreadable (cannot confirm consumer edit); preserved (not clobbered)"
	case managedfile.UnknownBaseline:
		return "platform_managed existing file with no recorded origin (migration baseline unknown); preserved (not clobbered) — awaiting baseline resolution"
	default:
		return ""
	}
}

func fieldErrorsString(errs []schema.FieldError) string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return strings.Join(out, "; ")
}
