# Decision: `checkArmedSchema` all-known-defaults consumption — verdict INTENDED (latent hazard unreachable, guard recorded)

**Date:** 2026-09-29
**Status:** Accepted (study/verdict-only slice; no behavior change). Card
`defer-doctor-armed-schema-defaults` (candidate finding from the 2026-08-15
review: "the armed-schema logic treats 'all defaults known' in a way flagged
as latently wrong"). Region fence honored: the `checkArmedSchema` function
body + its `runDoctor` consumption block — the original `doctor.go:835` line
anchor had drifted to `doctor.go:867-938` (predicted by the card).
**Supersedes:** none.
**See also:** `core_manifest.go:104-116` (the all-known-view retention
rationale this verdict references), `researches/decisions/
2026-07-23-dcp-ownership-layer.md` (CoreOutputs `ae5b30d` background).

## Question

`checkArmedSchema` lints armed files from the ALL-KNOWN ownership walk
(`corpus.CoreOwnershipDefaults()`, `doctor.go:868`) rather than the
selection-aware walk (`CoreOwnershipDefaultsWithExclusion`). Is that a defect
under some profile state?

## Verdict: INTENDED

The all-known consumption for armed-schema linting is the documented design
choice at `core_manifest.go:104-116`: *"armed-schema linting (platform_armed
files are always in the all-known set regardless of capability selection)"*.
The pinned invariant behind that claim holds in current code:

- The core `platform_armed` set is exactly two unconditional core-root files
  (classified in `classifyCorePath`, `core_manifest.go:166-177`):
  `.vh-agent-harness/vh-harness-profile.yml` and
  `.vh-agent-harness/complexity-policy.yml`. Neither is capability-gated.
- The only capabilities declaring `CoreOutputs` (the sole mechanism that puts
  a core-corpus live path into `InactiveLivePaths`) are `core/media-perception`
  (`.opencode/agents/media-perception.md`,
  `.opencode/skills/media-perception/SKILL.md` — `internal/resolver/
  catalog.go:298-301`) and `core/worker-read-only`
  (`.opencode/agents/worker-read-only.md` — `catalog.go:324-326`). No armed
  path appears in any declaration; the current-declaration state is
  machine-pinned by `TestCoreCatalog_MediaPerceptionCoreOutputs`,
  `TestCoreCatalog_WorkerReadOnlyCoreOutputs`, and
  `TestCoreCatalog_OtherCapabilitiesHaveNoCoreOutputs`
  (`internal/resolver/catalog_test.go:104-195`).
- Therefore, for every profile state expressible today (any preset, any
  capability union, any overlay selection), all-known == selection-active
  over the armed class. **No consumer can reach the wrong branch.**

## The latent hazard (conditional, currently unreachable)

IF a future capability ever declared an armed path as a `CoreOutput`, then
under a profile that deselects it: the renderer skips the source
(`ExcludeLivePaths`, `internal/cli/seam.go:820`), `update` never re-seeds it
(excluded from staging), yet `checkArmedSchema` would WARN forever with the
remediation *"missing (will be re-seeded by `vh-agent-harness update`)"*
(`doctor.go:885`) — a permanently false remediation. Note the failure
direction is fail-LOUD operator noise, never fail-silent: all-known is a
superset of active, so no armed file that should be linted escapes linting.

**Guard (the coupling rule this memo exists to carry):** keep
`platform_armed`-classified core paths OUT of every `CoreOutputs` declaration
— armed files are selection-independent by contract. If a capability ever
genuinely needs an armed, capability-gated file, the change must land in the
same slice as either (a) switching `checkArmedSchema` to the selection-aware
map (accepting the skip) or (b) re-wording the missing-branch remediation for
excluded armed paths — plus a tripwire test asserting armed ∩ CoreOutputs =
∅ (today only the concrete declarations are pinned, not the general
invariant).

## Evidence

| Claim | Verifying source | Re-verified |
|-------|------------------|-------------|
| `checkArmedSchema` consumes the all-known walk; missing armed file ⇒ WARN "will be re-seeded" | `internal/cli/doctor.go:867-938` (esp. 868, 875-885) | yes (this slice) |
| `runDoctor` consumption is plain tier application (no hidden armed logic) | `internal/cli/doctor.go:142-146` | yes (this slice) |
| Core armed set = the two profile/complexity files | `core_manifest.go:161-177` (`classifyCorePath`) | yes (this slice) |
| All-known retention for armed linting is documented intent | `core_manifest.go:104-116` | yes (this slice) |
| Only media-perception + worker-read-only declare CoreOutputs; no armed path among them | `internal/resolver/catalog.go:285-328`; `internal/resolver/catalog_test.go:104-195` | yes (this slice) |
| Unselected CoreOutputs are excluded from render/staging (never re-seeded) | `internal/cli/seam.go:820,1024,1040`; `core_manifest.go:96-103` | yes (this slice) |

Study executed under session `wave3-debt-paydown` (wave-3 debt-paydown batch,
2026-09-29); full trail in the card closeout report under
`.local/coordinator/reports/defer-doctor-armed-schema-defaults/`.
