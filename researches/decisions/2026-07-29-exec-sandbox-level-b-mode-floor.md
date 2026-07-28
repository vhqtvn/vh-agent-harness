# 2026-07-29 — exec-sandbox as the Level-B mechanism + binary-enforced mode-floor

- **Status:** DECIDED (implementation in-flight across Slices 1–4; this memo is
  the FIRST deliverable and lands before/at the same time as the floor + grant).
  All four implementation slices (mode-floor → grant → policy doc → verify) ship
  under this decision.
- **Scope:** the read-only-execution Level-B realization (kernel-contained
  arbitrary read-code) and the safety floor that contains the agent grant.
  Narrow, reversible.
- **Supersedes / extends:** `templates/core/.opencode/docs/agents/read-only-execution-policy.md`
  (the policy doc is updated in Slice 3 to name exec-sandbox as the canonical
  Level-B mechanism, superseding the "pin exact script paths" workaround for the
  general read-code case).

## Context — read-only and no-shell are DIFFERENT concerns

The harness already documents a Read-Only Execution Policy with three levels
(`templates/core/.opencode/docs/agents/read-only-execution-policy.md`):

- **Level A (Observe):** pure recon — deny edits, deny git mutation, allow only
  inspection commands, no project script execution.
- **Level B (Audit runner):** read-only agent that needs command execution —
  deterministic commands via `vh-agent-harness exec ...`, still no edits and no
  git mutation. The doc WARNED against broad interpreters (`python *`, `node *`,
  `bash *`) for read-only agents and told callers to "pin exact script path
  patterns instead."
- **Level C (Builder):** `edit: allow`, `git *: ask`, broader bash — for
  implementation agents only.

The gap: a read-only agent that needs to run ARBITRARY read-code (a DB-cited
data analysis, an AST walk, a perception tool) had no good Level-B mechanism.
Broad interpreter grants (`python *`) are unsafe because an interpreter can do
anything. Pinning exact script paths is brittle and does not generalize. The
"wildcard → ask" flip pushes the decision onto an LLM for every invocation.

## Resolved design decision 1 — exec-sandbox IS the modern Level-B realization

**Decision:** `exec-sandbox` (Landlock + seccomp, host-local) is the correct
modern realization of Level B. It lets a read-only agent run ARBITRARY read-code
(including `python3`, `node`, `bash`) while the KERNEL makes writes-outside-tmp
and network PHYSICALLY impossible.

- **Profile B (the default exec-sandbox profile):** read repo root + system/tool
  paths; write ONLY `./tmp/`; `.git` read-only (Landlock is additive); network
  denied (seccomp blocks socket/connect/bind/listen/accept/sendto/recvfrom).
- **So:** read-only agents that need code get `exec-sandbox` (Level B,
  kernel-contained), NOT a broad interpreter grant and NOT a wildcard→ask flip.

