# Decision: v0.15.0 Consumer Report Dispositions — Maintainer Disposition (TrueAI skill/doctor surfaces + vh-solara `doctor*` wildcard)

**Date:** 2026-07-24
**Status:** Accepted (disposition + record-of-decision + documentation lands in this slice). Both
reports are Class B (repo-class, fully re-derivable in this repo); every structural claim was
re-verified against the actual files and commits before recording a verdict. One report contains a
partial misread that is corrected; both surface real-but-narrow gaps that are tracked (residue +
half-delete UX; `doctor*` inventory tightening) rather than changed in product semantics.
**Supersedes:** none.
**See also:**
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md)
(house style + the "deferred work filed as `.local/coordinator/tasks/` transport, not committed canon" convention).
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(the "model output is a candidate, never transition authority" line this disposition preserves).
[`../../templates/migrations/v0.15.0.md`](../../templates/migrations/v0.15.0.md)
(the release note carrying the `ae5b30d` capability-owned CoreOutputs wording this disposition clarifies).

## Framing

Two consuming projects sent v0.15.0-era reports about the agent harness. Both are **Class B** —
they are about *this* repo's contracts (doctor / skill surfaces, the `read_only` permission
inventory), so every claim was re-derived against the actual Go sources, the embedded corpus, and
the named commits. No claim below depends on un-re-derivable consuming-repo evidence.

A read-only compare-and-plan pass (solution-brief) selected two dispositions:

- **Report A (TrueAI) → A2:** preserve the skill/doctor product semantics; correct one partial
  misread; document the supported no-orphan cleanup path; track the *retirement/residue-UX* gap
  separately. No code change.
- **Report B (vh-solara) → B2:** preserve the `doctor*` read-only family semantics; document the
  family-classification rule and its fail-open caveat in durable sources; park the inventory
  tightening behind a named trigger. Comment-only / docs-only; no behavior change.

The decisions below are recorded as record-of-decision; the documentation that Report B requires
lands in this same slice (listed in the Report B provenance table) so the disposition and the
written rule ship together.

---

# Report A — TrueAI: skill list / skill validate / doctor surfaces

## A.1 Claim verification

| ID | Report claim (condensed) | Verdict | Citation |
|----|--------------------------|---------|----------|
| TA1 | "doctor inspects the rendered corpus, so it agrees with `skill list`" | **PARTIAL MISREAD** | `internal/cli/doctor.go:261-298` `checkSkillValidity` derives its skill set via `renderedSkillDirNames` (every directory under `.opencode/skills/`) — i.e. **DISK**, not the rendered name list. `internal/cli/skill.go:304-336` documents that `renderedSkillNames` (which backs `skill list` / `validateSkills`) filters OUT a directory lacking `SKILL.md`, and that `checkSkillValidity` deliberately does NOT reuse it. |
| TA2 | "`skill list` / `skill validate` and `doctor` inspect the same surface" | **REFUTED (three distinct surfaces, by design)** | (1) `skill list` RENDERED column + `validateSkills` → rendered name surface (filters half-deleted dirs); (2) `doctor` skills check → disk directory surface (FAILs a dir with no `SKILL.md`); (3) catalog/CoreOutputs → declared source surface. `doctor_test.go:1277` `TestRenderedSkillNamesUnchangedForMissing` pins that `skill list` must NOT surface half-deleted skills. |
| TA3 | "A deselected capability leaves orphan files and doctor should catch them" | **CONFIRMED (real gap, narrow scope)** | `internal/resolver/core_outputs.go:11-50` + `internal/resolver/catalog.go:285-302`: a selected→deselected transition leaves the prior on-disk file as **InactiveLivePaths** — "left untouched and exempt from drift" (neither deleted nor flagged). `ae5b30d`. There is **no auto-delete and no HEALTHY-failing orphan signal** for residue. |
| TA4 | "doctor FAILs when a deselected capability leaves residue" | **REFUTED** | Residue is a *known* path (in `AllKnownLivePaths`), exempt from managed-drift and unexpected-drift. doctor does not FAIL on residue; it would only FAIL on a **half-deleted** skill (directory present, `SKILL.md` deleted) — `doctor.go:290-293`. |
| TA5 | "`--prune-orphans` cleans capability residue" | **REFUTED** | `internal/cli/update.go` `--prune-orphans` consumes `report.Orphans` = files with NO known source. Capability residue is a KNOWN path (just inactive), so it is never an orphan and never a prune target. `update.go` removes a whole skill directory only after verifying EVERY file in it is orphaned. |
| TA6 | "A half-deleted skill (dir present, no SKILL.md) FAILs doctor" | **CONFIRMED (deliberate)** | `doctor.go:290-293` `os.IsNotExist → "%s: SKILL.md missing"`; `doctor_test.go:1207` `TestSkillValidity_FailWhenSkillMdMissing`, `:1241` `TestSkillValidity_FailMixedValidAndMissing`, `:1261` SKIP-when-no-dir. The FAIL is the safety contract, not a bug. |

