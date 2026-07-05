package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// doctorCmd is the SEAM health report (Slice 2). It is the authoritative lint +
// integrity surface for a seam-installed tree and validates the four authority
// surfaces the seam owns:
//
//  1. lineage (S1): the .vh-agent-harness/lineage.yml record is present and
//     parseable (absent => WARN not-installed, unparseable/leaked => FAIL).
//  2. armed-schema: every platform_armed file present in the live tree is
//     schema-conformant (the authoritative lint; update reconciles, doctor lints).
//     A missing armed file is a WARN (it will be re-seeded on the next update).
//  3. managed-drift: every platform_managed file's live bytes equal the staged
//     (re-rendered) bytes. Drift or absence is a FAIL (run `vh-agent-harness update`).
//  4. environment: node is on PATH (shell-guard readiness) and the shell-guard
//     eval.js bridge is present in the live tree.
//
// WARNs do not fail the command; FAILs do. The validated mechanisms (ownership
// classifier, schema registry, lineage, capability/hooks readiness) are carried
// here without being rewritten: doctor reads lineage the seam wrote, lints armed
// files via the schema registry, and re-derives managed bytes via the renderer.
//
// This replaces the legacy manifest+drift+copier-answers doctor. The legacy
// manifest path (preflight) is unchanged; manifest convergence is a later slice.
var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Diagnose seam-installed harness health (lineage + armed + drift + env)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Detailed seam health diagnosis (read-only). Reports:

  lineage       .vh-agent-harness/lineage.yml present + parseable    FAIL if leaked/unparseable
  armed-schema  every platform_armed file schema-conformant          FAIL if schema-invalid
  managed-drift every platform_managed file matches re-rendered bytes FAIL if drifted/missing
  overlay-perm  active overlay permission-packs resolved in opencode.jsonc FAIL if resolver not run
  environment   node on PATH + shell-guard eval.js present            FAIL if missing
  gitignore     harness-written dirs (.opencode/state…, __pycache__) ignored WARN if not ignored

Exits non-zero if any FAIL is found. WARNs (armed file absent, lineage absent)
do not fail. This is the seam doctor surface; the legacy manifest model is
unchanged and converges in a later slice.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

// doctorTargetFlag lets tests/CI point doctor at a target other than cwd.
var doctorTargetFlag string

func init() {
	doctorCmd.Flags().StringVarP(&doctorTargetFlag, "target", "o", "",
		"target directory (default: current directory)")
}

