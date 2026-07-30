# Contract/Invariant Audit — Bounded Dogfood Pilot Run

**Date:** 2026-07-30
**Capability:** `contract-invariant-audit` (overlay-only S1 pilot)
**Skill commit range authored:** `9477687`..`f6889e1`
**Run anchor:** `50f2da8` (HEAD at run time)
**Mode:** bounded slice (declared-inventory)
**Authority:** INFORMS only — nothing here gates any commit/release/doctor/update. This run does NOT promote the skill to S2.

> This is a source packet produced by a single bounded pilot run. It is advisory
> evidence feeding a future, separate S2-promotion decision. It is intentionally
> NOT committed; the operator reviews/commits/revises it.

---

## 1. Chosen slice + rationale

**Boundary:** `internal/ownership/` — the S2 update-safety classification surface
and the D2-A raise-only lattice. Seven Go files (4 source, 2 test, 1 doc).

**Rationale (chosen over `internal/runshape/`):**

1. **Invariant-richness.** It is the safety contract (the config-authority §S2
   model). The package doc declares hard guarantees: a raise-only lattice, four
   fail-closed error classes, off-lattice rejection, and "never silently disarm a
   path." That is prime C3/C4/C5 territory per the design brief.
2. **Dormancy (the distinctness precondition).** Every file in the package was
   last touched 2026-06-27 / 2026-06-30; none appears in the last 15 commits
   (which are all 2026-07-28+, covering overlay/decision/runshape/exec-sandbox
   work). `internal/runshape/` was touched in the most recent commit window
   (`a2e8d3d`, 2026-07-29) and would skew commit-review-duplicate. The ownership
   package gives the distinctness A/B test a genuine blind spot: commit-review
   literally cannot see this surface right now.
3. **Tractable size.** 7 file-units — small enough for a complete declared
   inventory, large enough to host real cross-unit invariants.

---

## 2. Manifest summary

| Field | Value |
|---|---|
| anchor (ref / resolved) | `HEAD` / `50f2da8dc4ae286685e722367d8eb9a3eafb3c47` |
| anchor_basis | `git-ls-tree-at-anchor` |
| roots | `internal/ownership/**` |
| exclusions | none |
| granularity | `file` (deterministic fallback) |
| discovery adapter | none (fallback file inventory) |
| coverage tier | **`declared-inventory`** (NOT adapter-complete) |
| shard strategy | unsharded |
| units manifested | 7 |
| enumeration evidence | `git ls-tree -r --name-only 50f2da8…` filtered by roots/exclude |

**Coverage-tier limit (explicit):** the file fallback enumerates files, not Go
declarations, callables, or cross-unit contracts. The manifest is complete as a
*file inventory*; it is NOT a declaration/transition/semantic-unit enumeration.
Cross-unit invariants were reached by manual reading across the 7 files, not by
manifest enumeration. `unsupported_surfaces` recorded by the helper: *"non-file
units (declarations, transitions, cross-unit contracts) are NOT enumerated by the
file fallback."*

Manifested units (unit_id → locator):

| unit_id | locator |
|---|---|
| `e429e0ab1f98e5b0` | internal/ownership/class.go |
| `99050f8160fbaecb` | internal/ownership/ordering.go |
| `9fec91e87a1fbcc9` | internal/ownership/resolve.go |
| `669d949660fad4be` | internal/ownership/doc.go |
| `587d7619d185de1c` | internal/ownership/errors.go |
| `567c81ce7d4812a8` | internal/ownership/resolve_test.go |
| `ff40e5faf817103b` | internal/ownership/ordering_test.go |

Determinism: `--self-test` passed (byte-identical manifests on identical inputs);
the helper pinned the inventory to the resolved anchor tree, not the live index.

---

## 3. Disposition accounting

Helper-enforced completeness (`complete` command): **status = `complete`**,
7 manifest units / 7 dispositioned, anchor_status = `matches`, no
missing/duplicate/unknown/non-terminal/shard defects.

