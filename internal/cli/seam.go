package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
	"github.com/vhqtvn/vh-agent-harness/internal/proposals"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// This file is the Slice-2 bridge between the Cobra CLI verbs (install / update
// / doctor) and the validated substrate seam (internal/substrate). It centralizes
// the three seam inputs that are constant for the embedded core corpus so the
// verbs stay thin:
//
//   - the embedded corpus (CoreFS) exposed as an fs.FS via fs.Sub;
//   - the S2 ownership classifier built from corpus.CoreOwnershipDefaults;
//   - a single seamApply helper that renders into an out-of-tree staging dir and
//     runs substrate.Apply (classify -> plan per-class -> execute -> write lineage).
//
// The CLI verbs are deliberately thin: they resolve a target + answers and then
// call seamApply. Everything below (renderer, classifier, lineage, schema
// reconcile, ownership raise-only enforcement) is the seam's responsibility and
// already unit-tested in internal/substrate.
//
// Coexistence note: this is the SEAM install path and, since the legacy
// render/installer/component path was retired, the ONLY install source. The
// remaining legacy read surfaces (loadManifest / drift.Compute, used by
// uninstall/preflight/diff/status) read on-disk manifests for back-compat with
// pre-seam installs but no command writes them anymore — the seam writes
// lineage (.vh-agent-harness/lineage.yml) instead. Verbs that need lineage
// authority call seamApply; verbs that only need to read a tracked-file
// manifest call loadManifest.

// coreSubFS returns the embedded curated corpus as an fs.FS rooted at the corpus
// root (templates/core). The embed.FS sub-tree is materialized at compile time,
// so fs.Sub cannot fail at runtime unless the embed directive itself is broken;
// a panic here is the correct fail-loud response (the binary is unusable).
//
// It is memoized via coreSubFSOnce so the corpus is walked exactly once per
// process. The EmbedFSRenderer re-reads files from this fs.FS on every render,
// which is cheap (embed bytes are already in memory).
var (
	coreSubFSErr error
	coreSubFSVal fs.FS
)

func coreSubFSImpl() (fs.FS, error) {
	if coreSubFSVal != nil || coreSubFSErr != nil {
		return coreSubFSVal, coreSubFSErr
	}
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		coreSubFSErr = fmt.Errorf("seam: embed sub %q: %w", corpus.CoreDir, err)
		return nil, coreSubFSErr
	}
	coreSubFSVal = sub
	return coreSubFSVal, nil
}

// seamClassifier builds the seam's read view over S2 ownership for the core
// corpus: CoreOwnershipDefaults (every path platform_managed except the two
// documented exceptions) -> ownership.Resolve (raise-only) -> substrate
// Classifier (exact-path map, fail-closed for anything off-map). The classifier
// is memoized because the ownership map is a constant of the embedded corpus.
var (
	seamClassifierErr error
	seamClassifierVal *substrate.Classifier
)

func seamClassifierImpl() (*substrate.Classifier, error) {
	if seamClassifierVal != nil || seamClassifierErr != nil {
		return seamClassifierVal, seamClassifierErr
	}
	defaults, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		seamClassifierErr = fmt.Errorf("seam: core ownership: %w", err)
		return nil, seamClassifierErr
	}
	eff, err := ownership.Resolve(defaults, nil)
	if err != nil {
		seamClassifierErr = fmt.Errorf("seam: ownership resolve: %w", err)
		return nil, seamClassifierErr
	}
	seamClassifierVal = substrate.NewClassifier(eff, nil)
	return seamClassifierVal, nil
}

