# Decision: Coordination Control-Plane Options (PLACEHOLDER STUB)

**Date:** 2026-04-29 (nominal; this file is a placeholder)
**Status:** Unfilled stub. This path is cited by
`docs/coordination/README.md:14-15` ("For durable option comparison and
background design rationale, see …"), but no real memo was ever written here
(`git log` on this path is empty). This stub preserves the citation rather than
leaving the README reference dangling.
**Supersedes:** none.
**See also:**
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(the partial refill — it fixes the coordinator runtime shape, the owned scope,
and the authority line for the control-plane question this stub was meant to
answer).

## Why this stub exists

`docs/coordination/README.md` references this filename as the home for "durable
option comparison and background design rationale" on the coordination
control plane. De-referencing the README citation is a docs edit that was out
of scope for the slice that surfaced the dangling ref; creating this stub
instead preserves the link target. **This is a placeholder, not a decision.**
A future docs slice should either (a) fill this file with the real
control-plane options comparison, or (b) de-reference the README line and
delete this stub.

## What the partial refill covers

The 2026-07-22 memo (linked above) resolves the portion of the control-plane
question that was actually load-bearing:

- **Runtime shape:** scoped stateful coordinator, read-only and
  non-authoritative, no background runtime (flat-file + lazy-read +
  gate-read-at-release-boundary).
- **Owned scope:** §4.3 defer-liveness, §4.1 invariant/closure kernel, §4.2
  premise-recheck.
- **Authority line:** coordinator state *informs*; safety-layer gates *act*;
  see its transition-authority table.
- **Deferred:** the TencentDB-R4-style async-recovery runtime.

## What this stub does NOT cover

Everything else a full "coordination control-plane options" memo would: a
multi-option comparison (background-runtime vs flat-file vs hybrid),
alternatives rejected, the full state-location matrix, and cross-references to
`docs/coordination/TASK_MODES.md` and `RUNTIME_MODEL.md`. Those remain open
until this stub is filled or de-referenced.