The decisive correction is **TA1/TA2**: doctor inspects **disk directories**, not the rendered name
corpus. The report's conflation of the two surfaces is the root of the residue confusion.

## A.2 Shipped mechanisms already in this problem space

| Commit | Mechanism | State |
|--------|-----------|-------|
| `ae5b30d` | Capability-owned CoreOutputs filter (`internal/resolver/core_outputs.go`, `catalog.go`) — unselected capability files are skipped at corpus-walk; prior on-disk file becomes recognized **inactive residue** (exempt from drift, not auto-deleted). | shipped + active |
| (doctor) | `checkSkillValidity` enumerates directories independently of `renderedSkillNames` so a half-deleted skill is a FAIL, not silently invisible. | shipped + active; **gate ACTS** (FAIL is a health signal) |

## A.3 Reconciliation: three surfaces, one health contract

The surfaces are distinct *on purpose*:

- **Catalog / CoreOutputs** — *declared source* (what a capability owns).
- **`skill list` / `validate`** — *rendered name surface* (filters a half-deleted dir out; this is
  correct for a listing/validate verb).
- **`doctor` skills check** — *disk directory surface* (a dir with no `SKILL.md` is a half-delete,
  which is a health FAIL).

Conflating them is the bug the report tripped over. Reusing `renderedSkillNames` in doctor would
make a half-deleted skill silently invisible, breaking the "missing `SKILL.md` is a FAIL" contract.
The two helpers are kept separate precisely so the safety signal survives.

## A.4 Disposition A2 — preserve product semantics; document the supported path

**WON'T:**
- Weaken the half-delete FAIL (TA6). It is the deliberate safety contract.
- Implement auto-retirement of capability residue without a design (retirement authorization is a
  distinct, harder question than selection — see the `defer-001` card history). This is explicitly
  out of scope.
- Treat `--prune-orphans` as a residue cleaner (TA5). It is not, and implying so would mislead.

**WILL (documentation only, no code change):**
- Document that the supported no-orphan cleanup path is to **remove the whole skill directory**
  (`.opencode/skills/<name>/`), not `SKILL.md` alone. Removing the directory makes the doctor
  skills check SKIP (no dir) → doctor PASSes. To remove *both* the skill dir and the capability's
  agent residue, also remove the agent file (e.g. `.opencode/agents/media-perception.md`).
- Track the *retirement / residue-observability* remainder as a separate, named-trigger item (not
  selection — selection was closed by `ae5b30d`). See §Deferred work.

## A.5 Authority-line engagement

No authority line is crossed. doctor's half-delete FAIL is a *safety gate that acts* (it refuses to
report HEALTHY); the residue exemption is a *recognition* rule (known inactive path). Neither is
weakened. The disposition only documents; it changes no transition.

---

# Report B — vh-solara: the `doctor*` read-only wildcard

## B.1 The question, re-derived

`internal/permconfig/tables.go:503-532` `HarnessReadOnlyCommands` is the canonical, Go-owned
read-only verb inventory emitted as `allow` *after* the broad `"vh-agent-harness *": "deny"` entry
(region 4b) for every `read_only` specialist. Under opencode's `findLast` resolution, these specific
allows override the broad deny. The inventory includes:

```
"vh-agent-harness doctor",
"vh-agent-harness doctor *",
```

