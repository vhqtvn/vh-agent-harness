package substrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
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
}

// FileAction labels what the seam did to one staged file. It is the machine
// label the ApplyReport carries per file.
type FileAction string

const (
	ActionManagedOverwrite FileAction = "managed-overwrite"   // platform_managed -> overwrite
	ActionProjectPreserved FileAction = "project-preserved"   // project_owned present -> skip
	ActionProjectSeeded    FileAction = "project-seeded"      // project_owned absent -> seed once
	ActionArmedMerged      FileAction = "armed-merged"        // platform_armed clean reconcile applied
	ActionArmedProposal    FileAction = "armed-proposal"      // platform_armed needs-decision; not written
	ActionArmedNoop        FileAction = "armed-noop"          // platform_armed already up to date
	ActionUnsupportedClass FileAction = "skipped-unsupported" // reserved for future off-lattice classes (overlay_extension/external_generated now implemented in Slice 4)
	ActionIgnoredLocal     FileAction = "ignored-local-only"
)

// FileOutcome is the seam's per-file result.
type FileOutcome struct {
	Path      string
	Class     ownership.Class
	Action    FileAction
	Applied   []string          // human-readable merge notes (armed-merged)
	Proposals []schema.Proposal // populated when Action == ActionArmedProposal
	Note      string            // extra context (e.g. why skipped)
}

// ApplyReport is the seam's result.
type ApplyReport struct {
	Outcomes     []FileOutcome
	StagingDir   string
	LineagePath  string // absolute path to the written lineage.yml
	Proposals    []schema.Proposal
	RendererName string
}

// Apply runs the seam: it walks the staged tree, classifies every candidate via
// S2, plans all per-class outcomes (validating fail-closed BEFORE any write so a
// mis-authored ownership map aborts without touching the live tree), then
// executes the writes. Finally it writes the D3-B lineage file.
//
// Atomicity contract: the live tree is never churned. The render happened in
// staging (a scratch directory), never in ProjectRoot. project_owned files are
// never overwritten when present (preserved) and seeded at most once when absent.
// platform_armed files are overwritten only with a clean schema-reconciled value;
// a needs-decision conflict leaves the project instance untouched (a proposal is
// emitted instead). A fail-closed unclassified path aborts before any write.
func Apply(r Renderer, opts ApplyOptions) (*ApplyReport, error) {
	// 1. Enumerate staged candidate files (sorted, deterministic).
	staged, err := walkStaged(opts.StagingDir)
	if err != nil {
		return nil, fmt.Errorf("walk staging: %w", err)
	}

	// 2. PLAN all outcomes before any write. A fail-closed unclassified path or a
	//    malformed armed instance aborts here, before the live tree is touched.
	planned := make([]FileOutcome, 0, len(staged))
	for _, rel := range staged {
		outcome, pErr := planOutcome(opts, rel)
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

	// Dry-run stops here: the plan (report.Outcomes/Proposals) is the preview;
	// nothing is written and lineage is left untouched.
	if opts.DryRun {
		return report, nil
	}

	// 3. EXECUTE writes from the plan. Owned/armed files are only ever written
	//    with their final value (never transiently clobbered then restored).
	for i := range planned {
		executeOutcome(opts, &planned[i])
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
// Slice 5.1 ownership gate: the overwrite route (ActionManagedOverwrite) is
// reserved for ownership.IsPlatformOverwritable classes — platform_managed
// (generic force-overwrite; ownership.IsMutableByPlatform) and overlay_extension
// (overlay-system overwrite when the pack is active). Every other class is
// preserved, seeded-once, schema-reconciled/proposed, or off-path. The per-class
// switch below is the concrete routing; only those two classes reach
// ActionManagedOverwrite, so the live ownership lattice is the single authority
// for which paths a plain apply may clobber.
func planOutcome(opts ApplyOptions, rel string) (FileOutcome, error) {
	cls, err := opts.Classifier.MustClassify(rel)
	if err != nil {
		return FileOutcome{}, err
	}
	stagedPath := filepath.Join(opts.StagingDir, rel)
	livePath := filepath.Join(opts.ProjectRoot, rel)

	switch cls.Class {
	case ownership.ClassPlatformManaged:
		// IsMutableByPlatform(ClassPlatformManaged) == true: the generic
		// force-overwrite class. A plain re-render overwrites it wholesale.
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
	// Preserved / proposal / noop / unsupported / ignored -> no write.
}

// writeArmedManaged computes the bytes to write (copy for managed/seed; reconcile
// result for armed) and writes them exactly once into the live tree.
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
				return
			}
			bytes = b
		} else {
			projectInstance, _ := os.ReadFile(livePath)
			stagedDefault, err := os.ReadFile(stagedPath)
			if err != nil {
				o.Note = fmt.Sprintf("ERROR reading staged: %v", err)
				return
			}
			sch, _ := schema.SchemaForPath(rel)
			res, err := sch.Reconciler.Reconcile(projectInstance, stagedDefault)
			if err != nil || res.Outcome != schema.OutcomeApply {
				o.Note = fmt.Sprintf("ERROR re-deriving merge: %v", err)
				o.Action = ActionArmedProposal
				return
			}
			bytes = res.Merged
		}
	}

	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		o.Note = fmt.Sprintf("ERROR mkdir: %v", err)
		return
	}
	if err := os.WriteFile(livePath, bytes, renderWriteMode(livePath)); err != nil {
		o.Note = fmt.Sprintf("ERROR write: %v", err)
		return
	}
}

// --- small helpers ---

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func fieldErrorsString(errs []schema.FieldError) string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return strings.Join(out, "; ")
}
