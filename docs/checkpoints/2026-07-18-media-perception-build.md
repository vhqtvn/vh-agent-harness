# Checkpoint: media-perception build slice (2026-07-18)

**Session role:** build subagent (no OpenCode session binding)
**Goal:** Implement locked `media-perception` slice (1 agent + 1 skill, provider-neutral, opt-in capability `core/media-perception`); validate; regenerate; report for commit routing. DO NOT COMMIT.
**Status:** implementation complete + validated; awaiting commit routing
**Next step:** route to committer agent (gated-commit). Live no-refusal dogfood run PENDING operator backend availability.

## Goal
Implement the locked `media-perception` slice for vh-agent-harness core: one opt-in agent + one caller-facing skill, provider-neutral and discovery-oriented, with the no-refusal acceptance signal as behavioral proof. Validate, regenerate rendered outputs, and report for commit routing. DO NOT COMMIT — route to committer.

## Files added (NEW source) — 2
- `templates/core/.opencode/agents/media-perception.md` — read-only perception specialist; no-refusal signal (must scan session capabilities before any refusal); two backend modes (model-native, tool-orchestrated); `available`/`unavailable`/`uncertain` reporting; consolidated report shape (capability_status, input, basis, tools_used, observations, limitations, next_action); path/url handoff contract.
- `templates/core/.opencode/skills/media-perception/SKILL.md` — caller-facing two-path guidance (Path A in-context perception for vision-capable callers; Path B single-delegation to media-perception for text-only callers OR heavy/multi-step); capability-class vocabulary (image/OCR, diagram, chart, video, document/PDF, audio); overlay add-a-leaf recipe.

## Files edited (source) — 6
- `internal/resolver/catalog.go` — registered `core/media-perception` CapabilityManifest (Provides: media-perception; self-contained; no HardDeps).
- `internal/permconfig/tables.go` — added CoreLocationRules leaf rule for media-perception (read-only, gate: Deny, NOT gate-exempt); added `{"media-perception", Allow}` to CoreTaskRules[build/coordination/project-coordinator/researcher]; added `"media-perception": {{"*", Deny}}` deny-all leaf entry.
- `templates/core/opencode.jsonc.tmpl` — added media-perception agent block (wrapped `{{- if .capabilities.media_perception }}...{{- end }}`) after ship-review; added `{{- if .capabilities.media_perception }}"media-perception": "allow"{{- end }}` to 4 callers' task blocks.
- `templates/core/.opencode/docs/agents/callable-graph.md` — added media-perception to read-only specialists roster; new "Opt-in perception routing" section (4 inbound caller edges, path/url handoff, capability_status contract).
- `templates/core/.vh-agent-harness/AGENTS.core.md` — added media-perception entry to specialist list (opt-in via core/media-perception capability, path/url handoff, read-only).
- `README.agent.md` — added "Opt-in core capabilities" subsection + 4 sub-blocks: model seed behavior, missing-tool vs broken-model-reference distinction, integration recipe (overlay/operator), no-refusal acceptance procedure (UNVERIFIED without backend).

## Files edited (tests) — 4
- `internal/resolver/catalog_test.go` — TestCoreCatalog_SeedCapabilities expects 3 capabilities [core/debate, core/gated-commit, core/media-perception]; added TestCoreCatalog_MediaPerceptionProvides.
- `internal/permconfig/emit_test.go` — added TestEmit_MediaPerceptionPresentKeepsInboundEdges (positive), TestEmit_MediaPerceptionAbsentDropsInboundEdges (negative), TestEmit_MediaPerceptionLeafWildcardAlwaysPreserved; updated TestEmit_PresentAgentFilterNoopWhenAllPresent fixture.
- `internal/cli/capability_render_test.go` — added TestSeamRender_MediaPerceptionOptInCapabilityRenders (minimal + capabilities:[core/media-perception] → 10 agents, 4 caller edges, deny-all task map), TestSeamRender_MediaPerceptionUnselectedDropsEdges (supervised preset → 21 agents, no dangling edges).
- `internal/resolver/release_overlay_test.go` — **DEVIATION**: forced mechanical update of hardcoded count from 3 → 4 (TestMergeCatalogs_ReleasePackShapeMirrorsEmbedded) with comment. Outside locked surface; documented as deviation.