**Rationale (load-bearing principle):** LLM adjudication is ACCEPTED for the
SOFT mutation/edit judgment tier (e.g. the `build` agent's `git *: ask` / `edit:
allow` Level-C surface), but it is NOT trusted to gate ARBITRARY interpreters.
Where structural (kernel) containment applies, it is preferred over an LLM
judgment call. exec-sandbox makes the worst case "the agent wrote to `./tmp/`"
or "the agent tried to write outside tmp and got `EACCES`," never "the agent
ran `git push` or exfiltrated over the network." That is the property that makes
arbitrary read-code safe for a read-only agent.

- `committer` stays **deny-except-gate** (hard invariant, untouched).
- The LLM-auto-approve stays scoped to the Level-C mutation/edit tier (`build`).
- `commit-reviewer` / `ship-review` use the `plan_state` MCP tool (not shell);
  the shell wildcard is not their axis, so they are unchanged.

## Resolved design decision 2 — a binary-enforced MODE-FLOOR is mandatory, not optional

**Dogfood evidence (gathered on this host, kernel 6.17, Landlock active; cited,
not re-derived except to confirm the floored behavior in Slice 4):**

Six probes against a freshly built binary:

| # | Probe | Result |
|---|-------|--------|
| 1 | strict mode reads repo files | PASS |
| 2 | write-outside-tmp → EACCES (file not created) | PASS |
| 3 | write-to-tmp | PASS (allowed) |
| 4 | `--net=deny` blocks sockets (seccomp) | PASS |
| 5 | **`--sandbox=off` FULLY BYPASSES** (wrote outside tmp) | **CRITICAL — naive `exec-sandbox *` allowlist is UNSAFE** |
| 6 | python3 read + tmp-write works | PASS |

**The P5 finding is the load-bearing one.** A naive `vh-agent-harness exec-sandbox *`
permission allowlist is unsafe on its own: the agent just appends
`--sandbox=off` and runs uncontained. Therefore the **mode-floor is mandatory**,
not optional, and it MUST be enforced **BINARY-SIDE**, NOT via permission-string
globs. Flag parsing is too fragile a chokepoint:

- `--sandbox=off` (the obvious bypass);
- duplicate `--sandbox` flags (cobra/pflag last-wins) — `--sandbox=strict --sandbox=off`;
- interspersed `--sandbox` among payload args.

A permission glob cannot see the flag value; it can only allow/deny the verb.
The floor must clamp the FINAL resolved mode AFTER cobra parses, regardless of
how many times `--sandbox` appeared.

### Floor mechanism (implemented in Slice 1)

- **Config authority:** a new top-level `exec_sandbox` block in
  `.vh-agent-harness/run-shape.yml` (the project-owned runtime config), with one
  field `min_mode` (`off | best-effort | strict`). This reuses the existing
  project-owned config authority, ownership class, and seed path (no new file
  type, no new ownership class).
- **Binary-side enforcement:** the exec-sandbox CLI loads the floor best-effort
  (run-shape absent or key absent → no floor) and clamps the caller-supplied
  mode UP to the floor (`strict > best-effort > off`) BEFORE handing the mode to
  `execsandbox.Run`. When floored to strict, any `--sandbox=off|best-effort` is
  upgraded to strict; the caller can NEVER run below the floor.
- **Fail-closed:** when floored to strict and the OS sandbox primitives are
  unavailable, `execsandbox.Run`'s existing strict path refuses (exit non-zero,
  does not run). This is the documented strict contract; the floor inherits it.
- **Tests MUST cover (Slice 1):** the P5 bypass case (`--sandbox=off` writing
  outside tmp) now DENIES/upgrades under a strict floor; duplicate/interspersed
  `--sandbox` flags cannot downgrade below the floor.

### Floor default + ship-to-consumers caveat

- **Binary default floor when the key is absent: `off` (no floor).** This
  preserves current standalone behavior exactly (zero surprise on upgrade; no
  surprise fail-closed on hosts lacking Landlock/seccomp; the documented
  `--sandbox=off` operator escape remains available when no floor is set).
- **The floor is OPT-IN via config.** The dogfood repo's run-shape sets
  `exec_sandbox.min_mode: strict`; the install seed (`defaultRunShapeSeed`) seeds
  `exec_sandbox.min_mode: strict` so FRESH consumer installs get the strict floor.
- **Containment caveat (project_owned seed semantics, stated honestly):** the
  grant (Slice 2) ships force-managed via the Go tables → `opencode.jsonc`; the
  floor ships via the project-owned run-shape seed. On a FRESH install both land
  (grant + strict floor) so the grant is contained. On UPDATE from a pre-floor
  version, the project's existing run-shape is preserved (project_owned), so an
  upgrading consumer gets the grant WITHOUT the floor until they add
  `exec_sandbox.min_mode: strict` to their run-shape. This is the standard
  project_owned seed caveat that applies to every such config in this repo. The
  policy doc (Slice 3) states the containment requirement explicitly: **the
  exec-sandbox grant is contained WHERE `exec_sandbox.min_mode: strict` is in
  effect.**

  This default-off-key / opt-in choice was taken over a binary-default-strict
  floor deliberately: a binary-default-strict floor would surprise-upgrade
  existing operator workflows (refuse `--sandbox=off`, fail-closed on non-Linux)
  on every install, not just where the grant is active. Opt-in via the
  project-owned config keeps the floor where the operator placed it and makes
  the containment boundary explicit and auditable.

## Resolved design decision 3 — researcher is promoted to Level B (REVERSIBLE role decision)

Granting exec-sandbox (floored) to `researcher` **consciously promotes researcher
to Level B.** Rationale: researcher is the agent that performs DB-cited / data
analysis (read a DB copy, classify, compute shares). Today that work is either
blocked or handed off to `build`. With a floored exec-sandbox, researcher can run
`python3`/`node` read-analysis in-lane — reading the input read-only, writing a
neutral dump to `./tmp/` — while the kernel guarantees no write outside tmp and
no network. This is in-charter for a read-only research specialist.

- **This is a REVERSIBLE role decision.** If researcher abuse of the exec-sandbox
  surface ever becomes a concern, remove the one allowlist entry (Slice 2) and
  researcher reverts to Level A. The floor (Slice 1) and the policy doc (Slice 3)
  are independent of this grant.
- researcher's `media-perception` outbound task edge (its one existing delegation)
  is unaffected.

## Resolved design decision 4 — the grant is per-agent, NOT a shared read-only verb

The grant is NOT added to `HarnessReadOnlyCommands` (the SHARED table applied to
EVERY `read_only` agent). Adding it there would grant exec-sandbox to the
pure-deliberation agents (`planner`/`plan`, `debate`×4, `solution-brief`,
`commit-message`, `commit-reviewer`, `ship-review`, the `commit-reviewer-a..d`
cluster leaves) that do NOT need to run code. The grant is a NEW per-agent allow
emitted after the canonical read-only harness verbs (region 4b), scoped to
exactly the three agents grounded by per-agent analysis:

- `media-perception` — perception tooling;
- `repo-explorer` — exploration scripts / AST walks;
- `researcher` — DB-cited / data analysis (Level-B promotion, decision 3).

**Explicitly NOT granted:** `committer` (deny-except-gate invariant); all
pure-deliberation agents (covered by readonly + read-only git/harness verbs);
`build`'s Level-C `Ask` wildcard is NOT touched.

## Payoff — the M1 natural-miss study unblocks in-lane

Once `researcher` has floored exec-sandbox, the M1 natural-miss classification
study unblocks WITHOUT a `python3` wildcard grant and WITHOUT a build handoff:
researcher runs `m1_classify.py` under `vh-agent-harness exec-sandbox --sandbox=strict`
(reads the DB copy mode=ro, writes the neutral dump to `./tmp/`), classifies, and
computes the (e) share. The kernel contains every side effect. **This is the
dogfood proof of the whole change:** a read-only agent runs arbitrary read-code
safely, in-lane, with no LLM gate on the interpreter.

## Slice 0 — investigation report (gate: report before building)

**Question:** does a per-agent or global exec-sandbox MODE-FLOOR already exist?

**Finding: NO.** Inspected:

- `internal/cli/exec_sandbox.go` — `execSandboxMode` is bound directly to the
  `--sandbox` flag (default `best-effort`) and passed straight to
  `parseSandboxMode` → `execsandbox.Run`. There is NO clamp, NO floor, NO config
  read. The resolved mode is whatever the caller's last `--sandbox` flag said.
- `internal/execsandbox/profile.go` — defines `SandboxMode` (`ModeOff` /
  `ModeBestEffort` / `ModeStrict`) and `Profile`. No floor concept.
- `internal/execsandbox/sandbox.go` — `Run` consumes the mode as-is; `ModeStrict`
  + unavailable primitives → fail-closed (the only strict behavior that exists
  today). No floor.
- Config-authority path — `.vh-agent-harness/run-shape.yml` + the seed in
  `internal/cli/seam.go` (`defaultRunShapeSeed`) carry NO `exec_sandbox` block.
  The run-shape schema allowlist (`runShapeAllowedTopLevel`) has no such key.

**Conclusion:** the floor does not exist and must be built. The P5 bypass
(`--sandbox=off` writing outside tmp) is LIVE today. Slice 1 builds the floor;
Slice 2 grants the (now-contained) verb.

## Verification (Slice 4, summarized)

- `go test ./...`, `gofmt -l .`, `go vet ./...` green.
- Embedded corpus refreshed via `make` (satisfies the dev-stale-embed guard).
- The 6 dogfood probes re-run under the floored config: P5 (`--sandbox=off`)
  now DENIES the bypass (upgraded to strict → EACCES outside tmp); the other
  five unchanged (read, tmp-write, net-deny, python3 read+tmp-write).
- `vh-agent-harness update --dry-run` previews the ownership-class seam.
- `doctor` = HEALTHY.
- `behavioral-closure` crux: an agent granted exec-sandbox cannot escape the
  strict floor and can still run read-code + tmp-write.
