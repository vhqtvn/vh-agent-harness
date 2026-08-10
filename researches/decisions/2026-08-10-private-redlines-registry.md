# Decision: Private Redlines Registry — machine-local sensitivity notes with cross-project scope

**Date:** 2026-08-10
**Status:** Proposed (input to downstream research / solution-brief / debate; challengeable, not binding)
**Basis:** codebase seams cited inline; precedents: `.gitignore:32` (`*.local.md` "Local-only notes (sensitive; never commit)"), `templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js:352-388` (XDG user-config dir + four-level ladder), `researches/decisions/2026-07-26-repo-mail-protocol.md` (anonymization constraint + `privateDenyRules` seam), `internal/cli/doctor.go:2143+` (`checkAutoGateGitignored` tiering)
**Supersedes:** none
**See also:** [`./2026-07-26-repo-mail-protocol.md`](./2026-07-26-repo-mail-protocol.md), [`./read-isolation-policy.md`](./read-isolation-policy.md)

## Framing

Two kinds of operator knowledge must shape every commit on a machine yet can never
themselves be committed anywhere:

- **K1 — scrub-projects.** Projects whose material (docs, code, data, naming) must be
  scrubbed before anything derived from them lands in any git history. Typical shape:
  a repo forked from a sensitive upstream whose original artifacts must never be
  copied forward.
- **K2 — forbidden-relations.** Relations between two fact-sets (e.g. an org identity
  and a technical domain) where each side may be individually mentionable, but their
  *co-occurrence in committed artifacts* would let a reader infer a private relation.

Both are inherently **cross-project**: the fact exists once per machine and applies in
every repo the operator touches. Today this knowledge lives only in operator memory and
assistant memory notes — no machine-checkable form, nothing adopters can install.

**Meta-constraint (self-demonstrating):** this memo and everything shipped in
`templates/core/` must stay domain-free. All concrete entries live only in private
files. The instantiated motivating examples for this memo live in the gitignored
companion `2026-08-10-private-redlines-registry.local.md`.

## Decision 1 — Storage: user-level registry, plus optional repo-local binding

| Option | Where entries live | Cross-project? | Verdict |
|---|---|---|---|
| A. Per-repo local file only | `.vh-agent-harness/redlines.local.yml` in each repo | No — every repo re-states the fact; a new clone starts unprotected | Reject |
| B. User-level registry only | `$XDG_CONFIG_HOME/vh-agent-harness/redlines/registry.yml` | Yes — one source of truth | Almost |
| C. **B + optional repo-local binding** (recommended) | Registry at user level with `repos:` matchers; `.vh-agent-harness/redlines.local.yml` may add repo-specific entries or tighten scope | Yes, with per-repo additions | **Accept** |

Precedent: the documented four-level ladder `project-local > committed-project > user
(XDG) > defaults` (`auto-tool-gate.js:2773-2830`). Here the "committed-project" level is
*structurally absent by design* — there is no committable form of this config.

Registry entries carry `repos:` matchers (path globs and/or remote patterns) so a repo
is protected the moment it exists on the machine, with no per-repo setup. The repo-local
file is an optional override, never a prerequisite.

File permissions: registry files written/expected 0600, same posture as
`origin-hashes.json`.

## Decision 2 — Schema (Contract A)

> **[VERBATIM — downstream slices reproduce exactly, do not summarize]**

```yaml
# $XDG_CONFIG_HOME/vh-agent-harness/redlines/registry.yml   (0600, never in any repo)
version: 1
subjects:
  - id: subj-<opaque>          # opaque id; the ONLY token safe to echo in
                               # diagnostics, doctor output, and committed text
    kind: scrub-project
    labels: [<names, aliases, path fragments identifying the project>]
    source_repos: [<path/remote globs of the sensitive origin>]
    repos: [<globs of repos where this rule is enforced>]   # omit = all repos
    policy: scrub-before-commit
    why: <one line, private>

  - id: subj-<opaque>
    kind: forbidden-relation
    side_a: [<termset A — e.g. org names/aliases/domains>]
    side_b: [<termset B — e.g. domain/technique terms>]
    ambient_repos: [<globs of repos where side A is ambient>]
      # In matching repos, side A is implied by repo identity itself
      # (org-owned repo), so the rule degenerates to: side B terms are
      # banned outright in committed artifacts.
    repos: [<globs where the co-occurrence rule is enforced>]  # omit = all
    unit: file | diff            # co-occurrence window; default: both
    why: <one line, private>
```

```yaml
# .vh-agent-harness/redlines.local.yml   (gitignored; optional per-repo layer)
version: 1
extends: user-registry           # always; local file cannot suppress user entries
subjects: [...]                  # same shapes; additions/tightening only
```

Semantics pinned:

- **scrub-project** hit = any `labels` token or `source_repos`-relative path fragment
  appearing in committed artifacts (file contents, paths, commit messages, branch names).
- **forbidden-relation** hit = ≥1 side-A term AND ≥1 side-B term co-occurring within one
  `unit`; in `ambient_repos`, a side-B term alone is a hit.
- Local layer is **additive/tightening only** — it can never weaken or mask a user-level
  entry (mirror of the raise-only rule in `internal/ownership/doc.go`).

## Decision 3 — Enforcement: four surfaces, defense in depth, no git hooks

Hard constraints respected: the harness installs no git hooks and never auto-edits
`.gitignore` (README.agent.md ~439). All enforcement is doctor/agent-gate/verb shaped.