## Test files NOT edited (confirmed no change needed)
- `internal/substrate/embed_renderer_test.go` — TestEmbedFSRenderer_RealCoreCorpusResolvesHarnessTokens walks whole corpus for unresolved sentinels; new files automatically pass.
- `internal/resolver/merge_test.go` — no capability-merge logic touched.

## Rendered (via `vh-agent-harness update`; do NOT hand-edit)
- `.opencode/agents/media-perception.md` (NEW)
- `.opencode/skills/media-perception/SKILL.md` (NEW)
- `.opencode/docs/agents/callable-graph.md` (+24)
- `.vh-agent-harness/AGENTS.core.md` (+1)
- `.vh-agent-harness/lineage.yml` (digest refresh)
- `.vh-agent-harness/rendered-outputs.json` (digest refresh)
- `AGENTS.md` (+1)
- `README.agent.md` (+99)

NOTE: `opencode.jsonc` was NOT modified because the repo's `supervised` profile doesn't select `core/media-perception`, so the agent block correctly did NOT render. The 4 managed-overwrite files are the docs files whose source was edited + `AGENTS.md` compose.

## Verification
| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| gofmt clean on changed Go files | `gofmt -l internal/ cmd/` → empty | yes |
| go vet clean | `go vet ./...` → no output | yes |
| go build clean | `go build ./...` → no output | yes |
| Binary builds | `go build -o tmp/vh-agent-harness-new ./cmd/vh-agent-harness` → 16632880 bytes | yes |
| All tests pass | `go test ./...` → ok 17+ packages, no failures | yes |
| Update dry-run preview works | `./tmp/vh-agent-harness-new update --dry-run` → 4 managed-overwrite, 154 unchanged, 9 project-preserved, 1 armed-merged | yes |
| Dogfood update applies cleanly | `./tmp/vh-agent-harness-new update` → APPLIED, 4 files overwritten | yes |
| Doctor healthy | `./tmp/vh-agent-harness-new doctor` → 0 problems, 0 warnings; 11 rendered skills valid (was 10, now includes media-perception); 38 `{file:}` refs resolve | yes |
| Domain-free audit (no vendor literals in core) | `rg -c "zai-mcp-server|analyze_image|npx|tesseract|ffmpeg|pdftotext|pdftoppm" <all touched source files>` → ALL_CLEAN | yes |
| Sub-task 1: model seed mechanics | Verified via repo's supervised profile: `.local/config/agent-model/media-perception` NOT created because capability unselected → no model ref in rendered opencode.jsonc → `seedAgentModelDefaults` has nothing to match | yes (mechanics); operator-flow not yet run on a capability-selected repo |
| Sub-task 4: skill registration (skills auto-discovered, no catalog entry needed) | Verified empirically: under profile:minimal (no capabilities), ALL agent prompt files AND skills render; capability gate controls ONLY opencode.jsonc agent-block + present-agent-filtered task edges | yes |
| Live no-refusal acceptance test | No perception backend available during build | **NO — UNVERIFIED (pending operator-backed validation)** |

## Findings
- **Skill capability-gating IMPOSSIBLE without new mechanism** (confidence=high, type=fact): locked surface required skill to "render conditionally with capability"; empirically the EmbedFSRenderer walks `templates/core/` unconditionally and renders every file. Skill leak to disk when unselected is BENIGN (opencode only loads agents registered in `opencode.jsonc`). Documented as deviation; proceeded with the existing pattern (consistent with every other capability).
- **emit.go unchanged** (confidence=high, type=fact): locked-surface prediction held — present-agent filter (`computeTaskBlock` line 361) drops task edges to absent agents automatically.
- **Capability key derivation** (confidence=high, type=fact): `core/media-perception` → `media_perception` template gate via `capabilityTemplateKey`.

## Contradictions
- **Pre-existing (OUT OF SCOPE, NOT fixed)**: callable-graph.md's "Delegation ownership" section is narrower than tables.go's actual caller set. Documented in closeout; not fixed per task instruction.
- **Pre-existing (OUT OF SCOPE)**: no canonical ROLES.md/LANES.yaml.
- **Pre-existing (OUT OF SCOPE)**: future media breadth exceeds verified runtime support.

