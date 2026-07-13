package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
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

  lineage         .vh-agent-harness/lineage.yml present + parseable     FAIL if leaked/unparseable
  armed-schema    every platform_armed file schema-conformant           FAIL if schema-invalid
  managed-drift   every platform_managed file matches re-rendered bytes  FAIL if drifted/missing
  overlay-perm    active overlay permission-packs resolved in opencode.jsonc FAIL if resolver not run
  environment     node on PATH + shell-guard eval.js present             FAIL if missing
  config-refs     {file:...} refs resolve; empty agent-model files       FAIL if missing ref / WARN if empty
  gitignore       harness-written dirs (.opencode/state…, __pycache__) ignored WARN if not ignored
  auto-classifier auto-classifier-pilot overlay config shapes valid      FAIL if present-but-invalid

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

	// 8. Auto-classifier-pilot overlay config shapes (present-but-invalid = FAIL).
	//    Validates the JSON envelope of the overlay's 4 config files (project +
	//    user, plugin + LLM) when present. Absent files are not failures (LLM
	//    config is normally absent; plugin defaults apply). Clean no-op (SKIP)
	//    when the overlay is unselected AND no config files exist.
	fmt.Fprintln(out, "  auto-classifier:")
	ar2 := checkAutoGateConfig(abs)
	fmt.Fprintln(out, "    "+ar2.String())
	applyTier(ar2.tier, &problems, &warns)

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
// Ownership-override-aware (decision A2, narrowed by the F3 fix / P1-DRIFT-002):
// the byte-compare filter gates on the EFFECTIVE ownership ORIGIN, not merely
// the effective class. ownership.Resolve reconciles platform defaults with the
// project's raise-only harness-ownership.yml overrides, and each EffectiveEntry
// records how its class was derived. Only a path whose effective class was
// GENUINELY RAISED above its platform default by an override
// (EffectiveEntry.Origin == OriginOverrideRaise) is treated as preserved — NOT
// every path whose effective class differs from platform_managed. update
// (substrate.Apply) already routes a raised path to ActionProjectPreserved, so
// its live bytes are intentionally divergent; doctor surfaces such a present-
// and-divergent raised path as a non-failing `preserved` (tierInfo) signal
// instead of a perpetual FAIL — closing the gap where doctor re-rendered,
// byte-compared, and failed forever on a divergence the update it points at is a
// no-op-by-design on. Default-class non-managed files (project_owned seeds like
// README.md/Makefile, the platform_armed profile, external_generated recon data)
// diverge by design and are silently skipped — they are NOT overrides.
//
// An absent override file (the common case) resolves to nil overrides, so every
// path keeps OriginDefault and behavior is byte-identical to the raw-class check
// for repos without overrides.
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
		// Resolve seeds every default path, so eff[path] is always present; its
		// Origin records whether an override genuinely raised the class.
		entry := eff[path]
		staged, serr := os.ReadFile(filepath.Join(staging, filepath.FromSlash(path)))
		live, lerr := os.ReadFile(filepath.Join(target, filepath.FromSlash(path)))
		if entry.Origin == ownership.OriginOverrideRaise {
			// Genuinely RAISED above its platform default by an ownership override
			// (e.g. platform_managed -> project_owned). update (substrate.Apply)
			// routes such a path to ActionProjectPreserved, so the live bytes are
			// intentionally divergent. A present-and-divergent raised path is
			// surfaced as a non-failing `preserved` (tierInfo) signal instead of a
			// perpetual FAIL. Absence is silent: a raised path is the operator's
			// concern (update never seeds/touches it), so a missing raised file is
			// neither drift nor preservation.
			if serr == nil && lerr == nil && string(live) != string(staged) {
				preserved++
			}
			continue
		}
		if entry.Class != ownership.ClassPlatformManaged {
			// Default (non-raised) non-managed class: project_owned seeds
			// (README.md, Makefile, backlog.md…), the platform_armed profile
			// (vh-harness-profile.yml), or external_generated recon data. These
			// legitimately diverge from a fresh render BY DESIGN (operator-curated
			// / armed-and-editable / externally built) and are NOT ownership
			// overrides, so they are silently skipped — never counted as drift or
			// preserved. This restores pre-A2 behavior for the common no-override
			// install (F3 fix: the preserved gate is "origin == override-raise",
			// NOT "effective class != platform_managed").
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

// autoGateOverlayName is the overlay-pack name the auto-classifier config check
// keys on. It matches the overlays: list entry the operator adds in
// vh-harness-profile.yml to opt into the pilot.
const autoGateOverlayName = "auto-classifier-pilot"

// autoGateFile names one of the up-to-4 config files the auto-classifier check
// inspects. level is "project" or "user"; kind is "plugin" (auto-gate-config.json)
// or "llm" (auto-gate-llm.json).
type autoGateFile struct {
	level string // "project" | "user"
	kind  string // "plugin" | "llm"
	path  string // absolute path to the file
}

// label renders a stable, human-readable identifier for a finding: it names the
// level + kind + basename so an operator knows exactly which of the 4 files to
// fix. Project and user files share basenames, so the level prefix is what
// disambiguates them.
func (f autoGateFile) label() string {
	return fmt.Sprintf("%s %s (%s)", f.level, f.kind, filepath.Base(f.path))
}

// checkAutoGateConfig validates the SHAPE (field set + types + enums) of the
// auto-classifier-pilot overlay's config files, then cross-validates the
// EFFECTIVE mode against the EFFECTIVE LLM config. Modeled on
// checkOverlayPermissionState: it is a no-op (SKIP) when the overlay is
// unselected AND no config files are present, and validates each present file
// standalone for shape (it does NOT reimplement the JS coerce/normalize
// logic). Missing optional files are never shape failures — LLM config is
// normally absent (live mode only) and plugin config defaults apply on absence
// — but a PRESENT-but-invalid file (corrupt JSON, wrong type, bad enum) FAILs.
//
// After the shape pass, step 4b applies a two-level (project overrides user)
// merge to compute the effective mode + effective LLM fields and FAILs when the
// selected mode's runtime requirements are unmet (e.g. mode=live with no
// top-level model, mode=live-tiered with no/empty leaves[]). This is a SEMANTIC
// check, not a shape check: it mirrors what the JS plugin requires at runtime
// (see the runtime authority below) so doctor catches mode↔LLM mismatches
// BEFORE the runtime fail-close. It does NOT resolve env vars (an env-var NAME
// counts as present); audit/enforce are exempt (they make no LLM call).
//
// Dual trigger: overlay-unselected + no-files → SKIP (clean no-op);
// overlay-unselected + corrupt-file-present → FAIL (safety net so a stale file
// does not silently break a selected-in-another-worktree config);
// overlay-selected → validate every present file.
//
// It does NOT shell out to the JS plugin (no precedent; doctor's only shell-outs
// are git + a node version probe) and does NOT resolve env vars
// (modelEndpointEnv/apiKeyEnv) — it is a SHAPE lint, not a semantic resolver.
//
// DRIFT CONTRACT — SCHEMA SOURCE OF TRUTH is the JS plugin:
//
//	templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js
//	  - DEFAULT_PLUGIN_CONFIG  (~L376-385): the plugin-config field set + defaults
//	  - DEFAULT_LLM_CONFIG     (~L403-413): the LLM-config field set + defaults
//	  - normalizePluginConfig  (~L480-521): plugin field type/enum rules
//	  - normalizeLlmConfig     (~L543-588): LLM field type/range rules
//	  - RUNTIME MODE REQUIREMENTS (step 4b cross-validation authority):
//	    normalizeLlmConfig (~L541-585) supplies a non-empty default
//	    modelEndpointEnv ("AUTO_GATE_MODEL_ENDPOINT", ~L403) whenever the merged
//	    config lacks one, so the endpoint NAME is never empty post-normalize.
//	    mode=live        -> top-level model (gate ~L1057 "no model"); the
//	                        endpoint check (~L1048) never fires post-normalize
//	                        (the env VALUE is resolved in classifyLive). doctor
//	                        applies the same default (autoGateEffectiveEndpoint)
//	                        so a model-only config is not false-FAILed.
//	    mode=live-tiered -> leaves[] with a well-formed leaf (model + endpoint
//	                        form; default endpoint env applies per leaf)
//	                        (gate ~L1515-1533 "no leaves" when none well-formed)
//	    mode=audit|enforce -> no LLM config read (exempt from cross-check)
//
// This Go check reimplements only the SCHEMA ENVELOPE (field set + types +
// enums), NOT the coerce/normalize logic (e.g. _normNonNegInt accepting a
// numeric string is a JS normalize concern; doctor treats a non-number as a
// wrong-type FAIL). A future JS schema change (field added/removed/retyped or an
// enum widened) MUST be mirrored in the known-field slices (autoGatePlugin/
// LlmKnownFields — the unknown-field WARN source of truth) + the per-field
// switch cases below, AND in the pinning tests (TestAutoGateConfig_Plugin/
// LlmKnownFieldsPinned). The SELF-ENFORCING drift contract is
// TestAutoGateConfig_SchemaParityWithJSSource, which parses the live JS source
// (DEFAULT_PLUGIN_CONFIG / DEFAULT_LLM_CONFIG) and fails if its top-level field
// set diverges from the Go known-field slices — so a JS schema change that
// forgets Go breaks the parity test rather than silently WARN-ing on a valid
// new field. The header comment + those tests are the cross-reference so a JS
// schema change visibly breaks the Go test.
//
// XDG parity: Go's os.UserConfigDir() returns $XDG_CONFIG_HOME (if non-empty)
// else $HOME/.config on Unix — matching the JS userConfigDir()
// (process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config")) then joined
// with "vh-agent-harness". The two diverge only on Windows (Go returns
// %AppData%; JS stays on the XDG/HOME rule), which is outside the Linux/macOS dev
// container this check runs in.
func checkAutoGateConfig(target string) checkResult {
	const name = "auto-classifier"

	// 1. Resolve candidate files (up to 4: project plugin/llm + user plugin/llm).
	//    User-level resolution via os.UserConfigDir(); on error, skip user-level
	//    rather than failing the whole check on an XDG resolution problem.
	var candidates []autoGateFile
	candidates = append(candidates,
		autoGateFile{level: "project", kind: "plugin", path: filepath.Join(target, ".opencode", "repo-configs", "auto-gate-config.json")},
		autoGateFile{level: "project", kind: "llm", path: filepath.Join(target, ".opencode", "repo-configs", "auto-gate-llm.json")},
	)
	if userDir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates,
			autoGateFile{level: "user", kind: "plugin", path: filepath.Join(userDir, "vh-agent-harness", "auto-gate-config.json")},
			autoGateFile{level: "user", kind: "llm", path: filepath.Join(userDir, "vh-agent-harness", "auto-gate-llm.json")},
		)
	}

	// 2. Determine trigger: overlay selection + any-present on disk.
	selected := slices.Contains(activeOverlays(target), autoGateOverlayName)
	anyPresent := false
	for _, c := range candidates {
		if isRegularFile(c.path) {
			anyPresent = true
			break
		}
	}

	// 3. Short-circuit: unselected + no files → clean no-op.
	if !selected && !anyPresent {
		return checkResult{name: name, tier: tierSkip,
			detail: "overlay not selected; no config files present"}
	}

	// 4. Validate each present file standalone (each file must be independently
	//    well-formed). Accumulate FAIL-level and WARN-level findings across all
	//    present files. Successfully-parsed docs are also retained, keyed by
	//    level, so step 4b can resolve EFFECTIVE (two-level-merged) values for
	//    the mode↔LLM cross-validation. A file that fails to parse is NOT
	//    retained — its shape FAIL already covers it, and there is no doc to
	//    merge.
	var fails, warns []string
	validated := 0
	pluginDocs := map[string]map[string]any{} // level ("project"/"user") -> parsed plugin doc
	llmDocs := map[string]map[string]any{}    // level ("project"/"user") -> parsed llm doc
	for _, c := range candidates {
		if !isRegularFile(c.path) {
			continue // absent optional file — not a failure
		}
		validated++
		raw, rerr := os.ReadFile(c.path)
		if rerr != nil {
			fails = append(fails, fmt.Sprintf("%s: unreadable: %v", c.label(), rerr))
			continue
		}
		var doc map[string]any
		if jerr := json.Unmarshal(raw, &doc); jerr != nil {
			fails = append(fails, fmt.Sprintf("%s: invalid JSON: %v", c.label(), jerr))
			continue
		}
		if doc == nil {
			// JSON `null` decodes to a nil map: treat as not-a-JSON-object.
			fails = append(fails, fmt.Sprintf("%s: top-level value is not a JSON object", c.label()))
			continue
		}
		if c.kind == "plugin" {
			pluginDocs[c.level] = doc
		} else {
			llmDocs[c.level] = doc
		}
		ff, fw := validateAutoGateFile(c.kind, doc)
		for _, m := range ff {
			fails = append(fails, fmt.Sprintf("%s: %s", c.label(), m))
		}
		for _, m := range fw {
			warns = append(warns, fmt.Sprintf("%s: %s", c.label(), m))
		}
	}

	// 4b. Cross-validate the EFFECTIVE mode against the EFFECTIVE LLM config.
	//     Step 4 lints each file's SHAPE independently; it does NOT check that
	//     the LLM config carries the fields the selected mode requires at
	//     runtime. The auto-gate plugin is the runtime authority (see DRIFT
	//     CONTRACT): mode=live reads top-level model+modelEndpoint (fail-closes
	//     "no model"; the "no modelEndpoint" gate never fires post-normalize
	//     because normalizeLlmConfig defaults modelEndpointEnv to
	//     "AUTO_GATE_MODEL_ENDPOINT"); mode=live-tiered reads leaves[]
	//     (fail-closes "no leaves" when no leaf is well-formed); audit/enforce
	//     make no LLM call. The endpoint helpers apply the same default so
	//     doctor does not false-FAIL a model-only config the runtime accepts
	//     (the env VALUE is resolved at call time, not by doctor). This step
	//     catches the mismatch BEFORE runtime, using the same two-level
	//     (project overrides user) layering the plugin applies. It runs only
	//     when at least one plugin config was successfully parsed — no plugin
	//     doc means there is no mode to cross-check (the short-circuit at step
	//     3 already handled the no-files case).
	if len(pluginDocs) > 0 {
		mode := autoGateEffectiveString(pluginDocs, "mode", "audit")
		switch mode {
		case "live":
			model := autoGateEffectiveString(llmDocs, "model", "")
			endpoint := autoGateEffectiveEndpoint(llmDocs)
			if model == "" || endpoint == "" {
				fails = append(fails,
					"mode=live requires LLM config with non-empty model "+
						"(modelEndpoint/modelEndpointEnv default to AUTO_GATE_MODEL_ENDPOINT when omitted); runtime would fail-close "+
						"(run 'vh-agent-harness overlay docs auto-classifier-pilot' for configuration reference)")
			}
		case "live-tiered":
			leaves := autoGateEffectiveLeaves(llmDocs)
			if len(leaves) == 0 {
				fails = append(fails,
					"mode=live-tiered requires LLM config with non-empty leaves[]; runtime would fail-close "+
						"(run 'vh-agent-harness overlay docs auto-classifier-pilot' for configuration reference)")
			} else if !autoGateLeafHasModelAndEndpoint(leaves) {
				fails = append(fails,
					"mode=live-tiered: no leaf in leaves[] has a non-empty model "+
						"(modelEndpoint/modelEndpointEnv default to AUTO_GATE_MODEL_ENDPOINT per leaf); runtime would fail-close "+
						"(run 'vh-agent-harness overlay docs auto-classifier-pilot' for configuration reference)")
			}
		}
	}

	// 5. Aggregate: any FAIL → tierFail; else any WARN → tierWarn; else PASS.
	sort.Strings(fails)
	sort.Strings(warns)
	switch {
	case len(fails) > 0:
		return checkResult{name: name, tier: tierFail, detail: strings.Join(fails, "; ")}
	case len(warns) > 0:
		return checkResult{name: name, tier: tierWarn, detail: strings.Join(warns, "; ")}
	default:
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d config file(s) shape-valid", validated)}
	}
}