// seamApply renders the embedded core corpus into an OUT-OF-TREE staging
// directory and runs the full seam apply (classify -> plan all outcomes
// fail-closed before any write -> per-class execute -> write lineage.yml). It is
// the single entry point both install and update use: install runs it against an
// empty target (every managed file seeded/overwritten, armed files seeded from
// the platform default, owned files seeded once), update runs it against an
// existing tree (managed files refreshed, owned files preserved byte-for-byte
// when present, armed files schema-reconciled or turned into a proposal).
//
// Staging MUST live outside the target tree: if staging were under <target>,
// walkStaged would classify the .vh-agent-harness/.staging/* paths and, because
// they are off the S2 ownership map, the fail-closed classifier would abort the
// whole apply before touching the live tree. Using os.MkdirTemp("", ...) keeps
// staging on the host temp dir, outside the project root, and it is removed on
// completion regardless of outcome.
//
// Ref is the content origin tag lineage.yml carries (spec: the harness carries
// its bundled-template version in Template.Ref). HarnessVersion feeds the
// lineage update id (content-addressed render identity).
//
// The live S3 vh-harness-profile.yml at <target>/vh-harness-profile.yml is the
// feature-surface authority: its features (v1: backlog) and overlays selections
// drive render conditionals ({{ if .features.backlog }}) and Slice-4 overlay
// selection. seamApply projects the profile onto render answers and merges them
// with the caller-supplied answers, profile-wins for feature/overlay keys. On
// first install the live profile is absent (it is seeded FROM the platform
// default during apply), so render decisions fall back to defaults.
func seamApply(target string, answers map[string]string, dryRun bool) (*substrate.ApplyReport, error) {
	sub, err := coreSubFSImpl()
	if err != nil {
		return nil, err
	}

	// Merge the live profile's feature/overlay answers over the caller answers.
	// Profile wins for the keys it owns (features.*, overlays); project_name /
	// project_slug come from the caller and are never overwritten.
	renderAnswers := mergeRenderAnswers(answers, readProfileAnswers(target))

	staging, err := os.MkdirTemp("", "harness-seam-staging-*")
	if err != nil {
		return nil, fmt.Errorf("seam: create staging: %w", err)
	}
	defer os.RemoveAll(staging)

	renderer := substrate.EmbedFSRenderer{Source: sub}
	// Render the core corpus + active overlay packs (selected via the live S3
	// profile) and perform the overlay merges (opencode-append deep-merge,
	// callable-graph append). Returns the LIVE .opencode-relative paths the
	// overlays contributed so the classifier can mark them overlay_extension.
	overlayFiles, err := renderSeamStaging(staging, renderer, renderAnswers, target)
	if err != nil {
		return nil, err
	}

	// Migration courtesy check (O5 slice 2c, Q5c): allowed-commands.js is now
	// generated from Go canonical tables (internal/permconfig/tables.go). If the
	// LIVE file differs from the generated form, it was either customized by the
	// operator (adding/removing commands in the readonly/git_readonly/gate
	// groups) or is a prior harness version. Either way, the canonical overwrite
	// would discard those changes — warn loudly so the operator can back up the
	// file or port custom deny-rules to forbidden-patterns.project.js. This is a
	// WARNING, not a refusal: the file is platform_managed (free-overwrite by
	// contract), and refusing would block legitimate version upgrades whenever
	// the Go tables change.
	if !dryRun {
		warnIfAllowedCommandsCustomized(target, staging)
	}

	// The classifier is per-apply: core defaults PLUS overlay_extension rules for
	// every path the active overlays rendered, resolved against the project's S2
	// raise-only overrides (harness-ownership.yml). A downgrade override (or any
	// other D2-A violation) aborts the apply here, before any write touches the
	// live tree (Slice 5.1 live wiring). (The memoized seamClassifierImpl is
	// core-only and cannot see overlay paths or project overrides — fail-closed
	// would abort.)
	overrides, oerr := readOwnershipOverrides(target)
	if oerr != nil {
		return nil, fmt.Errorf("seam: ownership overrides: %w", oerr)
	}
	cls, err := seamClassifierWithOverlays(overlayFiles, overrides)
	if err != nil {
		return nil, err
	}

	// Lineage (S1) records the INSTALL answers (project_name/slug) for the
	// answer-digest drift check; the S3 profile (features/overlays) is a separate
	// authority and must NOT enter the install-answer digest (else install→update
	// would false-flag answer drift the moment the profile exists). Render used the
	// merged answers above; Apply records the original install answers below.
	report, err := substrate.Apply(renderer, substrate.ApplyOptions{
		ProjectRoot:    target,
		StagingDir:     staging,
		Classifier:     cls,
		HarnessVersion: Version,
		TemplateSource: corpus.CoreDir,
		Ref:            "harness/" + Version,
		Answers:        answers,
		DryRun:         dryRun,
	})
	if err != nil {
		return nil, fmt.Errorf("seam: apply: %w", err)
	}

	// Dry-run: substrate.Apply wrote nothing (it returned the plan only). Skip
	// every side-effecting post-apply step too — the proposal ledger append, the
	// run-shape seed, and the AGENTS.md compose all WRITE to the live tree. The
	// returned report is a pure preview.
	if dryRun {
		return report, nil
	}

	// Slice 5.3: record every armed-file conflict the apply surfaced but did not
	// auto-resolve (ActionArmedProposal outcomes) into the proposal ledger
	// (.vh-agent-harness/proposals.jsonl). The live instance is left untouched
	// (apply wrote nothing for it); the ledger is the durable, reviewable record
	// an operator lists via `vh-agent-harness proposals`. Append-only across updates.
	if n := len(report.Proposals); n > 0 {
		records := make([]proposals.Record, 0, n)
		for _, o := range report.Outcomes {
			if o.Action == substrate.ActionArmedProposal && len(o.Proposals) > 0 {
				records = append(records, proposals.Record{
					Path:      o.Path,
					Class:     string(o.Class),
					Proposals: o.Proposals,
				})
			}
		}
		if _, lerr := proposals.Append(target, "harness/"+Version, records); lerr != nil {
			// A ledger write failure does not undo a successful apply; surface it
			// on stderr so the operator knows the conflict was not recorded.
			fmt.Fprintf(os.Stderr, "seam: warning: proposal ledger write failed: %v\n", lerr)
		}
	}

	// Seed the S4 runtime shape (the config-authority model S4). The runtime verbs
	// (exec/shell/up/down/logs/ps) read .vh-agent-harness/run-shape.yml FIRST to
	// resolve the backend. Without a seeded run-shape, a fresh seam install
	// resolves no backend and the verbs are inert. The seed is the documented
	// "platform seeds a minimal default on first install and refuses to clobber"
	// behavior: host-shell is the safe, no-container default (works for any
	// repo, including web-less). It is written ONLY when the project instance is
	// absent (S4 is project_owned); an existing run-shape is never overwritten.
	if err := seedRunShapeDefault(target); err != nil {
		// A seed failure does not undo a successful apply; surface it so the
		// operator knows the runtime verbs may be inert until run-shape exists.
		fmt.Fprintf(os.Stderr, "seam: warning: run-shape seed failed: %v\n", err)
	}

	// Compose AGENTS.md from the managed generic rules (AGENTS.core.md) + the
	// project's domain mission (AGENTS.mission.md), when the project has adopted
	// the split. Opt-in and backward-safe (see composeAgentsMd).
	if err := composeAgentsMd(target); err != nil {
		fmt.Fprintf(os.Stderr, "seam: warning: AGENTS.md compose failed: %v\n", err)
	}

	// Seed the per-agent model files referenced by opencode.jsonc
	// ({file:./.local/config/agent-model/<agent>}). They are gitignored,
	// operator-managed: seeded EMPTY here so OpenCode finds the files (a missing
	// {file:} ref breaks config load), then the operator fills in a model id.
	// doctor warns while they are empty.
	if err := seedAgentModelDefaults(target); err != nil {
		fmt.Fprintf(os.Stderr, "seam: warning: agent-model seed failed: %v\n", err)
	}
	return report, nil
}