The comment block (`tables.go:471-502`) states the v1 inventory is *deliberately conservative* and
names what is EXCLUDED: all mutation verbs, artifact producers, and **broad wildcards (`skill *`,
`overlay *`)**. So `doctor *` (verb + wildcard) is deliberately IN, while `skill *` and `overlay *`
are deliberately OUT. The same verb + verb-* shape is used for the whole read-only lint family
(`docs`, `status`, `guide`, `proposals`, `version`, `example`, `sys-prompt`, `help`, `diff`,
`preflight` — each as scalar and wildcard). This is `ba68c76` (Option C).

The report's concern: `doctor *` would allow any *future* `doctor <subcommand>` if doctor ever grows
a mutator (e.g. `doctor repair`, `doctor write-*`).

## B.2 The three questions

**Q1 — Is the current `doctor*` allow intentional?**
Yes. For the *current* doctor family — which is read-only inspection flags, not mutating
subcommands — `doctor` + `doctor *` is the same pattern used by the entire read-only lint family
(`docs*`, `status*`, `guide*`, …). It is NOT in the `skill *` / `overlay *` exclusion bucket; those
are excluded because `skill` and `overlay` already carry mutating verbs (`overlay new`, future
skill verbs) that must stay denied for read-only specialists. doctor has no mutating subcommand
today.

**Q2 — Is a runtime defense (exec-ro-style) required now?**
No. An `exec-ro`-style runtime confinement is an *optional later* pattern that becomes worth
building only if doctor grows mutators. Today doctor is a pure read-only lint; the read-only
permission inventory is the correct and sufficient defense. The `exec-ro *` allow already ships in
the same inventory (`tables.go:504`) as the precedent for the optional-runtime-defense shape.

**Q3 — Should the `doctor*` inventory be narrowed now as hardening?**
No (parked). Narrowing `doctor *` to an explicit subcommand allowlist today is *hardening against a
future that has not arrived*, not a fix for a present defect. doctor has no mutator, so `doctor *`
cannot leak a mutation today. It is parked behind a named trigger (see §Deferred work) so a future
mutating `doctor` subcommand — or evidence that a permission re-audit was missed — re-opens it.

## B.3 Disposition B2 — document the classification rule; park the tightening

The rule to document (durable sources, this slice):

> A verb is admitted to the read-only inventory as `verb` + `verb *` only when the **entire family
> is currently read-only** (inspection flags, no mutating subcommands). Broad wildcards over
> families that already carry mutators (`skill *`, `overlay *`) are excluded. **Fail-open caveat:**
> the read-only matrix does NOT deny a future subcommand of an admitted verb while `verb *` stays,
> so a future mutating subcommand would inherit the allow unless re-audited. Admission of `verb *`
> therefore carries a standing obligation to re-audit the family if it gains any mutator, repair,
> write, network-egress, or secret-sensitive path.

This is a documentation/decision rule, not a behavior change.

**WON'T:**
- Narrow `doctor *` to an explicit allowlist now (Q3). Hardening, not defect; parked.
- Change `internal/permconfig/tables.go` behavior. The edit there is **comment-only** (states the
  family-RO rule + fail-open caveat at the source of truth).
- Claim the matrix guarantees safety for future doctor mutators. The fail-open caveat is stated
  plainly.

**WILL (documentation lands in this slice):**
- `internal/permconfig/tables.go` — comment-only addition of the family-RO `verb *` rule + fail-open
  caveat (no inventory change).
- `templates/core/.opencode/docs/agents/permission-templates.md` — template 3a: explicit verb-*
  rule statement + fail-open caveat (source of the generated `.opencode/` mirror).
- `README.agent.md` — the read-only verb inventory section: add the family-RO rule + fail-open
  caveat (repo-root hand-maintained file, not a generated mirror).
- `templates/migrations/v0.15.0.md` — a light clarification that (a) capability residue ≠ prune-orphans
  target and (b) whole-directory removal is the supported no-orphan cleanup path.

The generated `.opencode/` mirror of `permission-templates.md` is regenerated by the normal
`vh-agent-harness update` flow; `.opencode/` mirrors are never hand-edited.

## B.4 Authority-line engagement

No authority line is crossed. The read-only inventory is a *permission gate* (it denies mutation
verbs). Documenting its classification rule and a fail-open caveat strengthens the gate's
auditability; it removes no deny, grants no new allow. Narrowing is parked, not done, so no
specialist loses access.

