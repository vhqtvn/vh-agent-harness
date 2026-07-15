---
type: decision
date: 2026-07-16
scope: skill subsystem — visibility, health, staleness mitigation, lifecycle tooling
status: research-complete, planner-ready (Slice 1)
---

# Skill Lifecycle Management — Decision Memo

Derived from read-only deep research over the vh-agent-harness skill subsystem.

## Problem
10 skills ship in the corpus, but a running opencode session sees only 9
(debugging-loop, tdd-loop invisible). Additionally there is no skill
lifecycle CLI tooling, and orphan cleanup is manual.

## D1 — Catalog staleness: root cause (high confidence)
opencode's skill cache is a module-closure Map at refs/opencode/packages/core/src/skill.ts:109-119
keyed by source. list() returns cached content and writes back on first load.
The cache is cleared ONLY by process death. Two refresh paths exist and NEITHER
clears it:
- skill.reload() (= state.reload, skill.ts:123, state.ts:78-85) re-materializes
  the source LIST but not content cache -> discovers new paths, serves stale content.
- config invalidation (config.ts:633-668) re-reads config, may re-register transforms;
  same limitation.
No fs-watch exists (config/watcher.ts is a 7-line stub with only ignore;
explicit open TODO at skill.ts:107-108). .opencode/skills/ is a native
DirectorySource from process start (config/plugin/skill.ts:18-48).
Conclusion: skills are well-formed; the cache is stale. Restart is the only
reliable refresh today.

## Mitigation design space (D1)
- M1 Document "restart opencode after update that touched skills" -> DO (prose, permanent, zero risk)
- M2 update emits restart hint when render diff touched .opencode/skills/ (~15 LoC at update.go:~194) -> DO, cheapest always-correct
- M3 same hint in guide footer (guide.go:~232) -> DO alongside M2
- M4 skill refresh no-op that prints restart -> DEFER (redundant unless skill cmd added, then bundle)
- M5 harness writes a sentinel opencode reads -> REJECT (opencode has no consumer)
- M6 render a config knob to force invalidation -> REJECT (no knob exists in watcher schema)
- M7 upstream patch (clear cache on reload / fs-watch) -> file as upstream issue ref skill.ts:107-108; track as p2 backlog; do not gate

## Slice 1 — "Skill visibility & health" (coalesced)
1. vh-agent-harness skill list — table: name, source(core/overlay-pack), rendered?, frontmatter-valid?
2. vh-agent-harness skill validate — Go port of quick_validate.py (~60 LoC). Checks: SKILL.md exists; YAML parses; name matches ^[a-z0-9]+(-[a-z0-9]+)*$, <=64 chars, equals dir name; description <=1024 chars, no angle brackets; compatibility == opencode.
3. New doctor skill-validity/drift check (9th check) — reuse the validator + renderer; Go-native, no JS/python shell (doctor.go:789-790 precedent).
4. D1 restart hint in update/guide output (M2/M3).
5. Docs (M1): README.agent.md + templates/core AGENTS.core.md note.

### Why split doctor-check vs skill command
doctor = always-run seam-health/drift surface (high discoverability).
skill list/validate = on-demand catalog + authoring lint.
Both call ONE shared internal validator package (no duplication).

### Go port vs wrap python
Reimplement in Go. doctor precedent is Go-native; removes python dependency for
a core health check. Keep quick_validate.py as a skill-creator authoring aid.

### Skill data location
Source: templates/core/.opencode/skills/<name>/ (embed corpus.go:36-37) + overlays.
Rendered: .opencode/skills/<name>/ (platform_managed core / overlay_extension pack).
Provenance: .vh-agent-harness/rendered-outputs.json (per-file digest).
NO prebuilt source->rendered skill map exists; walker must mirror core_manifest.go:51-131.

## Slice B (separate, destructive) — orphan prune (#21)
update --prune-orphans: delete ONLY DestUnchanged orphan files
(internal/renderstate/diff.go:52-73 carries DestinationState + RenderedDigest);
REFUSE DestModified (report for manual rm); never touch non-orphans.
Precedent: uninstall --force (uninstall.go:26,131-179) destructive-gated with
safety floor (never .local//state). Composes with --dry-run. Separate slice —
do NOT bundle with read-only Slice 1.

## Deliberate non-actions
- skill-overlap matrix (#15): NOT needed. Only debugging-loop<->tdd-loop confusable;
  already mutually cross-ref'd. think-mode is a router, not a competitor. Document so not re-proposed.
- upstream cache fix (M7): not a harness slice.

## Permissions
vh-agent-harness * already allow (opencode.jsonc.tmpl:101); the skill tool
frontmatter already allow (L49-51). No permission change needed.

## Cross-cutting risk
Slice 1: low (read-only + output text + 1 doctor check). Slice B: medium
(destructive writes, strong precedent, existing data). Neither touches the
ownership WRITE path. doctor Long help + README.agent.md must update in same change.

## Verification claims
- skill cache module-closure Map never cleared by reload: skill.ts:109-119 + L123
- skill.reload() callable but doesn't clear content cache: plugin/promise.ts:86, plugin/host.ts:209, state.ts:78-85
- watcher has no skill-cache knob: config/watcher.ts (7-line stub)
- no skill CLI command: grep Use: in internal/cli/*.go
- doctor has 8 checks, none validate skills, Go-native: doctor.go:45-66, :789-790
- update emits NO restart hint: grep internal/cli/*.go
- --force destructive-gated precedent: uninstall.go:26,51-52,131-179
- orphan machinery records DestinationState + digest: renderstate/diff.go:52-73, manifest.go
- skills are generic corpus files (no skill manifest): corpus.go:36-37, core_manifest.go:51-131
- quick_validate.py frontmatter-only, portable: templates/core/.opencode/skills/skill-creator/scripts/quick_validate.py

## Promotion targets (when built)
new internal/cli/skill.go, internal/cli/doctor.go, internal/cli/update.go,
internal/cli/guide.go, README.agent.md, templates/core/.vh-agent-harness/AGENTS.core.md,
README.md command table (+skill row).