// validateAutoGateFile dispatches to the per-kind validator. kind is "plugin"
// (auto-gate-config.json) or "llm" (auto-gate-llm.json). Returns fail-level then
// warn-level findings for the single file (without the file label, which the
// caller prepends).
func validateAutoGateFile(kind string, doc map[string]any) (fails, warns []string) {
	if kind == "llm" {
		return validateAutoGateLlmConfig(doc)
	}
	return validateAutoGatePluginConfig(doc)
}

// autoGatePluginKnownFields is the top-level field set the plugin-config
// validator accepts — the SCHEMA ENVELOPE of DEFAULT_PLUGIN_CONFIG in
// auto-tool-gate.js. It is the single source of truth for the unknown-field
// (WARN) detection; the per-field type/enum rules live in the switch below.
// TestAutoGateConfig_SchemaParityWithJSSource pins this slice against the live
// JS source so a JS schema change that forgets to update Go fails the parity
// test rather than silently WARN-ing on a valid new field.
var autoGatePluginKnownFields = []string{
	"enabled", "mode", "stubVerdict", "promptFile",
	"replyMode", "onUncertain", "harnessContext", "guides",
}

// validateAutoGatePluginConfig lints the field set + types + enums of a parsed
// auto-gate-config.json object (the SCHEMA ENVELOPE only). See the DRIFT
// CONTRACT on checkAutoGateConfig.
func validateAutoGatePluginConfig(doc map[string]any) (fails, warns []string) {
	for k, v := range doc {
		if !slices.Contains(autoGatePluginKnownFields, k) {
			warns = append(warns, fmt.Sprintf("unknown field %q", k))
			continue
		}
		switch k {
		case "enabled", "harnessContext", "guides":
			if _, ok := v.(bool); !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want bool)", k, jsonTypeName(v)))
			}
		case "promptFile":
			if _, ok := v.(string); !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		case "mode":
			if s, ok := v.(string); ok {
				if !slices.Contains([]string{"audit", "enforce", "live", "live-tiered"}, s) {
					fails = append(fails, fmt.Sprintf("%s: bad enum %q (want one of audit|enforce|live|live-tiered)", k, s))
				}
			} else {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		case "stubVerdict":
			if s, ok := v.(string); ok {
				if !slices.Contains([]string{"block", "allow", "fail"}, s) {
					fails = append(fails, fmt.Sprintf("%s: bad enum %q (want one of block|allow|fail)", k, s))
				}
			} else {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		case "replyMode":
			if s, ok := v.(string); ok {
				if !slices.Contains([]string{"once", "always"}, s) {
					fails = append(fails, fmt.Sprintf("%s: bad enum %q (want one of once|always)", k, s))
				}
			} else {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		case "onUncertain":
			if s, ok := v.(string); ok {
				if !slices.Contains([]string{"reject", "passthrough"}, s) {
					fails = append(fails, fmt.Sprintf("%s: bad enum %q (want one of reject|passthrough)", k, s))
				}
			} else {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		}
	}
	return fails, warns
}

// autoGateLlmKnownFields is the top-level field set the LLM-config validator
// accepts — the SCHEMA ENVELOPE of DEFAULT_LLM_CONFIG in auto-tool-gate.js. It
// is the single source of truth for the unknown-field (WARN) detection; the
// per-field type/range rules live in the switch below.
// TestAutoGateConfig_SchemaParityWithJSSource pins this slice against the live
// JS source (see autoGatePluginKnownFields for the full rationale).
var autoGateLlmKnownFields = []string{
	"modelEndpoint", "modelEndpointEnv", "model", "apiKey", "apiKeyEnv",
	"timeoutMs", "maxRetries", "retryDelayMs", "leaves",
}

// validateAutoGateLlmConfig lints the field set + types + ranges of a parsed
// auto-gate-llm.json object (the SCHEMA ENVELOPE only). See the DRIFT CONTRACT on
// checkAutoGateConfig. The `leaves` array is checked SHALLOWLY: each element must
// be a JSON object (non-object → WARN); leaf field types are NOT deep-recurse'd.
func validateAutoGateLlmConfig(doc map[string]any) (fails, warns []string) {
	for k, v := range doc {
		if !slices.Contains(autoGateLlmKnownFields, k) {
			warns = append(warns, fmt.Sprintf("unknown field %q", k))
			continue
		}
		switch k {
		case "modelEndpoint", "modelEndpointEnv", "model", "apiKey", "apiKeyEnv":
			if _, ok := v.(string); !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want string)", k, jsonTypeName(v)))
			}
		case "timeoutMs":
			n, ok := asJSONNumber(v)
			if !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want number)", k, jsonTypeName(v)))
			} else if !(n > 0) {
				fails = append(fails, fmt.Sprintf("%s: must be > 0 (got %v)", k, numberDisplay(v)))
			}
		case "maxRetries", "retryDelayMs":
			n, ok := asJSONNumber(v)
			if !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want number)", k, jsonTypeName(v)))
			} else if n < 0 || n != math.Trunc(n) {
				fails = append(fails, fmt.Sprintf("%s: must be a non-negative integer (got %v)", k, numberDisplay(v)))
			}
		case "leaves":
			arr, ok := v.([]any)
			if !ok {
				fails = append(fails, fmt.Sprintf("%s: wrong type %s (want array)", k, jsonTypeName(v)))
				continue
			}
			for i, el := range arr {
				if _, ok := el.(map[string]any); !ok {
					warns = append(warns, fmt.Sprintf("%s[%d]: not a JSON object (want object)", k, i))
				}
			}
		}
	}
	return fails, warns
}