| Disposition | Count |
|---|---|
| clean | 4 |
| candidate_violation | 3 |
| not_applicable | 0 |
| blocked_by_missing_evidence | 0 |
| excluded_by_contract | 0 |

The 4 `clean` dispositions each carry unit-level evidence (not bulk-defaulted):
`doc.go` (doc matches implementation; forward-looking notes honestly hedged),
`errors.go` (four typed errors are `errors.As`-detectable, join-safe,
self-explaining; the `NotHandOverridableError.Path`-filled-by-resolver division of responsibility is
documented), and the two test files (executable specs, not contract authorities;
test-coverage adequacy is out of scope per SKILL.md "When NOT to use").

---

## 4. Ranked ledger (survivors)

All 3 candidates survived the adversarial pass; Candidate B split into two
distinct findings. **4 verified findings, 0 refuted, 0 indeterminate.** Refuted
candidates would stay in the rigor record only — there were none.

### F1 — rank 1 — C5 — `IsPlatformOverwritable` doc overclaims live wiring; drift hazard *(strongest)*

| field | value |
|---|---|
| affected_units | `99050f8160fbaecb` (ordering.go) |
| contract_claim | The exported predicate `IsPlatformOverwritable` is documented as "the apply-path overwrite gate (Slice 5.1)" and "Slice 5.1 wired this predicate LIVE into the seam apply path (internal/substrate/apply.go): the per-class switch is now gated behind IsPlatformOverwritable" (ordering.go:124-130, 135-137). |
| holds | no |
| adversarial_result | **verified** |
| primary_evidence | `rg "IsPlatformOverwritable\("` over all `*.go` returns exactly ONE match: the function definition at ordering.go:155. There are **zero call sites** in the codebase. The live apply path `planOutcome` (internal/substrate/apply.go:245-294) routes via a hand-rolled `switch cls.Class` over `ClassPlatformManaged` / `ClassOverlayExtension` — it never calls the predicate. |
| counterevidence considered | None restores it: apply.go references the predicate only in a comment (apply.go:238-239); manifest.go:72 likewise only comments. The "wired live" claim is materially false. |
| confidence | high |
| impact | medium-high — a documented safety gate that is not the actual gate. A future maintainer who trusts the doc and changes the predicate (or the switch) will not be alerted that the two have diverged; the authoritative overwrite decision lives in the switch, the documented authority lives in the predicate. |
| likelihood | medium |
| risk_rank | 1 |
| cheapest_recheck | `rg -n "IsPlatformOverwritable\(" --glob "*.go" \| rg -v "//"` → expect only the definition line. |
| recommended_disposition | investigate + document: either call the predicate from `planOutcome` (make the doc true) or rewrite the doc to describe the switch as the authority and drop the "wired LIVE" claim. |

### F2 — rank 2 — C5 — confusable predicate names `IsMutableByPlatform` vs `IsPlatformOverwritable`

| field | value |
|---|---|
| affected_units | `99050f8160fbaecb` (ordering.go) |
| contract_claim | Two exported predicates' names should distinguish their semantics enough for correct caller selection. |
| holds | no |
| adversarial_result | **verified** |
| primary_evidence | `IsMutableByPlatform(c)` (ordering.go:131-133) is true for exactly `{platform_managed}` (generic ungated render); `IsPlatformOverwritable(c)` (ordering.go:155-157) is true for `{platform_managed, overlay_extension}` (seam-apply wholesale). The names — "mutable by platform" vs "overwritable by platform" — imply equivalence; the real distinction (generic ungated render vs seam-apply wholesale, where overlay_extension is overwritten by the OVERLAY SYSTEM) is recoverable only from doc.go. `IsMutableByPlatform` already has a live caller (`internal/hooks/ownership.go:41`); the misselection surface is real. |
| counterevidence considered | doc.go and the predicate doc-comments explain the distinction thoroughly — but the names are the contract surface callers read first, and the package exports both as safety predicates. |
| confidence | high |
| impact | medium — wrong predicate at an overwrite gate ⇒ platform clobbers a protected class, or fails to overwrite an active overlay_extension. |
| likelihood | medium |
| risk_rank | 2 |
| cheapest_recheck | read ordering.go:131-133 and 155-157 side by side; confirm neither name encodes "generic render" vs "seam apply". |
| recommended_disposition | investigate: rename to encode the distinction (e.g. `IsMutableByGenericRender` / `IsOverwritableBySeamApply`). Related C3: the subset invariant `IsMutableByPlatform ⊆ IsPlatformOverwritable` is assumed but unenforced. |