| Surface | Mechanism | Role |
|---|---|---|
| 1. Agent context (prevention) | `redlines compile` materializes `.vh-agent-harness/docs/redlines-guidance.local.md` (gitignored) and it is appended to `opencode.jsonc` `instructions[]` via the existing PROJECT slot (`opencode.jsonc.tmpl:16-22`); same pattern as `materializeContextDocs` (`internal/cli/docs.go:141-167`) | Agent knows the redlines *before* writing anything |
| 2. Runtime deny rules (gate) | `redlines compile` also emits `.opencode/repo-configs/forbidden-patterns.redlines.local.js` (gitignored) — `{id, test, why}` rules: token detectors for K1, co-occurrence/ambient detectors for K2. Plugs into the `forbidden-patterns.js` aggregator and **implements repo-mail's so-far-unimplemented `loadPrivateDenyRules()`** (`repo-mail-egress-wiring.js`) | Egress and tool-call surfaces reject hits mechanically |
| 3. Commit gate (backstop) | New verb `vh-agent-harness redlines scan --staged` (also `--range`, `--paths`) scans staged diff + commit message + branch name; commit-reviewer contract (`AGENTS.core.md:267`, commit-reviewer agents) gains one line: run the scan when redlines apply to this repo; any hit ⇒ disposition=block | Nothing reaches history unchecked |
| 4. Doctor (hygiene) | New check modeled on `checkAutoGateGitignored` (`doctor.go:2143+`): (a) registry perms 0600; (b) `*.local.*` redlines files effectively ignored via `git check-ignore -v`, portable vs non-portable source distinguished; (c) **tracked-despite-ignored ⇒ FAIL**; (d) redlines exist for this repo but compiled outputs stale/missing ⇒ WARN with exact command | Detects drift; never repairs silently |

Deliberately not built: repo-history rewriting, `.git/hooks` installation, any
committed allowlist/denylist.

## Decision 4 — Context exposure: full terms in local agent context

| Option | Prevention strength | Leak surface | Verdict |
|---|---|---|---|
| Opaque ids only in context; agent must run `redlines scan` to learn anything | Weak — the agent cannot avoid pairing terms it does not know | None | Reject |
| **Full terms + rules in the materialized local guidance doc** (recommended) | Strong — avoidance at generation time | Local disk only; file is gitignored + doctor-guarded; surfaces 2–3 backstop any echo | **Accept** |

Boundary argument: the repo-mail anonymization constraint governs *committed/published/
deployed* content, not local agent context. Local context is not egress. An agent that
does not know the forbidden relation will eventually write one side of it next to the
other by accident; prevention requires disclosure at the generation boundary, with
mechanical gates behind it.

## Decision 5 — Relation semantics scope (v1 honesty line)

v1 detects **lexical co-occurrence per unit** (file, diff) plus the **ambient-repo
degeneration** (Decision 2). It does not detect paraphrase, translation, or multi-file
inference chains in non-ambient repos. This is stated in the compiled guidance doc
verbatim (heuristic-honesty precedent: `auto-gate-scrub.js:76-95` header). An optional
LLM-assisted deep scan (`redlines scan --deep`) can follow the auto-gate LLM pattern
later; it is out of scope here.

## Contract B — CLI surface

```
vh-agent-harness redlines status    # which subjects bind this repo (ids + kinds only)
vh-agent-harness redlines compile   # (re)materialize guidance doc + local deny rules
vh-agent-harness redlines scan      # --staged | --range A..B | --paths ... | --tracked
```

- `compile` also runs automatically at the end of every non-dry-run `install`/`update`
  apply when any subject binds the repo (same hook point as `materializeContextDocs`).
- All human-visible output uses opaque `subj-*` ids, never labels/terms — so scan
  results can be pasted into issues/PRs safely.
- Exit codes: `scan` returns non-zero on any hit (block-grade for the commit gate).

## Ownership & corpus posture

- Generated/authored redlines files (`redlines.local.yml`, compiled outputs, guidance
  doc) stay **entirely off the seam corpus** — they are runtime-local artifacts like
  `.opencode/state/`, so no lattice change and no `core_manifest.go` exception is
  needed in v1. (If later classified, `local_only` is the obvious class — it exists,
  is off-lattice, and already has the `ignored-local-only` dry-run outcome.)
- Adopter `.gitignore` seed (`project_owned`, seeded once) gains the redlines patterns
  for **new** adopters; **existing** adopters get a doctor WARN listing the exact lines
  to add — never an auto-edit.
- Feature-flagged: `redlines` under `features:` in `vh-harness-profile.yml`
  (`platform_armed`), gating one short section in `AGENTS.core.md` via the existing
  `{{ if .features.X }}` template path (`internal/cli/seam.go:586+`).

## Non-goals (v1)

- Cross-machine registry sync (operator's private dotfiles problem, explicitly not the
  harness's; the registry must never ride in any project repo).
- Copy-provenance tracking ("this paragraph originated in source repo X").
- Semantic/paraphrase inference detection in non-ambient repos (Decision 5).
- History scrubbing of already-committed hits (report via `scan --tracked`; remediation
  is an operator decision).

## Proposed resolutions (challengeable)

1. Verb/feature name: **`redlines`** (neutral, covers both kinds).
2. `scan --staged` placement: **both** — a mandatory line in the staged-commit
   contract (so headless commits hit it) *and* invocation from the commit-reviewer
   agents (disposition=block on any hit).
3. Matcher vocabulary for `repos:`/`ambient_repos:`/`source_repos:`: **both** path
   globs and normalized git-remote patterns (remotes survive re-clones; paths cover
   remoteless scratch repos).
4. Term matching: case-insensitive, on word-ish boundaries; behavior documented in
   the compiled guidance doc. Anything smarter is Decision 5 territory (out of v1).