func runDoctor(cmd *cobra.Command, _ []string) (err error) {
	// doctor runs with SilenceErrors:true. Its normal failure path returns
	// errSilent (problems>0), which reportRunErrToStderr skips so the already-
	// printed UNHEALTHY report is the only output. A genuine runtime error
	// (getcwd/resolve-target, or any future classify-time failure) is printed to
	// stderr here so it is not silently swallowed into a bare non-zero exit.
	defer func() { reportRunErrToStderr(cmd, err) }()

	out := cmd.OutOrStdout()

	target := doctorTargetFlag
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getcwd: %w", err)
		}
		target = cwd
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	problems := 0
	warns := 0

	fmt.Fprintln(out, "doctor:")

	// 1. Lineage (S1 authority).
	fmt.Fprintln(out, "  lineage:")
	lr := checkSeamLineage(abs)
	fmt.Fprintln(out, "    "+lr.String())
	applyTier(lr.tier, &problems, &warns)

	// 2. Armed-schema (authoritative lint over platform_armed live files).
	fmt.Fprintln(out, "  armed-schema:")
	ar := checkArmedSchema(abs)
	fmt.Fprintln(out, "    "+ar.String())
	applyTier(ar.tier, &problems, &warns)

	// 3. Managed-drift (re-render staging, byte-compare every managed path).
	fmt.Fprintln(out, "  managed-drift:")
	dr := checkManagedDrift(abs)
	fmt.Fprintln(out, "    "+dr.String())
	applyTier(dr.tier, &problems, &warns)

	// 4. Overlay-perm (detection-only honesty check): an active overlay with a
	//    permission-pack.jsonc needs the operator-run node resolver to populate
	//    the pack agents' permission.bash/task blocks + delegateFrom edges in
	//    opencode.jsonc. The Go binary never invokes that resolver, so a freshly
	//    rendered overlay repo is "managed-drift clean" but functionally broken.
	//    This surfaces the broken state as FAIL instead of a silent HEALTHY.
	//    Read-only; core-only repos stay silent (PASS). The architecture fix
	//    (resolver-in-pipeline) is deferred to P2-PIPELINE-001 Slice 2.
	fmt.Fprintln(out, "  overlay-perm:")
	or := checkOverlayPermissionState(abs)
	fmt.Fprintln(out, "    "+or.String())
	applyTier(or.tier, &problems, &warns)

	// 5. Environment (shell-guard readiness: node + eval.js bridge).
	fmt.Fprintln(out, "  environment:")
	nr := checkNode()
	fmt.Fprintln(out, "    "+nr.String())
	applyTier(nr.tier, &problems, &warns)
	er := checkEvalJS(abs)
	fmt.Fprintln(out, "    "+er.String())
	applyTier(er.tier, &problems, &warns)

	// 6. Config file-refs ({file:...} in opencode.jsonc must resolve; per-agent
	//    model files exist-but-empty are a setup warning).
	fmt.Fprintln(out, "  config-refs:")
	cr := checkConfigRefs(abs)
	fmt.Fprintln(out, "    "+cr.String())
	applyTier(cr.tier, &problems, &warns)

	// 7. Harness-written dirs must be gitignored (WARN): runtime scratch +
	//    Python __pycache__. .gitignore is project_owned, so an adopted/edited
	//    repo can silently commit them; this surfaces it without failing.
	fmt.Fprintln(out, "  gitignore:")
	gr := checkRuntimeStateGitignored(abs)
	fmt.Fprintln(out, "    "+gr.String())
	applyTier(gr.tier, &problems, &warns)

	// Summary.
	fmt.Fprintf(out, "summary: %d problem(s), %d warning(s)\n", problems, warns)
	if problems > 0 {
		fmt.Fprintln(out, "result: UNHEALTHY — see details above; run `vh-agent-harness update` to repair drift.")
		return errSilent{}
	}
	fmt.Fprintln(out, "result: HEALTHY")
	return nil
}

func applyTier(tier string, problems, warns *int) {
	switch tier {
	case tierFail:
		*problems++
	case tierWarn:
		*warns++
	}
}

// checkSeamLineage reads the S1 lineage record. Absent => WARN (not seam-
// installed yet); unparseable or authority-leaked => FAIL; present => PASS.
func checkSeamLineage(target string) checkResult {
	lin, err := lineage.Read(target)
	if err != nil {
		return checkResult{name: "lineage", tier: tierFail,
			detail: fmt.Sprintf("lineage unreadable: %v (see .vh-agent-harness/lineage.yml)", err)}
	}
	if lin == nil {
		return checkResult{name: "lineage", tier: tierWarn,
			detail: "no lineage record (not seam-installed); run `vh-agent-harness install`"}
	}
	ref := lin.Template.Ref
	if ref == "" {
		ref = "(unset)"
	}
	return checkResult{name: "lineage", tier: tierPass,
		detail: fmt.Sprintf("rendered_by=%q ref=%q digest=%s",
			lin.Render.RenderedBy, ref, truncDigest(lin.Answers.Digest))}
}

