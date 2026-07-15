---
type: decision
date: 2026-07-16
scope: client adoption / onboarding — progressive disclosure (NO resolver change)
status: research-complete, planner-ready (Slice 3)
---

# Adoption — Progressive Disclosure Decision Memo

## New-consumer journey & overwhelm point
install.go:87-128 renders the ENTIRE templates/core corpus in one shot: 8 baseline agents,
10 skills, 38 slash-commands, ~411-line AGENTS.md, 12 docs/coordination files.
guide installed-phase (guide.go:167-194) then prints a 9-item CONFIGURATION dump with NO
"what you have" summary and NO recommended first task. THE overwhelm point is post-install,
caused by: (1) volume, (2) no recommended subset, (3) jargon density (overlays, profile enums,
capability IDs, deny-rules assumed familiar). greenfield phase (guide.go:88-120) is the bright spot.

## Key correction: README.agent.md is NOT domain-free-constrained
README.agent.md was NEVER under templates/ (git log --all -- 'templates/**/README.agent*' empty;
not embedded/ownership-classified). It is the source-repo dogfood manual. The domain-free/token
constraint applies ONLY to templates/core/.vh-agent-harness/AGENTS.core.md (consumer manual, 364 lines).
=> README.agent.md restructure is unconstrained.

## Key correction: D4 presets are NOT a defect
profile.go:169-175 + 179-184: coordination/web resolve to nil deliberately ("keeps the enum honest
rather than inventing a fake default"). minimal is already the default (templates/core/.vh-agent-harness/vh-harness-profile.yml:12);
it withholds only 2 capability clusters (gated-commit + debate), NOT the skill/command/doc surface.
The renderer renders the full tree unconditionally; only agent BLOCKS are capability-gated.
The preset enum is the WRONG lever for adoption. Right lever = orientation (guide + docs).

## Per-candidate recommendations
- #3 README.md command table: add ~7-11 missing subcommands. (OWNED BY SLICE 2 — drift gate enforces.)
- #9/#10 guide "what you have" + first task: FEASIBLE & CHEAP. guide.go does zero inventory today;
  os.ReadDir on .opencode/{agents,skills,commands} (live tree, no render). Reuse !st.HasMission
  (guide.go:137) for first task. Add to writeGuide installed branch (guide.go:212-224). ~15 LoC.
  Prefer live-tree count over seamInventory() (seam_inventory.go:27 triggers costly render).
- #12 README.agent.md "Common tasks" (L310-574): 0 H3 sub-headers today; dense prose (NOT a bullet list
  as prior audit said). Add ~5-6 H3 (Execution/Git & commit/Configuration/Diagnostics/Migration). No content rewrite.
- #14 binary-subcommands vs agent-slash-commands: ~27 binary vs 38 slash. 2-line note in README.agent.md
  section "The loop" (L14-23): "vh-agent-harness <cmd> = binary (runs anywhere); /<cmd> = slash (in OpenCode session)."
  Secondary: guide footer (guide.go:232-234).
- #17 overlay new/docs: guide overlay step (guide.go:144-150) add "overlay new <name> scaffolds a pack skeleton".
- #19 docs/coordination index: ALREADY EXISTS (docs/coordination/README.md Canonical State Map). Optional 1-line reading-order hint only. Lowest priority.
- #4/#16 presets/surface-gating: DO NOT touch resolver. Optional 1-line note "presets select agent clusters,
  not skill/command surface". Surface-gating (OPT-2) rejected — high-risk architectural; forces dogfood opt-in churn.

## Coalesced Slice 3 — "Progressive disclosure" (docs/guide-text only)
1. guide.go writeGuide: "You have: agents N . skills M . commands K" (os.ReadDir) + one recommended
   first task + binary-vs-slash footer. (#9/#10/#14-partial/#17-partial)
2. README.agent.md: H3 sub-headers under Common tasks + binary-vs-slash note in section "The loop". (#12/#14)
3. (optional, low) docs/coordination/README.md: 1-line reading-order hint. (#19)
Excluded: #4/#16 preset semantics (separate, mostly no-op docs clarification).
DO NOT edit README.md command table — Slice 2 owns it.

## Verification claims
- default profile is minimal: templates/core/.vh-agent-harness/vh-harness-profile.yml:12
- coordination/web resolve to nil deliberately: profile.go:179-184 + comments 169-175; profile_preset_test.go:47-48
- full corpus renders regardless of profile: find templates/core/.opencode/{skills,commands,agents}
- guide does zero inventory counting: grep guide.go (2 matches are instructional strings)
- README.agent.md never in templates/core: git log --all -- 'templates/**/README.agent*' empty
- Common tasks has 0 H3: sed -n '310,574p' | grep -c '^### ' = 0
- docs/coordination/README.md already an index: docs/coordination/README.md:1-60
- ~27 binary vs 38 slash: grep Use: cli + find commands

## Contradictions corrected (vs first audit)
- README.agent.md domain-free claim: FALSE (not in templates/core).
- "Common tasks flat bullet list": it's dense prose with 0 H3s.
- "D4 3/4 presets nil is a defect": deliberate forward-compat stubs.

## Promotion targets
internal/cli/guide.go (writeGuide), README.agent.md, optional docs/coordination/README.md.