// asJSONNumber reports whether v is a JSON number. encoding/json unmarshals every
// JSON number into a float64 when decoding into map[string]any, so float64 is the
// only number carrier here. bool / string / nil / object / array all return
// (0,false). An integer-constrained field accepts a float64 with a zero fraction
// part (e.g. 2.0); the fraction check is the caller's responsibility.
func asJSONNumber(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok
}

// jsonTypeName renders the JSON-level type name for a decoded value, used in
// fail messages so the operator sees the JSON type (not the Go type). A Go bool
// is JSON "boolean"; a float64 is JSON "number"; nil is JSON "null"; []any /
// map[string]any are "array"/"object"; everything else falls back to its Go type.
func jsonTypeName(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// numberDisplay renders a numeric value for a fail message. It mirrors the
// original JSON spelling where possible: a float64 with a zero fraction shows as
// an integer (so "2.0" reads "2", not "2.000000"), otherwise the raw value.
func numberDisplay(v any) any {
	if n, ok := v.(float64); ok {
		if n == math.Trunc(n) {
			return int64(n)
		}
		return n
	}
	return v
}

// --- auto-gate mode↔LLM cross-validation helpers (checkAutoGateConfig step 4b) ---
//
// These resolve EFFECTIVE values across the two-level (project overrides user)
// config layering the JS plugin applies at runtime. "Present" means the level's
// doc was successfully parsed AND the field key exists in it. The endpoint
// helpers apply the DEFAULT_LLM_CONFIG default for modelEndpointEnv
// (autoGateDefaultModelEndpointEnv = "AUTO_GATE_MODEL_ENDPOINT") when no level
// contributes an explicit endpoint form — this mirrors the runtime's
// normalizeLlmConfig (auto-tool-gate.js ~L541-585), which supplies that default
// so the live preflight's "no modelEndpoint" check (~L1048) never fires on a
// config that omits an explicit endpoint. They do NOT resolve env vars (an
// env-var NAME counts as present): doctor lints config FILES, so a config that
// passes may still fail-close at runtime if the named env var is unset — that
// is an environment concern, not a config-shape concern. See the DRIFT CONTRACT
// on checkAutoGateConfig.

// autoGateDefaultModelEndpointEnv is the DEFAULT_LLM_CONFIG default for the
// modelEndpointEnv field (auto-tool-gate.js ~L403). The runtime's
// normalizeLlmConfig applies it whenever the merged config lacks a non-empty
// modelEndpointEnv, so the live preflight never fail-closes on a missing
// endpoint NAME (the env VALUE is resolved at call time inside classifyLive).
// doctor mirrors this so a config the runtime accepts (e.g. a model-only
// {"model":"m"} under mode=live) is not false-FAILed. DRIFT: if the JS default
// changes, update this constant — the field-set parity test
// (TestAutoGateConfig_SchemaParityWithJSSource) does NOT pin default values;
// the default-reliance tests exercise the behavior but not the literal.
const autoGateDefaultModelEndpointEnv = "AUTO_GATE_MODEL_ENDPOINT"

// autoGateNonEmptyString returns doc[field] when it is a non-empty string, else
// "". A missing key, a non-string value (the shape validator owns wrong-type
// FAILs), or an empty string all return "".
func autoGateNonEmptyString(doc map[string]any, field string) string {
	if v, ok := doc[field]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// autoGateEffectiveString resolves a string field across the two-level (project
// overrides user) layering. A level contributes its value when its doc was
// parsed AND the field key exists AND the value is a string; a present-but-empty
// string IS returned (it is a present override, even if empty), and a present-
// but-wrong-type value stops the walk and returns def (the shape validator
// already FAILs on the wrong type — do not silently fall through to a user
// value that the wrong-typed project field was meant to override).
func autoGateEffectiveString(docs map[string]map[string]any, field, def string) string {
	for _, lvl := range []string{"project", "user"} {
		d, ok := docs[lvl]
		if !ok {
			continue
		}
		v, ok := d[field]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok {
			return s
		}
		return def // present but wrong type — shape validator owns this
	}
	return def
}

// autoGateEffectiveEndpoint resolves the effective model endpoint across the
// two-level layering, honoring the dual-form fields modelEndpoint (literal URL)
// / modelEndpointEnv (env-var NAME) where the literal wins over the env-var name
// within a level. When no level contributes a non-empty literal or env-var name,
// it returns the DEFAULT_LLM_CONFIG default modelEndpointEnv
// (autoGateDefaultModelEndpointEnv) — mirroring normalizeLlmConfig, which
// supplies that default so the live preflight never fail-closes on a missing
// endpoint NAME. The result is therefore never empty (an env-var NAME always
// applies); the env VALUE is resolved at call time inside classifyLive, so a
// passing config may still fail-close at runtime if the named env var is unset.
func autoGateEffectiveEndpoint(docs map[string]map[string]any) string {
	for _, lvl := range []string{"project", "user"} {
		d, ok := docs[lvl]
		if !ok {
			continue
		}
		if s := autoGateNonEmptyString(d, "modelEndpoint"); s != "" {
			return s
		}
		if s := autoGateNonEmptyString(d, "modelEndpointEnv"); s != "" {
			return s
		}
	}
	return autoGateDefaultModelEndpointEnv
}

// autoGateEffectiveLeaves resolves the effective leaves array across the
// two-level layering. Returns the project-level leaves when present (an explicit
// [] is a present override, even when empty), else the user-level leaves, else
// nil. A present-but-wrong-type value returns nil (the shape validator owns it).
func autoGateEffectiveLeaves(docs map[string]map[string]any) []any {
	for _, lvl := range []string{"project", "user"} {
		d, ok := docs[lvl]
		if !ok {
			continue
		}
		v, ok := d["leaves"]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		return nil // present but wrong type — shape validator owns it
	}
	return nil
}

// autoGateLeafHasEndpoint reports whether a leaf has a non-empty endpoint form,
// applying the DEFAULT_LLM_CONFIG default for modelEndpointEnv when the leaf
// specifies neither a literal modelEndpoint nor an explicit modelEndpointEnv.
// This mirrors normalizeLlmConfig, which normalizes each leaf through the same
// field rules as the top-level config (auto-tool-gate.js ~L577-584) so a
// model-only leaf is well-formed at runtime (its modelEndpointEnv defaults to
// autoGateDefaultModelEndpointEnv).
func autoGateLeafHasEndpoint(leaf map[string]any) bool {
	if autoGateNonEmptyString(leaf, "modelEndpoint") != "" {
		return true
	}
	if autoGateNonEmptyString(leaf, "modelEndpointEnv") != "" {
		return true
	}
	return autoGateDefaultModelEndpointEnv != ""
}

// autoGateLeafHasModelAndEndpoint reports whether any leaf in the array is
// well-formed for runtime use: a non-empty model AND a non-empty endpoint form
// (literal modelEndpoint, explicit modelEndpointEnv, or the DEFAULT_LLM_CONFIG
// default modelEndpointEnv). This mirrors the JS wellFormedLeaves filter
// (auto-tool-gate.js ~L1515-1528) AFTER normalizeLlmConfig has applied defaults
// to each leaf: when no leaf is well-formed, live-tiered fail-closes with "no
// leaves".
func autoGateLeafHasModelAndEndpoint(leaves []any) bool {
	for _, el := range leaves {
		leaf, ok := el.(map[string]any)
		if !ok {
			continue
		}
		if autoGateNonEmptyString(leaf, "model") == "" {
			continue
		}
		if !autoGateLeafHasEndpoint(leaf) {
			continue
		}
		return true
	}
	return false
}