// checkArmedSchema lints every schema'd file the seam owns. Slice 2/5.2:
//
//  1. Every core platform_armed file (vh-harness-profile.yml): validate the live
//     instance (schema-invalid => FAIL); missing => WARN (re-seeded on update).
//  2. The other three schema'd authorities — run-shape.yml, repo-recon-data.yml,
//     and forbidden-patterns.project.js — when present in the live tree
//     (Slice 5.2 full-registry wiring). These are project_owned /
//     external_generated, so absence is NOT a warn; only a present-but-invalid
//     instance is a FAIL.
//
// This is the authoritative lint surface: update reconciles (armed files),
// doctor lints (all schema'd files). The reconcile path already re-derives the
// merge on apply; doctor is the integrity probe an operator runs between updates.
func checkArmedSchema(target string) checkResult {
	defaults, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		return checkResult{name: "armed-schema", tier: tierFail,
			detail: fmt.Sprintf("core ownership: %v", err)}
	}
	var problems []string
	armedCount := 0
	for path, rule := range defaults {
		if rule.Class != ownership.ClassPlatformArmed {
			continue
		}
		armedCount++
		live := filepath.Join(target, filepath.FromSlash(path))
		raw, rerr := os.ReadFile(live)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				// Missing armed file: WARN per-file, but keep scanning.
				problems = append(problems, fmt.Sprintf("%s: missing (will be re-seeded by `vh-agent-harness update`)", path))
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: unreadable: %v", path, rerr))
			continue
		}
		sch, ok := schema.SchemaForPath(path)
		if !ok {
			// An armed path with no registered schema is a platform bug;
			// substrate.Apply would already have refused it. Report FAIL.
			problems = append(problems, fmt.Sprintf("%s: no registered schema (platform bug)", path))
			continue
		}
		if errs := sch.Validator.Validate(raw); len(errs) > 0 {
			problems = append(problems, fmt.Sprintf("%s: schema-invalid: %s", path, fieldErrs(errs)))
		}
	}

	// Slice 5.2: lint the other three schema'd authorities when present. These
	// are project_owned / external_generated, so absence is silent (not a warn);
	// only a present-but-invalid instance is a FAIL. repo-recon-data.yml is
	// seeded blank on install (external_generated), so it is usually present and
	// valid; run-shape.yml and forbidden-patterns.project.js are optional.
	for _, sp := range additionalSchemaPaths() {
		live := filepath.Join(target, filepath.FromSlash(sp.path))
		raw, rerr := os.ReadFile(live)
		if rerr != nil {
			continue // absent is fine for these classes
		}
		if errs := sp.validator.Validate(raw); len(errs) > 0 {
			problems = append(problems, fmt.Sprintf("%s: schema-invalid: %s", sp.path, fieldErrs(errs)))
		}
	}

	if len(problems) == 0 {
		return checkResult{name: "armed-schema", tier: tierPass,
			detail: fmt.Sprintf("%d armed file(s) schema-conformant (+ additional schema'd authorities clean)", armedCount)}
	}
	// Distinguish pure-missing (all WARN) from schema-invalid/bug (FAIL).
	allMissing := true
	for _, p := range problems {
		if !strings.Contains(p, "missing") {
			allMissing = false
			break
		}
	}
	tier := tierFail
	if allMissing {
		tier = tierWarn
	}
	return checkResult{name: "armed-schema", tier: tier,
		detail: strings.Join(problems, "; ")}
}

// schemaPath is one additional (non-core-platform_armed) schema'd authority
// doctor lints when present.
type schemaPath struct {
	path      string
	validator schema.Validator
}

// additionalSchemaPaths lists the three non-core-platform_armed schema'd
// authorities doctor lints if present (Slice 5.2). run-shape.yml is the S4
// runtime shape (project_owned, .vh-agent-harness/); repo-recon-data.yml is the
// external_generated recon map; forbidden-patterns.project.js is the
// project_owned deny-rule payload. Each resolves to its registered validator.
func additionalSchemaPaths() []schemaPath {
	out := make([]schemaPath, 0, 3)
	for _, p := range []string{
		".vh-agent-harness/run-shape.yml",
		".opencode/repo-configs/repo-recon-data.yml",
		".opencode/repo-configs/forbidden-patterns.project.js",
	} {
		if sch, ok := schema.SchemaForPath(p); ok {
			out = append(out, schemaPath{path: p, validator: sch.Validator})
		}
	}
	return out
}