## Sub-task outcomes
1. **Pre-agent vs agent-runtime absence split**: DONE via existing mechanisms. Seed via `seedAgentModelDefaults` (creates empty `.local/config/agent-model/<name>` when capability selected); doctor `config-refs` check FAILs on missing ref file (actionable: "run vh-agent-harness update"), WARNs on empty ref file (actionable: "set a model id"). Agent CANNOT catch pre-invocation config failure. Documented in README.agent.md.
2. **Provider-neutral no-refusal prompt**: DONE. Agent identity-rule section explicitly requires session-capability scan BEFORE any refusal; lists prohibited phrases ("I have no vision capability" etc).
3. **Skill two-path guidance**: DONE. SKILL.md has Path A (in-context) vs Path B (single-delegation) with handoff contract and report-handling expectations.
4. **Skill registration**: DONE — auto-discovery confirmed (skills render unconditionally per EmbedFSRenderer; no catalog entry needed). Deviation from locked surface (which required conditional skill rendering) documented.

## Backlog row
No active or archived backlog row exists for `media-perception` (verified via grep on `docs/planning/backlog.md` and `docs/planning/archive/`). Coordinator will decide whether to add one.

## Acceptance signal status
**UNVERIFIED (pending operator-backed validation).** No perception backend (e.g., z.ai MCP) was available during build. Per task spec, NOT claimed passed from prompt inspection alone. The prompt's no-refusal signal is established structurally (identity rule + prohibited phrases + capability-scan-before-refuse) but the behavioral proof requires a live run in a capability-selected, operator-wired session.

## Verification scope (accuracy clarification — added 2026-07-22)

This section was added in a later closeout pass to make the *breadth* of the
2026-07-18 verification explicit. The checkpoint above already marks the live
behavioral signal UNVERIFIED; the notes below sharpen what the render-test
coverage DID and DID NOT exercise, so a reader does not over-read the "yes"
rows in the Verification table as live behavioral proof.

- **Modality breadth = image sub-classes only.** The render/structural tests
  proved the capability renders and routes, but no live perception backend was
  driven across modalities. What was structurally covered amounts to image
  sub-classes (chart/data-viz, screenshot OCR, natural image). **Video, PDF,
  and audio were NEVER tested** — not even structurally beyond the prompt's
  vocabulary. Do not read any "yes" row as a 3/3 modality PASS; it is a 3/3
  image sub-class PASS at most, with video/PDF/audio never exercised.
- **Caller coverage = anti-refusal/delegation only, not perception.** Two
  caller roles were reasoned about (coordination-style and build-style). The
  build-caller path timed out on every perception attempt (no backend), so
  only the anti-refusal behavior was proven (the caller delegated instead of
  self-refusing) — **perception itself was never observed from the build
  caller**. The coordination caller's delegation was reasoned structurally.
  This is a 2/2 caller *anti-refusal* PASS, not a 2/2 caller *perception* PASS.
- **`researcher` and `project-coordinator` caller routing = render-tested
  only.** The inbound caller edges for these two roles were verified via the
  emit/render tests (edges present when capability selected, dropped when
  unselected) but were **never exercised live**.
- **capability-OFF behavior (disabled branch) = render-test coverage only.**
  The unselected-capability path (agent block absent, task edges dropped) was
  validated by render tests; **no live behavioral validation** of the
  disabled branch was performed.

**Verification thread remains OPEN — not all claims are fully validated.**
Anything beyond render/structural coverage requires a live, operator-wired,
capability-selected run.

## Next step
Hand off to coordinator for routing to `committer` agent (gated-commit protocol). Suggested commit split:
- (a) code commit: `templates/core/` + `internal/` source + tests
- (b) rendered-output commit: `.opencode/` + `AGENTS.md` + `README.agent.md` + `.vh-agent-harness/lineage.yml` + `.vh-agent-harness/rendered-outputs.json` + `.vh-agent-harness/AGENTS.core.md`

Coordinator to confirm commit-message lane and route. Build artifacts `tmp/vh-agent-harness-new` and `tmp/minimal-test/` should be cleaned via `/job-cleanup` at session end (not part of commit).

## Build artifacts (CLEAN UP at session end, NOT committed)
- `tmp/vh-agent-harness-new` (16.6MB binary; built from current source for dogfood validation).
- `tmp/minimal-test/` (temp install dir used to verify skill/prompt-file unconditional rendering during Sub-task 4).