### F3 — rank 3 — C5/C4 — "first invalid default aborts" implies a deterministic order the map-iteration implementation cannot deliver

| field | value |
|---|---|
| affected_units | `9fec91e87a1fbcc9` (resolve.go) |
| contract_claim | The doc states "The first INVALID class literal encountered among the platform defaults aborts immediately with that InvalidClassError" (resolve.go:88-89), implying a deterministic first-found ordering. |
| holds | no |
| adversarial_result | **verified** |
| primary_evidence | The seeding loop iterates a Go map (`for path, rule := range defaults`, resolve.go:103) whose order is nondeterministic. With ≥2 corrupted defaults, the reported `InvalidClassError.Class` (resolve.go:105) varies run-to-run. The abort *decision* is deterministic; the reported offending value is not. The existing test `TestResolve_InvalidDefaultClassAborts` uses a single invalid default and cannot expose this. |
| counterevidence considered | None — the nondeterminism is provable; the only question is severity. |
| confidence | high |
| impact | low — error-reporting path on corrupted upstream input only; no safety bypass (the abort still happens). |
| likelihood | high |
| risk_rank | 3 |
| cheapest_recheck | construct `defaults` with two invalid classes; call `Resolve` repeatedly; observe the reported class alternates. |
| recommended_disposition | investigate: sort the defaults iteration before validation, or reword the doc to drop "first" ("an invalid default aborts"). C4 attaches: deterministic reporting implicitly requires ≤1 invalid default or sorted iteration — an unstated precondition. |

### F4 — rank 4 — C1/C3 — dual declaration of the valid-class vocabulary, asymmetrically enforced

| field | value |
|---|---|
| affected_units | `e429e0ab1f98e5b0` (class.go) |
| contract_claim | There is a single authoritative declaration of the valid-class set (equivalently, `keys(allClasses) == set(AllClasses())`). |
| holds | no (no such contract is stated or enforced) |
| adversarial_result | **verified** |
| primary_evidence | The valid classes are declared twice: the `allClasses` map (class.go:44-51, consulted by `IsValid`/`ParseClass`) and the `AllClasses()` slice literal (class.go:78-87, consumed by error guidance via `validClassList` and by tests). Existing tests verify only `AllClasses() ⊆ allClasses`-keys (`TestClass_IsValid`), not the reverse. A class added to the map but omitted from the slice would pass `IsValid` while being silently absent from `AllClasses()`/error guidance — no test fails. |
| counterevidence considered | `AllClasses()` does not feed any *safety* decision (IsValid/Compare/protectionRank do not use it), so divergence corrupts only error guidance, not the lattice decision. |
| confidence | medium |
| impact | low — error-guidance correctness on a rare future edit; no safety bypass. |
| likelihood | low-medium |
| risk_rank | 4 |
| cheapest_recheck | add a 7th entry to `allClasses` only; `go test ./internal/ownership/` still passes; `AllClasses()` omits it. |
| recommended_disposition | investigate: add a one-line init/test asserting `keys(allClasses) == set(AllClasses())`. |

---

## 5. Distinctness A/B classification (primary metric)