// checkManagedDrift re-renders the core corpus into an out-of-tree staging dir
// and byte-compares every platform_managed path against the live tree. Drift or
// absence => FAIL (run `vh-agent-harness update`). It exercises the renderer + classifier
// the seam uses, so a doctor pass is a faithful integrity probe.
//
// Ownership-override-aware (decision A2): the byte-compare filter uses the
// EFFECTIVE ownership class (platform defaults reconciled with the project's
// raise-only harness-ownership.yml overrides via ownership.Resolve), mirroring
// seamInventory/computeSeamDrift. A path whose effective class is raised above
// platform_managed (e.g. project_owned via override) is NEVER counted as drift:
// update (substrate.Apply) already routes it to ActionProjectPreserved, so the
// live bytes are intentionally divergent. Such a present-and-divergent raised
// path is surfaced as a non-failing `preserved` (tierInfo) signal instead of a
// perpetual FAIL — closing the gap where doctor re-rendered, byte-compared, and
// failed forever on a divergence the update it points at is a no-op-by-design on.
//
// An absent override file (the common case) resolves to nil overrides, so
// behavior is byte-identical to the raw-class check for repos without overrides.
func checkManagedDrift(target string) checkResult {
	defaults, err := corpus.CoreOwnershipDefaults()
	if err != nil {
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("core ownership: %v", err)}
	}
	overrides, oerr := readOwnershipOverrides(target)
	if oerr != nil {
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("read ownership overrides: %v", oerr)}
	}
	eff, rverr := ownership.Resolve(defaults, overrides)
	if rverr != nil {
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("ownership resolve (raise-only): %v", rverr)}
	}
	sub, err := coreSubFSImpl()
	if err != nil {
		return checkResult{name: "managed-drift", tier: tierFail, detail: err.Error()}
	}
	staging, err := os.MkdirTemp("", "harness-doctor-staging-*")
	if err != nil {
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("create staging: %v", err)}
	}
	defer os.RemoveAll(staging)
	r := substrate.EmbedFSRenderer{Source: sub}
	// Render with the SAME pipeline seamApply uses (core + active overlays + live
	// S3 profile answers), so the doctor re-render is faithful to the operator's
	// feature/overlay decisions AND the install-identity answers
	// (project_name/slug/coordinator_dir recovered from lineage). Without the
	// profile answers a features.backlog=true project would false-flag drift on
	// opencode.jsonc (the conditional would collapse); without the recovered
	// install answers the token-bearing managed files would false-flag drift
	// whenever the install name/slug differ from the target dir basename.
	answers := mergeRenderAnswers(installRenderAnswers(target), readProfileAnswers(target))
	if _, err := renderSeamStaging(staging, r, answers, target); err != nil {
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("render staging: %v", err)}
	}
	drifted, missing, preserved := 0, 0, 0
	checked := 0
	for path := range defaults {
		effClass, _ := eff.ClassOf(path)
		staged, serr := os.ReadFile(filepath.Join(staging, filepath.FromSlash(path)))
		live, lerr := os.ReadFile(filepath.Join(target, filepath.FromSlash(path)))
		if effClass != ownership.ClassPlatformManaged {
			// Raised via override (e.g. project_owned). update preserves it; doctor
			// must not flag byte divergence as drift. A present-and-divergent raised
			// path is surfaced as non-failing `preserved`. Absence is silent: a
			// raised path is the operator's concern (update never seeds/touches it),
			// so a missing project_owned file is neither drift nor preservation.
			if serr == nil && lerr == nil && string(live) != string(staged) {
				preserved++
			}
			continue
		}
		checked++
		if serr != nil {
			// A managed path that fails to render is a platform/template bug.
			drifted++
			continue
		}
		if lerr != nil {
			if os.IsNotExist(lerr) {
				missing++
			} else {
				drifted++
			}
			continue
		}
		if string(live) != string(staged) {
			drifted++
		}
	}
	switch {
	case drifted > 0:
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("%d drifted, %d missing of %d managed — run `vh-agent-harness update`", drifted, missing, checked)}
	case missing > 0:
		return checkResult{name: "managed-drift", tier: tierFail,
			detail: fmt.Sprintf("%d missing of %d managed — run `vh-agent-harness update`", missing, checked)}
	case preserved > 0:
		return checkResult{name: "managed-drift", tier: tierInfo,
			detail: fmt.Sprintf("%d managed file(s) in sync; %d project-preserved (ownership override)", checked, preserved)}
	default:
		return checkResult{name: "managed-drift", tier: tierPass,
			detail: fmt.Sprintf("%d managed file(s) in sync", checked)}
	}
}

// overlayPermRecoveryCmd is the recovery command surfaced when the overlay-perm
// check detects unresolved permissions. Since O5 (P2-PIPELINE-001 Slice 2), the
// Go-native emitter (internal/permconfig/) resolves all permission blocks inside
// `vh-agent-harness update`'s render pipeline, so the recovery is simply to
// re-run update.
const overlayPermRecoveryCmd = "vh-agent-harness update"