// agentModelRefRe matches the per-agent model-file references opencode.jsonc
// carries. The `[^}<]` class skips the literal `<name>` example in the file's
// header comment.
var agentModelRefRe = regexp.MustCompile(`\{file:\./(\.local/config/agent-model/[^}<]+)\}`)

// seedAgentModelDefaults creates each .local/config/agent-model/<agent> file
// referenced by the rendered opencode.jsonc that does not yet exist, as an EMPTY
// file. These are project-local, gitignored, operator-managed model selections;
// seeding them empty keeps OpenCode's config load working (no missing-file
// error) while leaving the actual model choice to the operator. Existing files
// are never overwritten.
func seedAgentModelDefaults(target string) error {
	data, err := os.ReadFile(filepath.Join(target, "opencode.jsonc"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read opencode.jsonc: %w", err)
	}
	seen := map[string]bool{}
	for _, m := range agentModelRefRe.FindAllSubmatch(data, -1) {
		rel := string(m[1])
		if seen[rel] {
			continue
		}
		seen[rel] = true
		p := filepath.Join(target, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			continue // operator's file present — never overwrite
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// composeAgentsMd assembles the agent-facing <target>/AGENTS.md as the managed
// generic rules (.vh-agent-harness/AGENTS.core.md) followed by the project's
// domain mission (.vh-agent-harness/AGENTS.mission.md), implementing the
// documented "core first, then mission" composition.
//
// The two SOURCE files live under .vh-agent-harness/ (the config space), NOT at
// the repo root — only the composed AGENTS.md is agent-facing, so an agent
// exploring the repo never reads the half-documents as authoritative and sees
// duplicated rules. The seam concatenates the sources into the single root
// AGENTS.md that opencode loads.
//
// It is OPT-IN and backward-safe: it runs ONLY when the project supplies
// .vh-agent-harness/AGENTS.mission.md. A project that has not adopted the split
// (no mission source) keeps its own root AGENTS.md untouched — the seam never
// clobbers a hand-authored AGENTS.md. When a mission source is present, AGENTS.md
// becomes a generated artifact regenerated on every install/update, so its
// generic half always tracks the platform while the mission half stays
// project-owned.
func composeAgentsMd(target string) error {
	srcDir := filepath.Join(target, runshape.DirName)
	mission, err := os.ReadFile(filepath.Join(srcDir, "AGENTS.mission.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // opt-in: no mission source → leave AGENTS.md alone
		}
		return fmt.Errorf("read .vh-agent-harness/AGENTS.mission.md: %w", err)
	}
	core, err := os.ReadFile(filepath.Join(srcDir, "AGENTS.core.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no generic core to compose with
		}
		return fmt.Errorf("read .vh-agent-harness/AGENTS.core.md: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(bytes.TrimRight(core, "\n"))
	buf.WriteString("\n\n")
	buf.Write(mission)
	if err := os.WriteFile(filepath.Join(target, "AGENTS.md"), buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write composed AGENTS.md: %w", err)
	}
	return nil
}

// defaultRunShapeSeed is the minimal, schema-valid run-shape the seam seeds on
// first install. host-shell is the safe no-container default (the web-less
// example in the run-shape spec §6a); the project edits it to docker_compose
// when it adopts a container runtime. Every lifecycle point is an explicit
// no-op (absent = no-op already, but spelling them out documents the surface).
const defaultRunShapeSeed = `run_shape_version: "0.1"
# S4 runtime shape (.vh-agent-harness/run-shape.yml). Project-owned: edit freely;
# ` + "`vh-agent-harness update`" + ` never overwrites this file. See the run-shape spec.
runtime:
  backend: host-shell
# services: {}      # none for host-shell; declare for docker_compose
# lifecycle: {}     # hooks are scripts/ pointers; absent = no-op
# runners: {}
# verbs: {}
`

// seedRunShapeDefault writes the default run-shape.yml at
// <target>/.vh-agent-harness/run-shape.yml when (and only when) it is absent.
// The .vh-agent-harness/ directory already exists at this point (substrate.Apply
// wrote lineage.yml into it). A present file is preserved byte-for-byte (S4 is
// project_owned).
func seedRunShapeDefault(target string) error {
	rsPath := filepath.Join(target, runshape.DirName, runshape.FileName)
	if _, err := os.Stat(rsPath); err == nil {
		return nil // project instance present — never clobber (project_owned)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat run-shape: %w", err)
	}
	if err := os.WriteFile(rsPath, []byte(defaultRunShapeSeed), 0o644); err != nil {
		return fmt.Errorf("write run-shape seed: %w", err)
	}
	return nil
}

// renderSeamStaging renders the core corpus into staging, then renders every
// active overlay pack's unit files and performs the overlay merges. It is shared
// by seamApply (install/update) and the doctor managed-drift check so both
// render like-for-like (an overlay project's drift check must compare against an
// overlay-merged baseline, not core-only).
//
// Unknown pack names in the profile are skipped with a stderr notice rather than
// aborting the apply (a stale profile entry should not block install). Returns
// the sorted LIVE .opencode-relative paths contributed by overlays.
func renderSeamStaging(staging string, renderer substrate.Renderer, renderAnswers map[string]string, target string) ([]string, error) {
	// Fold in the project.config.json-sourced tokens (mission/architecture/db).
	// project.config.json is project_owned and read LIVE from the target so the
	// seeded CLAUDE.md/Makefile resolve {{MISSION_SUMMARY}} etc. The config keys
	// never clobber install identity (project_name/slug) — they are disjoint.
	renderAnswers = mergeRenderAnswers(renderAnswers, projectConfigAnswers(target))
	if err := renderer.Render(staging, substrate.RenderSpec{
		TemplateSource: corpus.CoreDir,
		Answers:        renderAnswers,
	}); err != nil {
		return nil, fmt.Errorf("seam: render into staging: %w", err)
	}
	// existing is the set of LIVE .opencode-relative paths already on disk in
	// staging. Core has just rendered the builtin corpus; an overlay MUST NOT
	// silently shadow any of these (Slice 3 fail-closed guard). As each pack
	// renders, its new paths are added so a later pack cannot shadow an earlier
	// pack's units either.
	existing := walkStagedLivePaths(staging)
	var overlayFiles []string
	var packs []*overlay.Pack
	for _, name := range activeOverlays(target) {
		pack, err := overlay.OpenPackFor(target, name)
		if err != nil {
			// Fail CLOSED on a profile-listed overlay that won't open/apply (W9).
			// Every overlay processed here is profile-listed — activeOverlays reads
			// ONLY vh-harness-profile.yml `overlays:`; there is NO separate
			// auto-discovered/lenient category — so the old warn-and-skip would
			// silently produce an INCOMPLETE render (missing the agents/commands/
			// skills the operator explicitly declared) with no signal. Hard-fail
			// naming the pack + the underlying error so the operator can fix the
			// pack or remove it from the profile overlays: list. Refusing the whole
			// render is correct: a partial overlay set is unpredictable state.
			return nil, fmt.Errorf("seam: overlay %q (declared in vh-harness-profile.yml overlays:) failed to open: %w\n"+
				"fix the pack or remove it from the profile overlays: list; refusing to render an incomplete overlay set",
				name, err)
		}
		// Shadowing guard (Slice 3): fail CLOSED before any unit is rendered if
		// this pack would overwrite an existing rendered path. Point the consumer
		// at the explicit S2 managed→owned replacement path.
		shadow, err := pack.DetectShadowing(existing)
		if err != nil {
			return nil, fmt.Errorf("seam: overlay %s: shadow check: %w", name, err)
		}
		if shadow != nil {
			return nil, shadow
		}
		rendered, err := pack.RenderUnits(staging, renderAnswers)
		if err != nil {
			return nil, fmt.Errorf("seam: overlay %s: %w", name, err)
		}
		overlayFiles = append(overlayFiles, rendered...)
		for _, rel := range rendered {
			existing[rel] = true
		}
		if err := pack.MergeAppend(staging); err != nil {
			return nil, fmt.Errorf("seam: overlay %s: %w", name, err)
		}
		if err := pack.AppendCallableGraph(staging); err != nil {
			return nil, fmt.Errorf("seam: overlay %s: %w", name, err)
		}
		// Materialize the pack's self-describing permission descriptor (if any)
		// so the Go-native permission emitter (internal/permconfig) can resolve
		// the active roster DYNAMICALLY (by directory listing) instead of the
		// canonical Go tables hardcoding any pack's agents.
		ppRel, err := pack.MaterializePermissionPack(staging)
		if err != nil {
			return nil, fmt.Errorf("seam: overlay %s: %w", name, err)
		}
		if ppRel != "" {
			overlayFiles = append(overlayFiles, ppRel)
		}
		packs = append(packs, pack)
	}
	// Prompt-extension merge pass (Slice 2): inject each active pack's
	// *.extend.<slot>.<ext> snippets at their matching anchors in the rendered
	// target files. Orphans (snippets with no matching anchor) are WARNED, never
	// silently dropped.
	report, err := overlay.InjectExtensionSnippets(staging, packs, renderAnswers)
	if err != nil {
		return nil, fmt.Errorf("seam: inject prompt-extension snippets: %w", err)
	}
	for _, o := range report.Orphans {
		fmt.Fprintf(os.Stderr, "seam: warning: orphan prompt-extension snippet (no matching anchor): pack=%s target=%s slot=%s\n", o.Pack, o.TargetRel, o.Slot)
	}
	// Canonical permission emission (O5 slice 2b): collapse the dual permission
	// source of truth (corpus template bash/task literals AND the Node resolver
	// tables) into a single Go-native emitter. The emitter overwrites
	// permission.bash and permission.task blocks authoritatively from Go canonical
	// tables (internal/permconfig/tables.go), injects delegateFrom task edges from
	// the materialized permission-packs, and applies the features.backlog gate.
	// This replaces the demoted Node resolver (update-opencode-config.js, now a
	// deprecation stub) as the operational authority for permission content.
	// doctor re-renders via this same renderSeamStaging pipeline so managed-drift
	// auto-coheres with the canonical form.
	permPacks, err := permconfig.LoadPacks(staging)
	if err != nil {
		return nil, fmt.Errorf("seam: load permission-packs: %w", err)
	}
	cfgPath := filepath.Join(staging, "opencode.jsonc")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("seam: read staged opencode.jsonc for permission emission: %w", err)
	}
	features := permconfig.Features{Backlog: renderAnswers["features.backlog"] == "true"}
	emitted, err := permconfig.Emit(data, permPacks, features)
	if err != nil {
		return nil, fmt.Errorf("seam: emit canonical permissions: %w", err)
	}
	if err := os.WriteFile(cfgPath, emitted, 0o644); err != nil {
		return nil, fmt.Errorf("seam: write canonical opencode.jsonc: %w", err)
	}
	// Generate allowed-commands.js from the same Go canonical tables so the
	// shell-guard runtime hook (which imports it as a JS module at exec time)
	// stays in sync with the emitted permission blocks. Single-source: the Go
	// tables own both the opencode.jsonc permission content and this compat
	// artifact. The file is platform_managed; see README.agent.md.
	acDir := filepath.Join(staging, ".opencode", "repo-configs")
	if err := os.MkdirAll(acDir, 0o755); err != nil {
		return nil, fmt.Errorf("seam: ensure repo-configs dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(acDir, "allowed-commands.js"), permconfig.GenerateAllowedCommandsJS(), 0o644); err != nil {
		return nil, fmt.Errorf("seam: write generated allowed-commands.js: %w", err)
	}
	return overlayFiles, nil
}

// walkStagedLivePaths returns the set of LIVE .opencode-relative paths already
// present in staging (the builtin corpus the renderer just wrote, plus anything
// earlier overlays rendered). Paths are normalized to forward slashes and made
// relative to staging (e.g. ".opencode/agents/build.md"). Used by the Slice 3
// shadowing guard to detect overlay→builtin collisions before a pack renders.
func walkStagedLivePaths(staging string) map[string]bool {
	out := map[string]bool{}
	_ = filepath.WalkDir(staging, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(staging, p)
		if rerr != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = true
		return nil
	})
	return out
}

// warnIfAllowedCommandsCustomized compares the LIVE allowed-commands.js in the
// target with the freshly-generated canonical form in staging. If they differ,
// the operator either customized the file (adding/removing commands in the
// readonly/git_readonly/gate groups) or is running a prior harness version. The
// canonical overwrite will discard those changes; this warning makes that
// visible so the operator can back up the file or port custom deny-rules to
// forbidden-patterns.project.js (Q5c: never silently discard custom command
// groups).
func warnIfAllowedCommandsCustomized(target, staging string) {
	if !isAllowedCommandsCustomized(target, staging) {
		return
	}
	const rel = ".opencode/repo-configs/allowed-commands.js"
	fmt.Fprintf(os.Stderr, `
vh-agent-harness WARNING: %s has been modified and will be overwritten.
  The file is now GENERATED from Go canonical tables (internal/permconfig/tables.go).
  Previous content differed from the canonical form — custom commands in the
  readonly/git_readonly/gate groups are no longer picked up.
  To preserve custom deny-rules, use .opencode/repo-configs/forbidden-patterns.project.js.
  To preview the exact changes, run: vh-agent-harness update --dry-run

`, rel)
}

// isAllowedCommandsCustomized reports whether the live allowed-commands.js in
// target differs from the freshly-generated canonical form in staging. Returns
// false when either file is missing (first install or render bug surfaced
// elsewhere).
func isAllowedCommandsCustomized(target, staging string) bool {
	const rel = ".opencode/repo-configs/allowed-commands.js"
	staged, serr := os.ReadFile(filepath.Join(staging, rel))
	if serr != nil {
		return false
	}
	live, lerr := os.ReadFile(filepath.Join(target, rel))
	if lerr != nil {
		return false
	}
	return !bytes.Equal(staged, live)
}

// seamClassifierWithOverlays builds the seam classifier for one apply: the core
// ownership defaults extended with overlay_extension rules for every path the
// active overlays rendered, then resolved against the project's S2 raise-only
// overrides (Slice 5.1). When overlayFiles and overrides are both empty this is
// equivalent to the memoized core-only classifier. A downgrade override (or any
// other D2-A violation: unknown path, invalid class, off-lattice class) makes
// ownership.Resolve return a joined error; this function surfaces it so
// seamApply aborts before any write touches the live tree.
func seamClassifierWithOverlays(overlayFiles []string, overrides ownership.Overrides) (*substrate.Classifier, error) {
	defaults, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		return nil, fmt.Errorf("seam: core ownership: %w", err)
	}
	for _, rel := range overlayFiles {
		defaults[rel] = ownership.PathRule{
			Class:      ownership.ClassOverlayExtension,
			Provenance: "overlay",
		}
	}
	eff, err := ownership.Resolve(defaults, overrides)
	if err != nil {
		return nil, fmt.Errorf("seam: ownership resolve (raise-only; a downgrade override was rejected): %w", err)
	}
	return substrate.NewClassifier(eff, nil), nil
}

// mergeRenderAnswers combines the caller-supplied install answers with the
// profile-derived render answers (features.*, overlays). Profile answers win for
// the keys they own; install answers (project_name/project_slug) are never
// overwritten. This keeps the S3 profile as the feature-surface authority that
// drives render conditionals while the S1 lineage digest stays anchored to the
// install answers. A nil base yields a fresh map (the caller's map is never
// mutated).
func mergeRenderAnswers(base, profile map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(profile))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range profile {
		out[k] = v
	}
	return out
}