**Recent-commit baseline:** files changed in the last 15 commits
(`57d5488`..`50f2da8`, all 2026-07-28+). No `internal/ownership/*` file appears
in that set (package dormant since 2026-06-27/30).

| Finding | Affected code in recent diff? | Surfaceable by a single diff review? | Classification |
|---|---|---|---|
| F1 | no (ordering.go dormant; apply.go's last touch 2026-07-15 is also pre-window) | no — **cross-file/cross-history**: ordering.go's overclaiming doc diverged from apply.go's parallel switch across two separate edits; neither single-diff review surfaces the divergence | **net-new** |
| F2 | no (ordering.go dormant) | no — dormant code, no current diff touches it | **net-new** |
| F3 | no (resolve.go dormant) | no — dormant code | **net-new** |
| F4 | no (class.go dormant) | no — dormant code | **net-new** |

**Distinctness ratio: 4 net-new / 0 duplicate / 0 indeterminate.**

**Falsifier verdict: did NOT fire.** This is the inverse of the failure mode —
100% of surviving findings live in dormant or cross-history code that per-commit
review cannot see. F1 in particular is the canonical structural blind spot the
capability exists for: a documented safety-gate authority that drifted from the
actual gate across two files and two dates, invisible to any single-diff review.

---

## 6. Rigor-check (independent second opinion)

**Rejected-candidate sample:** the refuted/downgraded bucket is **empty** — all 3
candidates verified, none killed. Rather than fabricate a rejection, I
re-audited one `clean` unit as a "downgraded-suspicion" proxy and probed the
under-sampled class.

| sample_id | candidate / unit | original | second_opinion | basis |
|---|---|---|---|---|
| R1 | errors.go / resolve.go `NotHandOverridableError.Path` mutation (resolve.go:172-176 sets `nhe.Path` on an error returned by Compare) | clean (dismissed as "documented, minor") | **confirm** | `Compare` allocates a fresh `&NotHandOverridableError{…}` per call (ordering.go:80), so the mutation has no aliasing hazard across calls; the path-agnostic→path-filled division of responsibility is documented at the call site and handled in the message. Original disposition holds. |
| R2 (class probe) | whole package — **C2 hidden side effects** (probed as the suspected under-sampled class) | n/a | **no instances found** | Package-level state is write-once-at-init only (`allClasses`, `validClassList`, `protectionRank` — all initialized via `var`/IIFE, never reassigned). No `init()`, no I/O, no goroutines, no receiver mutation. `Resolve`/`decideOverride`/`Compare` are pure. The package is pure-functional; C2 is genuinely absent. Its only "state" is the dual-declaration C1 concern (already captured as F4), not a hidden side effect. |

**Effect on ledger:** none (no restore/downgrade/escalate). The primary pass was
precision-positive (lean candidate set, 100% survivor yield) rather than
over-rejecting; the empty reject bucket is itself a limitation — see §9.

---

## 7. Stop/reshape assessment

None of the eight stop/reshape conditions fired:

| # | Condition | Fired? |
|---|---|---|
| 1 | high candidate volume, low survivor yield | no — 3 candidates → 4 verified (100% yield) |
| 2 | rigor-check repeatedly restores wrongly killed | no — empty reject bucket |
| 3 | most survivors duplicate commit-review | **no — 0 duplicates (inverse; strong positive)** |
| 4 | fallback needs undisclosed semantic assumptions | no — deterministic file fallback, declared-inventory honestly labeled |
| 5 | unit IDs/anchors unstable | no — deterministic hash, anchor matches |
| 6 | exclusions grow without reason | no — zero exclusions |
| 7 | cost dominates evidence | no — 7 units, proportionate |
| 8 | silently treating unknown as clean | no — every unit has explicit unit-level evidence |

**No stop/reshape.** The run produced a strong positive distinctness signal.

---

## 8. S2 evidence classes touched by this run

The brief (§2) defines eight S2 evidence classes. This single run begins to
populate some; it does **not** satisfy all eight and does **not** claim S2
readiness.

| # | Class | This run | Note |
|---|---|---|---|
| 1 | Real repeated trigger | **not established** | one run; no recurrence evidence yet |
| 2 | Manifest reproducibility | **positive** | deterministic, anchor-pinned; self-test proves byte-identical re-runs |
| 3 | Disposition completeness | **positive** | helper enforced 7/7 complete |
| 4 | Semantic precision | **positive** | 4/4 verified via adversarial pass with real counterevidence tracing (zero call sites, provable nondeterminism) |
| 5 | Net-new value | **positive (strongest)** | 100% net-new incl. a cross-history divergence (F1) |
| 6 | Cost evidence | **positive** | 7 units / 4 findings / one focused session — proportionate |
| 7 | Failure-mode evidence | **weak/partial** | dormant slice; no unsupported-stack, budget-exhaustion, or snapshot-drift failure exercised; empty reject bucket ⇒ refutation-rate evidence thin |
| 8 | Authority containment | **positive** | nothing gated; all outputs advisory |

**Net:** this run strengthens the case for classes 2/3/4/5/6/8. It does not
establish recurrence (1) and barely touches failure modes (7). S2 is **not**
earned by this run.

---

## 9. Coverage, cost, and uncertainty declarations

- **Coverage tier:** `declared-inventory` (file fallback). Not adapter-complete.
  A Go declaration/callable adapter might surface additional findings; the file
  manifest cannot. Cross-unit invariants were reached by manual reading, not
  manifest enumeration.
- **Cost:** 7 units manifested, 7 examined, 0 blocked, 3 candidates, 4 verified
  findings, 0 refuted, 0 indeterminate. Proportionate to the evidence.
- **Limitations:**
  - bounded to one package; not a repository-wide claim;
  - the empty reject bucket means the rigor-check could not exercise its
    restore-wrongly-killed function on this run — a precision result with no
    negative-rejection evidence;
  - findings F3/F4 are low-impact (error reporting / future-edit guidance); only
    F1/F2 touch the safety-critical apply-path semantics;
  - F1/F2/F4 rely on tracing call sites outside the bounded slice (apply.go,
    hooks/ownership.go) — legitimate adversarial work, but the *affected unit*
    is inside the slice while the *evidence* spans packages.

## 10. Standing checks (advisory only)

| check_id | purpose | reproduction | expected_signal | authority |
|---|---|---|---|---|
| SC1 | predicate `IsPlatformOverwritable` is actually called somewhere (motivated by F1) | `rg -n "IsPlatformOverwritable\(" --glob "*.go" \| rg -v "//"` | ≥1 non-definition match when the doc claims live wiring | advisory |
| SC2 | valid-class vocabulary declared once (motivated by F4) | `go test ./internal/ownership/` after adding a map/set equality assertion | passes only when map keys == slice set | advisory |

These are reusable observation recipes. `authority: advisory`. They are report/
doctor-WARN-shaped at most and MUST NOT be wired into any gate.

---

## closure-verdict

```
closure-verdict:
  run: contract-invariant-audit bounded pilot (internal/ownership/, anchor 50f2da8)
  manifest: 7 units, declared-inventory, complete (helper-enforced, anchor matches)
  candidates: 3 → findings: 4 verified / 0 refuted / 0 indeterminate
  distinctness_A_B: 4 net-new / 0 duplicate / 0 indeterminate
  falsifier: did NOT fire (inverse signal — 100% net-new in dormant/cross-history code)
  stop_reshape: none of 8 conditions fired
  rigor: empty reject bucket (nothing to restore); C2 class-probe found no instances
  s2_status: NOT claimed; populates classes 2/3/4/5/6/8, not 1, weakly 7
  authority: INFORMS only — nothing gated, nothing committed
```