// jsoncCommentRe strips JSONC line (//…) and block (/*…*/) comments so a
// permission-pack body can be parsed by encoding/json. It is sufficient for the
// well-formed, machine-generated packs the seam ships; not a general JSONC parser.
var jsoncCommentRe = regexp.MustCompile(`(?s)//.*?\n|/\*.*?\*/`)

// jsoncTrailingCommaRe strips trailing commas before } and ] so a JSONC pack body
// (which commonly ends list/map entries with a comma) parses as strict JSON.
var jsoncTrailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)

// overlayPackRef names one active overlay pack that ships a permission-pack.jsonc.
type overlayPackRef struct {
	name string // overlay pack name (for human-readable errors)
	path string // absolute path to its permission-pack.jsonc
}

// checkOverlayPermissionState is a DETECTION-ONLY honesty check (P2-VERIFY-001
// Slice 1). It surfaces the silent "healthy-but-non-functional" overlay state.
// Since O5 (P2-PIPELINE-001 Slice 2) the Go-native emitter
// (internal/permconfig/) resolves overlay permissions inside the update render
// pipeline, so a clean `update` always produces resolved edges. This check
// catches the residual failure modes: a stale install from before the emitter,
// a hand-authored pack with a `__placeholder__` sentinel (Signal A; defensive —
// the scaffolder `overlay new` never emits it), or a permission-pack-declared
// agent whose `"<agent>": "allow"|"ask"` delegateFrom task edge is absent
// (Signal B; the primary detector). Either means the operator should re-run
// `vh-agent-harness update`.
//
// The check is READ-ONLY: it inspects the active overlays' permission-pack.jsonc
// files and opencode.jsonc for two resolver-has-not-run signals, and names the
// recovery command. It does NOT mutate any file.
//
// Core-only repos (no active overlays, or overlays without permission-packs) are
// SILENT — the check returns PASS so doctor stays HEALTHY.
func checkOverlayPermissionState(target string) checkResult {
	const name = "overlay-perm"

	// 1. No active overlays -> core-only -> resolver is not required -> silent PASS.
	overlays := activeOverlays(target)
	if len(overlays) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: "no active overlays (core-only) — permission resolver not required"}
	}

	// 2. Collect every active overlay that ships a permission-pack.jsonc. If none
	//    does, the resolver has nothing to resolve here -> silent PASS.
	var packs []overlayPackRef
	for _, ov := range overlays {
		p := filepath.Join(target, ".vh-agent-harness", "overlays", ov, "permission-pack.jsonc")
		if isRegularFile(p) {
			packs = append(packs, overlayPackRef{name: ov, path: p})
		}
	}
	if len(packs) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d active overlay(s), none carry a permission-pack.jsonc — resolver not required", len(overlays))}
	}

	// 3. opencode.jsonc is the surface the resolver rewrites. If it is absent the
	//    managed-drift check already FAILs the missing managed file; stay SKIP
	//    here rather than double-reporting existence as a content problem.
	data, err := os.ReadFile(filepath.Join(target, "opencode.jsonc"))
	if err != nil {
		return checkResult{name: name, tier: tierSkip,
			detail: "no opencode.jsonc (managed-drift reports absence)"}
	}
	cfg := string(data)

	// 4. Signal A (defensive) — the `__placeholder__` sentinel some hand-authored
	//    overlay packs use for unfilled permission buckets. The harness scaffolder
	//    (`overlay new`) writes resolved values, not this sentinel, so this branch
	//    is a belt-and-suspenders catch; Signal B below is the primary detector.
	//    Harmless when the literal never appears (no false-positive risk).
	if strings.Contains(cfg, "__placeholder__") {
		return checkResult{name: name, tier: tierFail,
			detail: unresolvedOverlayPermDetail("__placeholder__ present in opencode.jsonc")}
	}

	// 5. Signal B — the resolver injects each pack agent's delegateFrom edges as
	//    `"<agent>": "allow"|"ask"` task entries into the core orchestrators. A
	//    missing edge for any declared agent means the resolver has not run.
	agentKeys, parseErr := permissionPackAgentKeys(packs)
	if parseErr != nil {
		// A malformed pack is the schema/managed lint surfaces' concern; Signal A
		// was already checked above, so SKIP Signal B rather than mask Signal A or
		// fail on a parse detail outside this check's contract.
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("permission-pack parse: %v (signal A checked, signal B skipped)", parseErr)}
	}
	var missing []string
	for _, k := range agentKeys {
		if !hasResolvedAgentEdge(cfg, k) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return checkResult{name: name, tier: tierFail,
			detail: unresolvedOverlayPermDetail(
				fmt.Sprintf("missing delegateFrom edges for agent(s): %s", strings.Join(missing, ", ")))}
	}
	return checkResult{name: name, tier: tierPass,
		detail: fmt.Sprintf("%d overlay agent(s) across %d pack(s) have resolved permission edges", len(agentKeys), len(packs))}
}