---

# Deferred work (named triggers)

Filed as cards in `.local/coordinator/tasks/` (transport, not committed canon):

- **`defer-skill-capability-residue-observability.json`** (Report A, NEW) — residue/retirement-UX
  remainder: there is no HEALTHY-failing signal or doc-driven observability for capability residue
  short of careful whole-directory removal. *Trigger:* another post-documentation material confusion
  report about deselection residue, OR a dedicated retirement proposal is prepared (which would also
  re-open `defer-001`'s retirement track).
- **`defer-doctor-wildcard-inventory-tighten.json`** (Report B, NEW) — narrow `doctor *` to an
  explicit read-only subcommand allowlist. *Trigger:* doctor gains ANY subcommand or mutating mode
  (repair / write / network / secret-sensitive path), OR evidence that a permission re-audit was
  missed.
- **`defer-001-skill-capability-render-gate.json`** (Report A, UPDATED) — the **selection** track is
  **closed by `ae5b30d`** (capability-owned files now respect selection). What remains is the
  **retirement/residue-UX** remainder, surfaced again by this TrueAI report. Do not present selection
  as open work.

# Rejected

- **Weaken the half-delete FAIL.** It is the deliberate safety contract; making a half-deleted skill
  silently invisible to the health gate would regress the doctor skills check.
- **Auto-retirement of capability residue without a design.** Retirement authorization is a distinct,
  harder question than selection (it must respect local adoption / ownership-class); implementing it
  ad hoc would cross the authority line.
- **Conflate `--prune-orphans` with residue cleanup.** Residue is a known path, not an orphan.

# Disagreements with the reports' framing (stated, with reasons)

1. **Report A conflates doctor with `skill list`.** doctor inspects disk directories; `skill list`
   inspects the rendered name surface. The two are separate *on purpose* (TA1/TA2).
2. **Report A implies deselection alone should FAIL doctor.** Only a half-deleted skill FAILs; clean
   residue is exempt by design (TA3/TA4).
3. **Report B frames `doctor *` as a present defect.** It is the family-RO admission pattern,
   identical in shape to `docs*`/`status*`/`guide*`; doctor has no mutator today (Q1). The real item
   — inventory tightening — is hardening, parked behind a trigger (Q3).
4. **Report B implies the matrix should guarantee safety for future doctor mutators.** It cannot,
   while `verb *` stays; the fail-open caveat is documented plainly rather than implied away.

# Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| TA1/TA2 doctor = disk dirs, not corpus | `internal/cli/doctor.go:261-298` (`renderedSkillDirNames`); `internal/cli/skill.go:304-336` (why the two helpers differ) | yes |
| doctor half-delete FAIL deliberate | `doctor.go:290-293`; `doctor_test.go:1207,1241,1261,1227,1277` | yes |
| TA3/TA4 residue recognized, not auto-deleted | `internal/resolver/core_outputs.go:11-50`; `catalog.go:285-302`; `core_outputs_test.go` (`TestCompileCoreSelectionPlan_*` incl. `:162` media-unselected residue) | yes |
| TA5 prune-orphans ≠ residue | `internal/cli/update.go` (`report.Orphans` consumer; whole-dir delete only after every file orphaned) | yes |
| CoreOutputs shipped | `git show --stat ae5b30d` (`feat(core): add capability-owned CoreOutputs filtering for media-perception`) | yes |
| B.1 `doctor` + `doctor *` in RO inventory; `skill *`/`overlay *` excluded | `internal/permconfig/tables.go:471-532` | yes |
| read_only policy shipped | `git show --stat ba68c76` (`feat(permconfig): add read_only HarnessPolicy for RO specialists`) | yes |
| docs lands this slice | `internal/permconfig/tables.go` (comment-only), `templates/core/.opencode/docs/agents/permission-templates.md` (3a), `README.agent.md` (RO inventory), `templates/migrations/v0.15.0.md` (residue clarification) | yes |

House style: bolded-metadata frontmatter + Framing → Verification → Reconciliation → Disposition →
Authority-line → Deferred → Rejected → Evidence, following the `2026-07-05` / `2026-07-22` /
`2026-07-23` convention (not YAML frontmatter).