// unresolvedOverlayPermDetail renders the standard FAIL detail for the overlay-perm
// check: it always names the recovery command so an operator (or agent) can
// copy-paste it. reason is the specific signal that fired.
func unresolvedOverlayPermDetail(reason string) string {
	return fmt.Sprintf("unresolved overlay permissions (%s); run `%s` to resolve", reason, overlayPermRecoveryCmd)
}

// permissionPackAgentKeys reads each active pack's permission-pack.jsonc and
// returns the distinct agent keys declared under the top-level "agents" object,
// in first-seen order. Packs are JSONC (comments + trailing commas); this strips
// that noise then unmarshals into a generic map and walks ["agents"]. A pack that
// declares no "agents" object contributes nothing. A read/parse error short-circuits.
func permissionPackAgentKeys(packs []overlayPackRef) ([]string, error) {
	seen := map[string]bool{}
	var keys []string
	for _, pk := range packs {
		raw, err := os.ReadFile(pk.path)
		if err != nil {
			return nil, fmt.Errorf("read %s/permission-pack.jsonc: %w", pk.name, err)
		}
		cleaned := jsoncCommentRe.ReplaceAllString(string(raw), "")
		cleaned = jsoncTrailingCommaRe.ReplaceAllString(cleaned, "$1")
		var doc map[string]any
		if err := json.Unmarshal([]byte(cleaned), &doc); err != nil {
			return nil, fmt.Errorf("parse %s/permission-pack.jsonc: %w", pk.name, err)
		}
		agentsNode, ok := doc["agents"].(map[string]any)
		if !ok {
			continue // pack declares no agents; nothing to resolve for it
		}
		for k := range agentsNode {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
}

// hasResolvedAgentEdge reports whether cfg (opencode.jsonc text) contains a
// resolved task entry for agent — i.e. `"agent": "allow"` or `"agent": "ask"`.
// The resolver injects these as delegateFrom edges into the core orchestrators'
// task blocks; their absence (only a deny or no entry) means unresolved. The
// match is tolerant of arbitrary whitespace around the colon. The agent name is
// regexp-escaped defensively even though overlay names are filesystem-safe.
func hasResolvedAgentEdge(cfg, agent string) bool {
	re := regexp.MustCompile(fmt.Sprintf(`"%s":\s*"(allow|ask)"`, regexp.QuoteMeta(agent)))
	return re.MatchString(cfg)
}

// harnessWrittenIgnorableDirs are subtrees the harness's own runtime and scripts
// create that must be gitignored or they get committed:
//   - per-project agent scratch (.opencode/state|sessions|plans|runs), kept in
//     sync with seamUnexpectedSkip (the drift "unexpected" scan);
//   - the __pycache__ dirs Python leaves next to every shipped .py script dir
//     (.opencode/scripts, .opencode/sys-scripts, and the skill script dirs).
//
// The shipped .gitignore covers all of them (a global __pycache__/ plus the
// runtime-state entries), but .gitignore is project_owned (seeded on greenfield,
// PRESERVED on adopt and freely hand-editable), so an adopted repo can silently
// start committing them. checkRuntimeStateGitignored WARNs in that case.
var harnessWrittenIgnorableDirs = []string{
	".opencode/state",
	".opencode/sessions",
	".opencode/plans",
	".opencode/runs",
	".opencode/scripts/__pycache__",
	".opencode/sys-scripts/__pycache__",
	".opencode/skills/bgshell-job/scripts/__pycache__",
	".opencode/skills/skill-creator/scripts/__pycache__",
}

// checkRuntimeStateGitignored WARNs when any harness-written dir is not ignored
// by git (runtime scratch + Python __pycache__). It shells to `git check-ignore`
// (the authoritative resolver: honors nested ignores, negations,
// core.excludesFile) against a probe path UNDER each dir so a `dir/` rule matches
// whether or not the dir currently exists. The check is a no-op (SKIP) outside a
// git work tree or when git is unavailable, and is WARN (never FAIL) so it never
// blocks a command.
func checkRuntimeStateGitignored(target string) checkResult {
	const name = "gitignore"
	if _, err := exec.LookPath("git"); err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "git not on PATH"}
	}
	wt, err := exec.Command("git", "-C", target, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(wt)) != "true" {
		return checkResult{name: name, tier: tierSkip, detail: "not a git work tree"}
	}
	var notIgnored []string
	for _, d := range harnessWrittenIgnorableDirs {
		// Probe a path UNDER the dir so a `dir/` ignore rule matches even when
		// the dir does not yet exist on disk.
		runErr := exec.Command("git", "-C", target, "check-ignore", "-q", d+"/.probe").Run()
		if runErr == nil {
			continue // exit 0: ignored
		}
		if ee, ok := runErr.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			notIgnored = append(notIgnored, d) // exit 1: NOT ignored
			continue
		}
		// exit 128 / other: indeterminate — do not guess, skip the whole check.
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("git check-ignore failed: %v", runErr)}
	}
	if len(notIgnored) > 0 {
		return checkResult{name: name, tier: tierWarn, detail: fmt.Sprintf(
			"%d harness-written dir(s) NOT gitignored (e.g. %s/) — add to .gitignore so agent scratch / __pycache__ isn't committed",
			len(notIgnored), notIgnored[0])}
	}
	return checkResult{name: name, tier: tierPass,
		detail: fmt.Sprintf("%d harness-written dir(s) gitignored", len(harnessWrittenIgnorableDirs))}
}

// fieldErrs renders a slice of schema.FieldError into a compact string.
func fieldErrs(errs []schema.FieldError) string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return strings.Join(out, "; ")
}

// truncDigest shortens a "sha256:<hex>" digest for readable doctor output.
func truncDigest(d string) string {
	if len(d) > 20 {
		return d[:20] + "…"
	}
	return d
}

// configRefRe matches `{file:...}` directives in opencode.jsonc. The captured
// path is repo-relative (with or without a leading "./").
var configRefRe = regexp.MustCompile(`\{file:([^}]+)\}`)

// checkConfigRefs verifies that every `{file:...}` reference in opencode.jsonc
// resolves to an existing file, and flags per-agent model files that exist but
// are empty (operator has not chosen a model yet). A missing referenced file
// breaks OpenCode's config load (FAIL); an empty agent-model file means the
// agent falls back to OpenCode's default until a model id is set (WARN). The
// literal `<name>` example in the file's header comment is skipped.
func checkConfigRefs(target string) checkResult {
	const name = "config-refs"
	data, err := os.ReadFile(filepath.Join(target, "opencode.jsonc"))
	if err != nil {
		return checkResult{name: name, tier: tierSkip, detail: "no opencode.jsonc (not installed here)"}
	}
	var missing, emptyModels []string
	seen := map[string]bool{}
	for _, m := range configRefRe.FindAllSubmatch(data, -1) {
		rel := strings.TrimPrefix(string(m[1]), "./")
		if rel == "" || strings.Contains(rel, "<") || seen[rel] {
			continue
		}
		seen[rel] = true
		info, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(rel)))
		if statErr != nil {
			missing = append(missing, rel)
			continue
		}
		if info.Size() == 0 && strings.HasPrefix(rel, ".local/config/agent-model/") {
			emptyModels = append(emptyModels, rel)
		}
	}
	if len(missing) > 0 {
		return checkResult{name: name, tier: tierFail, detail: fmt.Sprintf(
			"%d {file:} ref(s) point to missing files (e.g. %s) — OpenCode config load will fail; run `vh-agent-harness update`",
			len(missing), missing[0])}
	}
	if len(emptyModels) > 0 {
		return checkResult{name: name, tier: tierWarn, detail: fmt.Sprintf(
			"%d agent-model file(s) empty (e.g. %s) — set a model id; agents fall back to OpenCode's default until then",
			len(emptyModels), emptyModels[0])}
	}
	return checkResult{name: name, tier: tierPass, detail: fmt.Sprintf("%d {file:} ref(s) resolve", len(seen))}
}
