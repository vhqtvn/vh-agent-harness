# Sources: DeepSeek Harness (dsh) — Execution-Safety Slice

Durable home: researches/sources/deepseek-harness/execution-safety.md (landed 2026-08-17).

**Date:** 2026-08-17
**Kind:** Source packet. NOT active repo guidance — reference material for
vh-agent-harness maintainers mining harness-design ideas.
**Subject:** `refs/deepseek-harness` — local checkout of
`deepseek-ai/deepseek-harness` @ `47f943859bef60e4160492346772ded9b24f765a`
(2026-08-13), static read-only. "DeepSeek Harness" (dsh): Cordis-based
everything-is-a-plugin agent harness, pnpm monorepo (nested layout:
`packages/<group>/<subpkg>/`).
**Slice:** execution-safety — packages `shell`, `subprocess`, `sandbox`, `fs`,
`storage`, `guard`, `code-runtime`, `e2b`, `terminal`, `workspace`,
`runtime-diagnostics`, plus `native/landlock-run` and docs
(`defensive-patterns.md`, `persistence-catalog.md`, `postmortem/`).
**Transfer target:** vh-agent-harness (Go repo-resident agent harness:
shell-guard permission parser, `exec`/`exec-ro`/`exec-sandbox` verb family,
ownership safety contract, doctor health checks).
**Time-sensitivity:** STABLE for mining purposes — the checkout is pinned to a
commit; all citations resolve against `refs/deepseek-harness` at that pin.
Upstream dsh may have moved on; that does not stale this packet's claims, only
its completeness w.r.t. later upstream work.
**Method:** static reading only (no builds, no test runs, no git mutations on
`refs/`). Anti-guess rule: every claim below traces to a file listed in the
Coverage table as `examined` or `partial`; unread areas are stubbed, not
described.

---

## Research question & scope

- **Question:** How does dsh make agent-driven execution (shell, subprocess,
  file effects, code execution) safe and observable end-to-end, and which of
  its mechanisms transfer to vh-agent-harness?
- **Scope:** the execution-safety slice only (package list above). Not in
  scope: LLM/context/compaction machinery, web GUI, ACP surface, i18n.
- **Recency requirement:** none beyond the pinned commit (stable).
- **Source policy:** primary sources only — the checkout's own source, docs,
  and postmortems. No secondary writing was needed or used.

## The safety/defense story end-to-end

A model-facing tool call in dsh traverses nine safety-relevant stages. Each is
a distinct mechanism; the architecture's signature move is keeping them
**separate and individually observable** rather than fused into one verdict.

1. **Tool surface** (`packages/shell/tool-bash/src/index.ts`). The model-facing
   bash tool advertises escalation fields (`sandbox_permissions` +
   justification) **only when the executor actually confines** — the tool
   schema itself never lies about available safety knobs. It carries verbatim
   teaching text: the retry-once-with-`sandbox_permissions` protocol.
   `resolveWorkdir` uses the canonical session cwd — the same path identity
   the confinement fence uses, so "where the command runs" and "what the
   fence protects" cannot drift apart.

2. **Policy resolution** (`packages/sandbox/sandbox-policy/src/index.ts`,
   `session-mode.ts`). `ctx.sandboxPolicy.resolve()` precedence: explicit
   approved mode > session `sandbox/mode` event fold > deployment default
   (`read-only`, the fail-safe default). **Session-log-as-store:** the
   effective sandbox mode is *folded from SessionEvents* (`sandbox/mode`,
   `approval/decided`, `permission/preset` in the event vocabulary); replay IS
   state; there is no external config store to dual-write and drift. A
   runtime-context prompt section (order 110) makes the current mode
   model-visible without rewriting the system prompt.

3. **Escalation** (`packages/sandbox/sandbox/src/escalation.ts`). A strict
   widening ladder (`WIDER_MODES`): read-only → workspace-write →
   danger-full-access. `validateEscalationArgs` enforces the
   `sandbox_permissions` ⇔ justification pairing before anything else.
   `approveEscalation` runs the strict-widening check **before** prompting the
   human — a non-widening request never produces a prompt. Denials are
   model-visible via `sandboxDenialMarker`
   (`[sandbox: file access denied under <mode> mode]`) and paired with an
   `escalationHintMarker` at the same decision point, nudging a same-turn
   retry with the right shape instead of a blind one.

4. **Confinement execution** (`packages/sandbox/sandbox-local/src/index.ts`,
   `profiles.ts`). Per-platform backend chains (`PLATFORM_CHAINS`): linux
   bwrap → landlock (fallback), darwin seatbelt, win32 windows-acl. Probing
   arbitrates **only when a chain has >1 candidate**. Profiles are explicit
   per backend: bwrap `--ro-bind / / --dev --proc --tmpfs /tmp --bind <ws>`;
   landlock grants `ro:[/]`, `rw:[/dev/null, /tmp, ws]`; Seatbelt SBPL
   `deny file-write*` with subpath allows from shared writableRoots.
   `STATIC_ENFORCEMENT` declares windows-acl *always partial*
   (WRITE_RESTRICTED Everyone requirement; NTFS hard-link aliasing) — the
   system labels partial enforcement instead of pretending full. Config
   validation mutually-requires `runnerCommand` ⇔ `runnerFailureSignatures`
   (you may not configure a custom runner without also teaching dsh how to
   classify its failures).

5. **Native enforcement** (`native/landlock-run/`). A pinned argv grammar
   (`--ro`/`--rw`/`--` separator), documented in `docs/cli-contract.md`; exit
   125 = launcher failure sentinel. `packages/entry/src/main.c` (298 lines,
   read in full): self-defined Landlock UAPI (ABI 1–5 masks) so it does not
   depend on host headers; `prctl(PR_SET_NO_NEW_PRIVS)` **before**
   `landlock_restrict_self`; execvp so the ruleset inherits across execve;
   fail-closed on ENOSYS/EOPNOTSUPP (exit 125, **never run unconfined**); a
   functional `--probe` that builds a maximal ruleset because "`--version`
   checks would miss a kernel that has the syscalls but refuses enforcement";
   non-directory grants keep file-only bits (`--rw /dev/null`); an unopenable
   grant root fails rather than silently narrowing. Distributed as
   musl-static per-platform npm packages with **no install-time build
   fallback, by design**. No environment variables override launcher
   behavior (`packages/entry/src/index.ts`).

6. **Process supervision** (`packages/subprocess/subprocess/src/types.ts`).
   `SubprocessSpawnSpec` has **no defaults**; env is scrubbed, then merged,
   with `undefined` tombstones (explicit removal beats ambient inheritance).
   Bounded `SubprocessCollect` keeps an in-memory TAIL plus a whole-stream
   spill file. `SubprocessOutputReader.readFrom(offset)` is cursor-free.
   `terminate()` is tree-scoped (POSIX process-group signals; Windows
   `taskkill /T`); `waitForExit` waits on the tree, not just the child; a
   full terminal primitive (`SubprocessTerminalHandle`:
   inspectForeground/signalForeground/terminate-quiescence) exists for
   interactive-process control.

7. **Outcome classification**
   (`packages/shell/bash-sandbox/src/helpers.ts`). Two-stage:
   `isRunnerSpawnFailure` requires ENOENT/EACCES **and** `error.path ===
   argv[0]` **and** an exact syscall match **and** an independently verified
   usable workdir — four conjunctive conditions so ordinary command-not-found
   is never misread as "sandbox broken". `classifyRunnerFailure` = nonzero
   exit + optional exit-code gate + exact-line informational exclusion +
   per-line case-insensitive fatal substring. Runner failure **outranks**
   denial in the result taxonomy. This is the machine-encoded form of
   postmortem 0004 (see below).

8. **Result reporting** (`packages/shell/shell/src/types.ts`).
   Request/spec split with an explicit `resolve()`. Env layering
   `ENV_OVERRIDES < caller env < dshEnv` — managed `DSH_*` values cannot be
   displaced. `ShellRunResult` reports **orthogonal cause facts** with
   mutually exclusive `timedOut`/`aborted` and first-cause classification via
   a fused deadline (one timer drives both timeout and abort, so the cause is
   deterministic). `ShellSandboxInfo` carries mode/denied/enforcement/
   runnerFailed alongside, so confinement facts travel with the result.
   `ShellProcess` (background contract): never-rejecting `done`, consuming
   `readOutput`, lossy with spill. bash-local defaults: timeout 120s / cap
   600s / 64KB in-memory / 64MB spill / 3s grace SIGTERM→SIGKILL; the fused
   `using d = deadline(...)` timeout+abort idiom; settings-section live
   reload; `onProcessDone` subclass hook.

9. **Persistence & replay** (`docs/persistence-catalog.md`, 944 lines —
   generated by `scripts/gen-persistence-catalog.ts` and **gated by a
   verify-persistence-catalog check**, so the catalog cannot silently drift
   from the event vocabulary). Complete `SessionEvent` envelope: monotonic
   `seq`; `ignorable?: true` skip-marker with a **fail-closed default** — an
   unknown event type *without* the marker makes the reader refuse
   reconstruction; `SurfaceOp` append/replace; `SESSION_FORMAT_VERSION = 0`.
   Event vocabulary includes `sandbox/mode`, `approval/asked|decided|policy`,
   `permission/preset`, `tool/call|result|code-dispatch*`,
   `hook/invoked|result`, `session/end-seed`.

10. **Loop hygiene & runtime invariants** (`packages/guard/`,
    `packages/runtime-diagnostics/invariants/`). Around the loop:
    `guard/timeout-policy` wraps every `tools/execute` whose tool declares a
    `timeoutMs` and substitutes a structured `TOOL_TIMEOUT` result only when
    its own fused deadline won; `guard/repeat-tool-reminder` counts
    consecutive identical calls on a canonicalized (deep key-sorted) argument
    key — including DENIED calls — and injects gentle-then-detailed
    model-visible reminders without ever vetoing. Under the loop: every
    package publishes a `./invariant` companion into `ctx.invariants`, whose
    runtime checks assert owned event/data relationships (e.g. the
    approval asked/decided audit pairing, sandbox-mode vocabulary) — an
    always-on registry analogous to our doctor checks, with a static gate
    (`verify-package-invariants`) requiring every package to own or explain
    its companion.

Cross-cutting: `docs/defensive-patterns.md` (33 lines, 8 rules) codifies
shipped-bug classes into portable discipline — report orthogonal outcomes
independently; dispose must reach quiescence (kill → await done, close
listeners *before* killing); scrub env of `*KEY*/*SECRET*/*TOKEN*/*PASSWORD*`;
`0700` temp dirs + `'wx'`/`0o600` exclusive opens; unlink link-shaped paths
via `lstatSync` before `unlinkSync` (symlink/junction races). The
`postmortem/` series (0001–0004) closes the loop: incident → root cause →
mechanical guardrail.

## Per-package deep-dives

### `native/landlock-run` — examined (100% of shipped surface)
README, `docs/architecture.md`, `docs/cli-contract.md`,
`packages/entry/src/index.ts` (launcherPath/probe/grantArgs,
`LAUNCHER_FAILURE_EXIT=125`, no env overrides), `packages/entry/src/main.c`
(all 298 lines). Key facts under stage 5 above. Design stance worth quoting:
musl-static per-platform binaries with **no install-time build fallback by
design** — a build-at-install step would reintroduce the toolchain-dependence
landlock-run exists to remove.

### `packages/sandbox/sandbox` (service definition) — examined (~95%)
Read: Service Definition (`SandboxMode` read-only/workspace-write/
danger-full-access — a *file-effects-only* vocabulary, deliberately not a
general "privilege" axis), per-call `SandboxPolicy` ("policy rides the call,
not the provider"), `ConfinedArgv` (argv + enforcement + per-backend
denialSignatures dialect + runnerFailureRules), `SANDBOX_UNAVAILABLE`
fail-closed, `escalation.ts`, `roots.ts`. Residual: `index.ts`/`invariant.ts`
re-verification.
`roots.ts`: `canonicalPath` uses `realpathSync.native` component-by-component
(matches chdir/spawn semantics — a JS impl collapses `..` lexically first on
some platforms); `writableRoots = {workspaceRoot, /tmp, os.tmpdir()}` deduped
canonical — **one shared home** so the in-process fs fence and the Seatbelt
profile cannot drift apart.

### `packages/sandbox/sandbox-local` — examined (~90%)
Read fully: `src/index.ts`, `profiles.ts`. Facts under stages 4–5.
`DENIAL_SIGNATURES` are per-backend (EROFS/EACCES/EPERM variants);
`RUNNER_FAILURE_RULES`: Landlock exit-125-gated with informational exclusion;
windows-acl exit-127-gated. Windows SID/grant lifecycle: per-workspace
standing ACE reuse cache; random per-session temp dir + SID, revoked on
dispose; **fail-closed half-materialized cleanup with AggregateError** (a
failed teardown surfaces every failure, never swallows). Not read: tests.

### `packages/sandbox/sandbox-policy` — examined (100% of `src/`)
`src/index.ts`, `src/session-mode.ts`. Facts under stage 2. The
session-log-as-store pattern is the load-bearing idea: no side-channel
config; mode changes are events; replay reconstructs authority.

### `packages/sandbox/sandbox-windows-acl` — skipped (stub)
11 src files (`ffi.ts`, `acl.ts`, `path-boundary.ts`, `errors.ts`,
`grant.ts`, `workspace-sid.ts`, `win32-abi.ts`, `runner.ts`, `token.ts`,
`spawn.ts`, `invariant.ts`) NOT read. Its externally visible behavior is
captured via sandbox-local usage — treat any windows-acl-specific claim here
as secondhand.

### `packages/shell/shell` — examined (~80%)
Read: `src/types.ts`, `src/index.ts`. Facts under stage 8. Not read:
`render.ts`.

### `packages/shell/bash-local` — examined (100% of `src/`)
`src/index.ts`. ENV_OVERRIDES (`NO_COLOR`, `TERM=dumb`, `PAGER=cat`,
`GIT_PAGER=cat`); defaults 120s/600s/64KB/64MB/3s; fused `deadline()`;
settings-section live reload; `onProcessDone` hook.

### `packages/shell/bash-sandbox` — examined (100% of `src/`)
`src/index.ts`, `src/helpers.ts` — the classifier (stage 7). This pair of
files is the single most transferable classifier design in the slice.

### `packages/shell/tool-bash` — examined (100% of `src/`)
`src/index.ts`, `src/background.ts`, `src/render.ts`. Stage 1 facts;
background path via `ctx.jobs`.

### `packages/shell/shell-env` — examined (100% of `src/`)
`src/index.ts`. `ctx.shellEnv` registry with reserved/declared key ownership
and duplicate detection; `collect()` rebuilds the whole `DSH_*` namespace on
every call after executors discard ambient values — managed env is
**re-derived, never trusted from inheritance**.

### `packages/shell/{pwsh-local, pwsh-sandbox, tool-pwsh, tool-bash-persistent}` — skipped (stubs)
Windows PowerShell family not read. `pwsh-sandbox/src/helpers.ts` presumably
mirrors the bash classifier, but that is an assumption, not a finding.

### `packages/subprocess/subprocess` — partial (~50%)
Read: `src/types.ts` in full (stage 6 facts — the type layer is the
contract). Not read: `src/index.ts` (wiring), `src/invariant.ts`.

### `packages/subprocess/subprocess-local` — partial after top-up (~35%: spawn.ts 100%; process-inspector/terminal/index unread)
Top-up read `src/spawn.ts` (543 lines) in full — the process plumbing. The
module contract is stated up front: "This layer reacts to an abort signal;
callers own deadlines, teardown ladders, and cause classification."
Highlights beyond the types (E11):

- `childEnv()`: explicit caller entries override the **scrubbed parent base**
  (`scrbedParentEnv` — the actual `*KEY*/*SECRET*/*TOKEN*/*PASSWORD*` scrub
  lives in `subprocess/src/index.ts`, still unread); a string restores or
  overrides, an explicit `undefined` tombstone removes; Windows merges with
  case-normalized key semantics.
- Spill discipline matches defensive-patterns exactly: a private 0700
  per-process `mkdtemp` spill dir ("Predictable world-readable paths would
  let other local users read command output or pre-create symlinks"); spill
  files opened `'wx'` 0o600 with random suffixes (defeats symlink planting);
  once the whole-stream total exceeds the spill cap the spill is discarded
  and disabled ("at most maxSpillBytes behind, never an unbounded file");
  `seal()` on a failed close stops advertising the spill path while
  in-memory reads keep working. In-memory tail is byte-exact (trims the head
  of a single over-cap chunk so "a diagnostic tail must hold the LAST
  maxBytes regardless of how the stream was chunked"). Tail-keep rationale
  credited in-source to "pi/OpenCode": errors and final results cluster at
  the end.
- Tree signalling: POSIX `kill(-pid, sig)` on the detached group with
  direct-child fallback; Windows `taskkill /PID <pid> /T /F` with the
  outcome deliberately unchecked ("an already-absent tree (status 128),
  exit races, and a missing taskkill binary … are as tolerable here as
  ESRCH is for a POSIX group signal").
- **Tree liveness ≠ child liveness:** `treeAlive()` probes the group with
  `kill(-pid, 0)`; ESRCH → dead, EPERM → alive (defensive); on Linux only,
  after direct-child settlement, a `/proc`-based refinement
  (`linuxProcessGroupHasLiveMembers`) distinguishes a group of only
  unreaped zombies — it "still answers kill(0), but it can execute no work
  and cannot be signalled into quiescence."
- **PID-reuse defense:** one single-shot whole-tree exit observer; "the
  first confirmed absence is a permanent no-more-signals boundary: it
  cancels a pending escalation before this process-group id can be reused."
- SIGTERM→SIGKILL escalation: the grace timer is NOT cleared at direct-child
  settlement ("a TERM-trapping helper can outlive the settled direct child
  and must stay signalable") and stays ref'd ("the pending SIGKILL is a
  commitment, and a parent exiting before it fires would orphan a trapped
  survivor"). `terminateForHostExit()` is a synchronous final SIGKILL
  "intentionally absent from the public subprocess seam".
- Settlement: `done` resolves on `close`, not `exit`; an inherited-pipe
  survivor "must not hold the outcome open indefinitely" — the same bounded
  grace that governs kills bounds the post-exit drain wait. Only spawn
  failures reject; runtime exits resolve. Failed spawns use pid −1 so
  signalling remains a no-op. Batch stdin writes are best-effort (EPIPE
  swallowed; "outcome rides on exit/output").

Not read: `process-inspector.ts` (374 ln, the `/proc` inspector),
`terminal.ts` (249 ln), `index.ts` (195 ln, incl. the scrub itself).

### `packages/fs` — partial after top-up (fs-sandbox + fs-observation-policy `src/` 100%; `fs/`, `fs-local/`, `tool-fs*` unread)
57 files across 7 subpackages; top-up read the two policy packages in full.

**`fs-sandbox/` (`src/index.ts` 151 ln, `src/containment.ts` 76 ln) — the
in-process fence.** `SandboxedFileSystem extends LocalFileSystem`; ALL local
text-storage mechanics (resolve, stat, read/stream, list, atomic write,
read-match-write edit critical section) are inherited verbatim — the package
adds **only a per-call policy fence on the two mutations**. Reads pass
through untouched: every mode permits reading. The module doc states the
threat model exactly: "The fence is a policy check in TRUSTED code over a
MODEL-CONTROLLED path, NOT a kernel boundary — the operations are the seam's
own (open, rename), and only the target path is untrusted, so
canonicalize-then-contain is the complete answer to this surface. Kernel-grade
isolation of untrusted CODE stays `ctx.shell`'s job. This mirrors the
`code-runtime` stance: containment, not a security boundary." The residual
TOCTOU (ancestor symlink swapped between re-check and syscall) is narrowed by
re-canonicalizing immediately before delegating and is **accepted** for this
threat model. Mechanism highlights:

- `checkedTarget()` eliminates check-here-write-there TOCTOU by returning the
  EXACT target the mutation must use: `workspace-write` re-canonicalizes NOW
  (`resolve` realpaths the deepest existing ancestor, reflecting a
  concurrently swapped symlink), requires containment under
  `writableRoots(policy)` — the SAME writable-root set Seatbelt grants,
  "derived from the one `writableRoots` function so bash and fs cannot drift"
  — and the mutation delegates with THAT fresh target, never the stale one.
  `read-only` throws; `danger-full-access` returns the caller's target
  unfenced.
- Denials throw the structured `FS_SANDBOX_DENIED` — "no text inference is
  needed (unlike bash's kernel stderr), because an in-process fence knows
  exactly what it refused." Two denial-evidence regimes by fence type:
  in-process fence → structured error codes; kernel fence → stderr-signature
  classification (stage 7). The escalation retry lives in the tool layer
  (`dsh-tool-fs`), exactly as bash's does — shared UX across fence types.
- The sandbox default mode is NOT plugin config: `ctx.sandboxPolicy` resolves
  each calling session per call.
- `containment.ts` `isPathUnder()`: lexical fast path (case-insensitive on
  win32), then a **filesystem-identity fallback** — walk the target's
  existing ancestors comparing `dev`+`ino` (BigIntStats) with the root;
  recognizes Windows long-name/8.3 aliases and casing "without weakening
  containment to a textual approximation". Identity-based, not string-based.

**`fs-observation-policy/` (`src/index.ts` 130 ln, `src/types.ts`) —
read-before-write CAS discipline for model edits.** Event-only plugin (no
service): three `fs/*` listeners over a
`WeakMap<owner=agent.session, Map<targetKey, FsObservation>>`. Policy:
**edit requires having read the file first** — unseen target rejects
`FS_NOT_OBSERVED` ("edit requires reading X first"); confirmed absence
rejects `FS_NOT_FOUND`; presence supplies the observed version as the CAS
basis (`replaceIfVersion`). Writes: unseen/absent → `createIfAbsent`;
present → `replaceIfVersion`. A direct tool call with no agent session reads
freely but cannot satisfy the write/edit prior-observation policy (fail
closed for edits). State is weakly held (a collected session frees its
state), cleared on disposal (HMR safety); the `{kind:'plugin'}` message
source label is load-bearing — an unlabeled context would render as a user
prompt in derived history (attribution integrity).

### `packages/storage` — partial after top-up (family README; subpackage src unread)
Top-up read the family README: storage is the **non-session** storage family
— "persists application data other than session event logs through named
backends and typed data forms" (`storage/` connects backends with forms;
`storage-json` registers backend `json`; `storage-sqlite` backend `sqlite`
with monotonic `SCHEMA_VERSION` per root AGENTS.md; `storage-domain`
provides validated domain-record storage `ctx.storageDomain` with
`DomainSpec`/`Domain`/`domain/changed`). Consequence for the
session-log-as-store story: the SessionEvent sink is NOT this family —
durable session data (persistence, projection) lives in `packages/session`,
and per the invariants README, Session itself "owns immutable, surface-valid
log storage in every composition: it takes one lossless JSON snapshot of
each candidate, validates complete cited source-event coverage and
positional replacement, restricts `tool/result` replacement to one current
result's `content`, deep-freezes the accepted record, and exposes the log
through immutable array snapshots." Subpackage internals (`backend.ts`,
`registry.ts`, `domain.ts`, `schema.ts`, …) unread.

### `packages/guard` — examined after top-up (both subpackage `src/` 100%)
**`timeout-policy/` (81 ln)** — cooperative tool-call timeout enforcer. A
tool declares `timeoutMs` and promises to honor `exec.signal`; this
`tools/execute` wrapper arms a deadline (the SAME fused `deadline()` from
`@deepseek-ai/dsh-timeout` that bash-local uses) and maps its own expiry to
a structured `TOOL_TIMEOUT` result "without racing or abandoning the tool
promise". Three discipline details: (a) the deadline is scoped by its own
classification code so a nested OUTER deadline firing first "reads as an
ordinary upstream cancel" — first-cause attribution by code scoping; (b) it
swaps `exec.signal` for dispatch and restores the caller's signal in
`finally` so post-execute listeners never see the wrapper's aborted signal;
(c) it replaces the result only when ITS OWN timer fired AND the tool
reached quiescence after seeing the abort. Layering answer: guard owns the
generic tool-call budget; bash-local owns shell-process timeouts; both share
one deadline primitive (`dsh-timeout`).

**`repeat-tool-reminder/` (233 ln)** — advisory per-agent repeat-call
detector; "enriches post-execute decisions with logged model context without
vetoing or rewriting calls" (observe-and-enrich, never veto). Canonical
chain key = deep key-sort of the parsed JSON args, then stringify — arg
objects differing only in property order canonicalize identically; the FULL
canonical string is the key while the model-visible preview is capped
(default 500 chars) "precisely in a loop scenario". Gentle-then-detailed
escalation at thresholds `[3,5,8]` (fail-loud validation: empty list,
non-integer, <2, or duplicate throws at plugin load). Counting happens in
post-execute **because denied calls also flow through the waterfall** — "a
model hammering a denied call is exactly the loop worth breaking" — and the
reminder rides `additionalContexts` on BOTH decision variants, so a blocked
call still gets the nudge. A user interjection resets the chain ("repetition
across it is not a loop"). State is a `WeakMap<Agent, Chain>`; reminders are
stamped `{kind:'plugin'}` so they never render as user prompts in derived
history.

### `packages/{e2b, code-runtime, terminal, workspace, runtime-diagnostics}` — partial after top-up (family READMEs; src unread)
Top-up read each family README (runtime-diagnostics' lives at
`invariants/README.md` — the group is a single package):

- **`e2b/` — remote execution-world POC.** E2B supplies only sandbox
  lifecycle plus TWO fundamental OS adapters: `fs-e2b` implements `ctx.fs`
  over E2B Filesystem APIs, `subprocess-e2b` implements `ctx.subprocess`
  (executable lookup, managed process groups and stdio, remote spill files,
  terminal sessions over E2B Commands/PTY APIs). Crucially, "bash-local,
  terminal-bash, and lsp-stdio need no E2B-specific forks. They delegate
  every execution-world operation to `ctx.fs` and `ctx.subprocess`, so
  mounting the two E2B adapters places their mutable work in the same
  sandbox." Sandbox deleted on timeout or disposal; the harness process,
  agent/session state, and session persistence do NOT move. The execution
  world is fully abstracted behind the fs+subprocess seams — local→remote is
  adapter mounting, not a parallel stack.
- **`code-runtime/` — model-written program execution seam.** Service
  Definition `ctx.codeRuntime` "for executing one model-written program
  against host-provided async bindings, capturing what it printed and
  returned"; worker-thread backend (`code-runtime-worker-thread`); the
  Consumer is Code Mode (`tools: { mode: code }` — the `run_code` tool plus
  an SDK generated in the loaded runtime's language). Isolation and
  execution-budget details live in child READMEs (unread). Per fs-sandbox's
  doc: the code-runtime stance is "containment, not a security boundary".
- **`terminal/` — persistent PTY capability.** `ctx.terminals` backend
  registry with branded ids, exact-Agent ownership, and awaited cleanup;
  `terminal-bash` builds on **`ctx.subprocess.spawnTerminal`** (the terminal
  primitive from E11 — layering resolved: subprocess owns the primitive,
  this family owns sessions/readiness/bounded reads/sandbox policy);
  `tool-terminal` exposes six model-facing tools. "PTY complements the
  one-shot bash and filesystem tools; it does not replace their stronger
  per-operation contracts."
- **`workspace/` — workspace entity.** `ctx.workspaceRegistry`: persistent
  workspaces as "user directories with titles and ordered session
  membership"; realpath canon, registration/resolution semantics (subsystem
  doc unread).
- **`runtime-diagnostics/invariants/` — dsh's doctor analogue (high-value).**
  `ctx.invariants`: a configurable registry service; **every workspace
  package publishes a `./invariant` companion** registering its exact npm
  name — this is what every `invariant.ts` seen across the slice is. Rules:
  runtime invariants assert **owned relationships** (authoritative event
  streams or mutable data), NOT service/method presence, plugin metadata, or
  fixed pure examples — those are "a type, load, or unit-test concern rather
  than a runtime invariant". When no plausible relationship exists the
  companion ships an EMPTY installer with a package-specific
  `No runtime invariant:` reason comment; "generated companions, unexplained
  empties, and ignored reporters fail `verify-package-invariants`" (the
  static source gate over all workspace packages). Each enabled check runs
  in a dedicated child Cordis fiber; session-backed companions "rebuild
  their baseline from durable events" (session-log-as-store again).
  Executable companions include: `dsh-sandbox-policy` (sandbox-mode
  vocabulary), `dsh-permission-presets` + `dsh-user-approval` ("Active-preset
  references and approval asked/decided audit pairing" — the approval audit
  is runtime-checked), `dsh-session` (session enclosure and call/result
  trace), `dsh-time-context` (durable clock readings agree with the open
  turn — "rendered time parses and does not postdate its event").

## Postmortem lessons (each cited)

- **0001 — `export default apply` dropped the plugin namespace**
  (`docs/postmortem/0001-acp-default-export-drops-inject.md`). Cordis
  `unwrapExports` discarded the namespace; inject was lost. 178 green tests +
  100% coverage missed it. Guardrail: a real-Loader keyless e2e. Lesson
  quote: "coverage proves lines ran, not that the feature works the way it
  ships." (Direct resonance with vh-agent-harness's B1/A verification-claim
  separation — coverage is structural, not behavioral.)
- **0002 — `!!js` expression in entry metadata, not plugin config**
  (`docs/postmortem/0002-js-expression-disabled-filesystem-tools.md`).
  `disabled: !!js ...` evaluated only under plugin config; in entry metadata
  it was always-truthy → fs tools permanently disabled. Snapshots had
  **accepted UNKNOWN_TOOL as valid** — the test suite codified the broken
  behavior. Guardrails: `verify-cordis-config` rejects expression nodes in
  entry metadata + snapshot suite rejects UNKNOWN_TOOL.
- **0003 — web agent validated the wrong server**
  (`docs/postmortem/0003-web-agent-gui-feedback-loop.md`). The agent
  validated a *replacement* server, not the hosting GUI; HTTP 200 ≠ app
  readiness (`window.__DSH_BOOT__`). Guardrail: model-visible
  `$DSH_WEB_URL`/`$DSH_WEB_MODE` so the agent can find the real surface.
- **0004 — Landlock "partial enforcement" notice misclassified child failures
  (highest value)**
  (`docs/postmortem/0004-landlock-partial-notice-misclassified-child-failures.md`).
  Any nonzero child exit — including ripgrep's exit-1 *no-match* — was
  misclassified as SANDBOX_UNAVAILABLE. Root cause: "a bag of substrings"
  type could not express exit-gating + exact informational exclusions. Fix:
  `RunnerFailureRule {allowedExitCodes, fatalSignatures, informationalLines}`.
  Lesson quote: **"a shared prefix is not a protocol."** This is the origin
  of stage 7's classifier and of `RUNNER_FAILURE_RULES` everywhere.

## Findings

- **(E1)** Sandbox policy rides the tool call, not the provider; resolution
  precedence is approved-mode > session-fold > read-only default:
  source=`packages/sandbox/sandbox/src/index.ts` (Service Definition) +
  `packages/sandbox/sandbox-policy/src/index.ts`, confidence=high, type=fact.
- **(E2)** Session sandbox mode is folded from SessionEvents; replay IS
  state; no external config store: source=`sandbox-policy/src/session-mode.ts`
  + `docs/persistence-catalog.md`, confidence=high, type=fact.
- **(E3)** Escalation is a strict-widening ladder; widening is checked BEFORE
  any human prompt; non-widening requests never prompt; escalation args
  require justification pairing: source=`packages/sandbox/sandbox/src/escalation.ts`,
  confidence=high, type=fact.
- **(E4)** Denials are model-visible markers paired with a same-turn
  escalation hint at the decision point: source=`escalation.ts`
  (`sandboxDenialMarker`, `escalationHintMarker`) + `tool-bash` teaching text,
  confidence=high, type=fact.
- **(E5)** Linux confinement chains bwrap → landlock with probe arbitration
  only when the chain has >1 candidate: source=`sandbox-local/src/index.ts`,
  confidence=high, type=fact.
- **(E6)** windows-acl is *declared permanently partial* (WRITE_RESTRICTED
  Everyone requirement, NTFS hard-link aliasing) rather than treated as full
  enforcement: source=`sandbox-local/src/index.ts` STATIC_ENFORCEMENT,
  confidence=high, type=fact.
- **(E7)** Landlock launcher: pinned argv grammar, exit-125 sentinel,
  NO_NEW_PRIVS-before-restrict ordering, fail-closed on ENOSYS, functional
  --probe with maximal ruleset, file-only bits for non-directory grants,
  unopenable grant root fails, musl-static with no build fallback:
  source=`native/landlock-run/docs/cli-contract.md` +
  `packages/entry/src/main.c` (all 298 lines), confidence=high, type=fact.
- **(E8)** Runner-failure classification is a typed protocol
  (`RunnerFailureRule`: allowedExitCodes + fatalSignatures +
  informationalLines), spawn-failure requires four conjunctive conditions,
  runner failure outranks denial: source=`bash-sandbox/src/helpers.ts` +
  `sandbox-local` RUNNER_FAILURE_RULES + postmortem 0004, confidence=high,
  type=fact.
- **(E9)** Shell results report orthogonal cause facts with mutually
  exclusive timedOut/aborted resolved by a single fused deadline:
  source=`packages/shell/shell/src/types.ts` + `bash-local/src/index.ts`,
  confidence=high, type=fact.
- **(E10)** Env is layered ENV_OVERRIDES < caller < dshEnv; managed DSH_*
  namespace is rebuilt per collect() after executors discard ambient values:
  source=`shell/src/types.ts` + `shell-env/src/index.ts`, confidence=high,
  type=fact.
- **(E11)** Subprocess spec has no defaults; env scrubbed then merged with
  undefined tombstones; output bounded (TAIL + spill); terminate/waitForExit
  are tree-scoped; terminal primitive exists for foreground control:
  source=`packages/subprocess/subprocess/src/types.ts`, confidence=high,
  type=fact. (Type-layer only; implementation unread at packet time.)
- **(E12)** SessionEvent envelope is monotonic-seq'd, version-macro'd
  (SESSION_FORMAT_VERSION=0), and fail-closed against unknown event types
  unless explicitly marked `ignorable: true`:
  source=`docs/persistence-catalog.md`, confidence=high, type=fact.
- **(E13)** The persistence catalog is generated and gated by a verifier
  (gen-persistence-catalog.ts + verify-persistence-catalog) so doc and
  vocabulary cannot drift: source=`docs/persistence-catalog.md` header,
  confidence=high, type=fact.
- **(E14)** Canonical paths use realpathSync.native component-by-component;
  writableRoots are one shared canonical set feeding both the fs fence and
  the Seatbelt profile: source=`packages/sandbox/sandbox/src/roots.ts`,
  confidence=high, type=fact.
- **(E15)** Postmortem discipline is numbered, public, guardrail-per-lesson,
  and quotes its own lessons ("coverage proves lines ran...", "a shared
  prefix is not a protocol"): source=`docs/postmortem/0001–0004`,
  confidence=high, type=fact.
- **(E16)** Defensive-patterns doc codifies 8 shipped-bug classes as portable
  rules (orthogonal outcomes, quiescent dispose, env scrubbing, 0700+'wx'
  temp discipline, lstat-before-unlink for link-shaped paths):
  source=`docs/defensive-patterns.md`, confidence=high, type=fact.
- **(E17)** bwrap isolates /tmp (fresh `--tmpfs /tmp`) while landlock grants
  rw on the REAL /tmp — two backends in the same linux chain give materially
  different /tmp semantics (private vs host-shared):
  source=`sandbox-local/src/profiles.ts` (both profile definitions),
  confidence=medium, type=inference (the divergence is read off the two
  profile shapes; dsh's own rationale for accepting it is not stated in the
  read files).
- **(E18)** Windows ACL teardown is fail-closed with AggregateError
  surfacing every half-materialized-cleanup failure: source=`sandbox-local`
  (SID/grant lifecycle), confidence=high, type=fact.
- **(E19)** Tool schema advertises escalation fields only when the executor
  confines, and teaches the retry protocol in verbatim model-facing text:
  source=`tool-bash/src/index.ts`, confidence=high, type=fact.
- **(E20)** Custom runner configuration mutually-requires failure signatures
  (runnerCommand ⇔ runnerFailureSignatures): source=`sandbox-local` config
  validation, confidence=high, type=fact.
- **(E21)** The fs fence is explicitly a policy check in trusted code over a
  model-controlled path — NOT a kernel boundary; kernel-grade isolation of
  untrusted code stays the shell sandbox's job; the residual TOCTOU is
  narrowed and accepted for that threat model:
  source=`packages/fs/fs-sandbox/src/index.ts` module doc, confidence=high,
  type=fact.
- **(E22)** The fs fence denies with structured `FS_SANDBOX_DENIED` — "no
  text inference is needed (unlike bash's kernel stderr), because an
  in-process fence knows exactly what it refused"; the tool layer maps it to
  the same `[sandbox: …]` marker + escalation hint as bash:
  source=`fs-sandbox/src/index.ts`, confidence=high, type=fact.
- **(E23)** fs containment is identity-based where it matters: lexical fast
  path, then dev+ino ancestor-walk fallback for alias-equivalent roots
  (Windows 8.3/casing) — "without weakening containment to a textual
  approximation": source=`fs-sandbox/src/containment.ts`, confidence=high,
  type=fact.
- **(E24)** The fs fence enforces check-here-write-there elimination: the
  containment check returns the EXACT fresh-canonical target the mutation
  must use, never the caller's stale one:
  source=`fs-sandbox/src/index.ts` `checkedTarget`, confidence=high,
  type=fact.
- **(E25)** Model edits are CAS-guarded by prior observation: edit rejects
  `FS_NOT_OBSERVED` unless the session previously read the file; presence
  supplies the observed version as the replace-if-version basis:
  source=`packages/fs/fs-observation-policy/src/index.ts`, confidence=high,
  type=fact.
- **(E26)** The e2b POC moves the entire execution world behind the two
  fs+subprocess seams — bash/terminal/LSP consumers need no E2B forks;
  mounting two adapters relocates all their mutable work into the remote
  sandbox: source=`packages/e2b/README.md`, confidence=high, type=fact.
- **(E27)** timeout-policy scopes its fused deadline by classification code
  so a nested outer deadline firing first reads as an upstream cancel, swaps
  and restores `exec.signal` around dispatch, and replaces the result only
  when its own timer fired and the tool reached quiescence:
  source=`packages/guard/timeout-policy/src/index.ts`, confidence=high,
  type=fact.
- **(E28)** repeat-tool-reminder is advisory-only loop hygiene: canonical
  deep-key-sorted argument identity, thresholds [3,5,8], counts denied calls
  too ("a model hammering a denied call is exactly the loop worth
  breaking"), reminders ride both block and pass decisions, user
  interjection resets the chain: source=`packages/guard/repeat-tool-reminder/src/index.ts`,
  confidence=high, type=fact.
- **(E29)** Every workspace package publishes a `./invariant` companion into
  the `ctx.invariants` registry; invariants assert owned event/data
  relationships (never method presence or metadata); unexplained empty
  companions fail the static `verify-package-invariants` gate:
  source=`packages/runtime-diagnostics/invariants/README.md`, confidence=high,
  type=fact.
- **(E30)** subprocess-local defends against PID reuse (single-shot
  whole-tree exit observer = "permanent no-more-signals boundary"),
  distinguishes zombie-only groups via `/proc` on Linux, keeps the SIGKILL
  escalation timer alive across direct-child settlement, and bounds
  inherited-pipe drain with the same grace: source=`packages/subprocess/subprocess-local/src/spawn.ts`,
  confidence=high, type=fact.
- **(E31)** Spill files are planted-symlink-safe by construction: 0700
  per-process mkdtemp dir + `'wx'`/0o600 opens + random suffixes; spill is
  discarded and disabled once the whole-stream cap is exceeded:
  source=`spawn.ts` OutputCollector + privateSpillDir, confidence=high,
  type=fact.

## Transferable ideas → vh-agent-harness

Ordered by estimated value-to-effort for our Go harness. These are ideas for
a future brief, NOT adopted policy.

1. **Runner-failure classification protocol** (E8, E15/0004). "A shared
   prefix is not a protocol": our `exec-sandbox` trampoline output parsing,
   and any doctor check that classifies child-tool output, should use a typed
   rule set (allowed exit codes + exact informational-line exclusions +
   case-sensitive fatal signatures) instead of substring matching. Postmortem
   0004 is the cautionary tale; `bash-sandbox/src/helpers.ts` is the
   reference implementation; our `exec-ro` classifier
   (`internal/execro/classifier.go`) is the obvious first consumer.
2. **Guard/escalation ladder** (E3, E4, E19). Pair every machine denial with
   a same-turn, model-visible recovery hint; validate request shape
   (permission ⇔ justification pairing) BEFORE spending a human prompt;
   never prompt for non-widening requests. Maps onto shell-guard's ask flow
   and the exec verb family's permission ladder.
3. **Landlock launcher contract** (E7). The whole `native/landlock-run`
   design — pinned argv grammar, single sentinel exit code, functional probe
   with maximal ruleset, fail-closed on ENOSYS, static binaries, no build
   fallback — is the Linux-native counterpart to the Codex bwrap study
   (`researches/sources/2026-07-07-codex-bubblewrap-sandbox-study.md`) and
   directly informs our briefed Go-native Landlock+seccomp trampoline.
4. **Session-log-as-store** (E2, E12). Effective permissions/mode folded
   from an append-only event log; no side-channel config to drift. Relevant
   to our plan-state/session-memory design and any future permission-ladder
   persistence; the `ignorable?: true` fail-closed unknown-event default
   (E12) is the forward-compatibility mechanism our checkpoint/session
   readers lack.
5. **Env scrubbing/layering** (E10, E16). Scrub
   `*KEY*/*SECRET*/*TOKEN*/*PASSWORD*` before merge; explicit-tombstone
   semantics; re-derive managed namespaces per call rather than trusting
   inheritance. Extends our shell-guard credential rule (currently: no
   secrets inline in commands) to the execution env itself.
6. **Orthogonal outcome reporting** (E9, E16). Our exec-family result
   surfaces should carry independently-reported cause facts (timedOut /
   signal / exitCode / runnerFailed / enforcement-level) rather than one
   fused verdict — the same shape as our B1 verification-claim separation,
   applied to child processes.
7. **Quiescent dispose + fail-closed teardown** (E16, E18). Kill → await
   done; close listeners before killing; AggregateError-style surfaced
   partial-cleanup failures. Relevant to bgshell-job lifecycle and any
   future sandbox resource teardown in Go.
8. **Generated-and-verified catalogs** (E13). Our `persistence-catalog.md`
   equivalent would be a generated map of session/checkpoint/task-card
   schemas gated by a doctor check — kills a whole class of stale-doc rot.
9. **Postmortem discipline** (E15). Numbered postmortems with one mechanical
   guardrail each, written for plugin authors. Our docs/checkpoints capture
   state; dsh's format captures *class-level* lessons with regression-proof
   guardrails. Cheap to adopt wholesale.
10. **Shared canonical writableRoots** (E14). One deduped, canonicalized
    root set feeding every fence implementation (fs fence + seatbelt + tool
    workdir resolution) so they cannot drift. Applies to exec-sandbox path
    handling and repo-relative path resolution (AGENTS.md command-hygiene
    rule 7's enforcement-side cousin).
11. **Invariant companion registry** (E29). Every workspace package
    publishes a `./invariant` companion into `ctx.invariants`; runtime
    checks assert owned event/data relationships, and unexplained empty
    companions fail the static `verify-package-invariants` gate. This is
    dsh's doctor analogue — an always-on runtime check registry plus a
    static ownership gate over every package. Maps onto our doctor health
    checks and any future per-package invariant contracts.
12. **CAS observation policy** (E25). Model edits are compare-and-swap
    guarded by prior observation: edit rejects `FS_NOT_OBSERVED` unless the
    session previously read the file, confirmed absence rejects
    `FS_NOT_FOUND`, and the observed version supplies the
    `replaceIfVersion` basis. Maps onto our edit-fence/session-memory
    surfaces — "read before write, edit against what you read" as a
    mechanical per-session policy rather than a prompt rule.
13. **Execution-world seam abstraction** (E26). The e2b POC shows `ctx.fs` +
    `ctx.subprocess` fully abstracting the execution world: bash, terminal,
    and LSP consumers need no E2B forks — mounting two adapters relocates
    all their mutable work into the remote sandbox. Maps onto our exec-verb
    family and exec-sandbox backend design: keep the execution world behind
    two seams so local→remote (or local→Landlock) is adapter mounting, not
    a parallel stack.

## Contradictions

- **(C1) /tmp semantics diverge across the linux chain** — bwrap profile
  mounts a fresh `--tmpfs /tmp` (isolated) while the landlock fallback grants
  rw on the host's real `/tmp` (shared). Same logical mode
  (`workspace-write`), observably different filesystem semantics depending on
  which backend won probe arbitration. dsh accepts this silently in the read
  files; no rationale found. (See E17.)
- **(C2) Fail-closed SANDBOX_UNAVAILABLE vs permanently-partial windows-acl**
  — the service demands enforcement and fails closed when a runner is
  unavailable, yet one backend is *documented as never fully enforcing*
  (E6). Not a bug: dsh resolves the tension by labeling enforcement quality
  (`enforcement: partial` in ShellSandboxInfo) instead of refusing the
  backend. Flagged because a "fail-closed" pitch that quietly ships a
  partial backend needs the same explicit labeling we would owe exec-sandbox
  users on Windows.
- **(C3) Snapshot tests codified broken behavior** (postmortem 0002) — the
  guardrail suite itself had accepted UNKNOWN_TOOL as valid, i.e. the
  verification layer was confirming the defect. Resolved in-repo; retained
  here as a contradiction class to watch for in our own snapshot/doctor
  checks.
- **(C4) CONFIRMED real asymmetry: `/dev/null`** — `/dev/null` is a Landlock
  rw grant (native/landlock-run's non-directory file-only `--rw` grants plus
  the landlock profile in sandbox-local) but is absent from the fs fence's
  `writableRoots` (`roots.ts`: {workspaceRoot, /tmp, os.tmpdir()}). The
  fs-sandbox top-up read resolves the pending check: the in-process fence
  would deny a write to `/dev/null` under `workspace-write`
  (`FS_SANDBOX_DENIED`) where the landlock runner grants rw — the fs fence
  denies writes landlock would allow. No rationale for the asymmetry is
  stated in the read files; recorded as a confirmed divergence between the
  two fence implementations.

## Coverage

Post-top-up statuses, consolidated by the promoting session from the author's
pending-edit list (pre-top-up table preserved in the tmp-stage original).

| Path (under `refs/deepseek-harness/`) | Status | Notes |
|---|---|---|
| docs (`defensive-patterns.md`, `persistence-catalog.md`, `postmortem/0001–0004`) | examined | 100% |
| `native/landlock-run/` | examined | 100% of shipped surface (README, architecture, cli-contract, entry/src full) |
| `packages/sandbox/sandbox-policy/` | examined | 100% of `src/` |
| `packages/shell/` bash family (`bash-local`, `bash-sandbox`, `tool-bash`, `shell-env`) | examined | ~100% (each `src/` read in full) |
| `packages/guard/` (`timeout-policy`, `repeat-tool-reminder`) | examined | 100% (both subpackage `src/` read in full, top-up) |
| `packages/sandbox/` (`sandbox`, `sandbox-local`) | examined | ~90% (residual: `sandbox/index.ts` + `invariant.ts` re-verification; sandbox-local tests unread) |
| `packages/sandbox/sandbox-windows-acl/` | skipped | internals unread (11 files); behavior secondhand via sandbox-local |
| `packages/shell/` (`shell`, pwsh family, `tool-bash-persistent`) | partial | ~70% (`shell` types+index read, `render.ts` unread; pwsh family + `tool-bash-persistent` skipped) |
| `packages/subprocess/` (`subprocess`, `subprocess-local`) | partial | ~50% (types.ts + spawn.ts read in full; process-inspector/terminal/index unread) |
| `packages/fs/` (7 subpkgs) | partial | ~30% overall BUT `fs-sandbox` + `fs-observation-policy` 100% (top-up: both `src/` read in full) |
| `packages/{storage,e2b,code-runtime,terminal,workspace,runtime-diagnostics}` | README-level | family READMEs read (top-up); subpackage src unread |

## Open questions

1. ~~**fs-sandbox fence semantics**~~ — **answered by the fs-sandbox top-up
   read**: the fence is a policy check in trusted code over a
   model-controlled path — `SandboxedFileSystem extends LocalFileSystem`
   adds a per-call policy fence on the two mutations (reads always pass);
   kernel-grade isolation of untrusted code stays `ctx.shell`'s job (E21).
   Its writable-root set does NOT include `/dev/null` — C4 confirmed.
2. ~~**fs-observation-policy**~~ — **answered by the top-up read**: it is
   the read-before-write CAS discipline for model edits — edit rejects
   `FS_NOT_OBSERVED` unless the session previously read the target,
   confirmed absence rejects `FS_NOT_FOUND`, and the observed version is
   the `replaceIfVersion` basis (E25). It does not share `writableRoots`:
   observation state is keyed per agent session, not by path containment.
3. **e2b** — does the per-call SandboxPolicy ride to the remote backend, and
   how are remote runner failures classified (no local argv to gate on)?
4. **code-runtime** — what execution model (container/VM/plugin host), and
   does it reuse the subprocess/sandbox vocabulary or invent its own?
5. ~~**terminal package vs SubprocessTerminalHandle**~~ — **answered by the
   terminal/e2b README top-up**: subprocess owns the terminal primitive
   (`ctx.subprocess.spawnTerminal`); the `terminal/` family owns sessions,
   readiness, bounded reads, and sandbox policy on top of it.
6. ~~**guard/timeout-policy vs bash-local fused deadline**~~ — **answered
   by the guard top-up read**: guard owns the generic per-tool-call budget
   (ToolDefinition `timeoutMs`); bash-local owns shell-process timeouts (its
   own 120s/600s defaults, not derived from guard); both share the one fused
   `deadline()` primitive from `dsh-timeout` (E27).
7. **storage** — REFRAMED (per author): the session package owns the log —
   storage is the explicitly non-session family — and the fail-closed reader
   is confirmed via AGENTS.md "required-on-read unless `ignorable: true`"
   (E12); residual: whether the refusing reader lives in `packages/session`
   src or only in the repo rule's prose.
8. **Landlock ABI ceiling** — main.c self-defines ABI 1–5 masks; how do
   newer kernel ABIs behave (gracefully capped at 5, or probe-failed)?
9. **windows-acl internals** — SID lifecycle mechanics beyond the
   sandbox-local view (11 unread files).
10. **pwsh parity** — does `pwsh-sandbox/helpers.ts` mirror the bash
    classifier's four-conjunctive spawn-failure test?

---

*Packet written from a prior pass's evidence base (coverage ~40% of slice),
then top-up reads folded in — see Coverage notes. Static reading only; no
files under `refs/` were modified.*

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the execution-safety gap-fill addendum. Method unchanged: static reading
only; OQs 3/4/8/9/10 answered, OQ7-residual closed, plus
subprocess/fs-local/storage focused reads.

### Open-question resolutions

- **OQ 10 (pwsh parity)** — YES, call-for-call mirror VERIFIED by direct
  read: `packages/shell/pwsh-sandbox/src/helpers.ts` (120 ln) and
  `bash-sandbox/src/helpers.ts` (116 ln) carry byte-identical logic in
  all four exported functions (`isRunnerSpawnFailure`, `classifyDenial`,
  `classifyRunnerFailure`, `matchesSignature`); only the header and the
  pwsh `jscpd:ignore` markers differ (pwsh lines 9/119). Postmortem-0004
  protocol is shared dialect. Parity is maintained by convention, not a
  generated mirror — twin drift would NOT fail `pnpm run duplication`
  (inference; knowingly accepted).
- **OQ 8 (Landlock ABI ceiling)** — gracefully capped:
  `native/landlock-run/packages/entry/src/main.c` (`restrict_self`,
  230-262) queries the kernel's highest ABI; `< 0` is the only fail-closed
  path; ABI > 5 clamps to `MAX_ABI 5` (`fs_mask_for_abi` saturates); the
  launcher reports `fully enforced` while governing exactly the ABI-1-5
  set. Honest-labeling nuance (inference): "full" = "every access THIS
  BUILD knows governed" — same partial-labeled-full class the packet
  flagged.
- **OQ 9 (windows-acl internals)** — all 12 src files of
  `packages/sandbox/sandbox-windows-acl/` read in full. SIDs: workspace
  write SID = sha256(canonical path) → two 30-bit subauthorities
  (deterministic); temp SID domain-separated; "The SID's power is defined
  solely by the ACEs that name it" (not a secret). ACEs: standing grants
  never revoked on dispose (the reuse cache) vs revocable temp grants;
  per-path `LockFileEx` serialization with non-share-delete locks;
  idempotence via exact-ACE skip (avoids tree re-propagation, "minutes on
  large workspaces"); `GRANT_MASK` excludes WRITE_DAC/WRITE_OWNER (no
  escape by DACL rewrite); record-before-grant ordering. Tokens:
  `CreateRestrictedToken(DISABLE_MAX_PRIVILEGE | LUA_TOKEN |
  WRITE_RESTRICTED)`; zero-write-SID workspace-write throws; downgrade
  safety (standing grants INERT under read-only, kept for re-upgrade);
  deliberate absences with verified failure codes (WMI 0x80041003, S-1-2-1
  → ERROR_INVALID_SID 1337); `setTokenDefaultDaclGrant` keeps new objects
  (anonymous pipes) alive under pass-2. Runner verifies SIDs re-derive
  from paths; ignores its own CTRL+C to revoke + mirror exit codes
  (full-width 32-bit). Spawn: `CREATE_SUSPENDED` → kill-on-close job →
  resume with failure-ordered cleanup; no hidden console (0xC0000142);
  drains resolved BEFORE `WaitForSingleObject`. FFI: lazy koffi, branded
  `NativePtr`, struct-size assertions, bounded offset reads,
  descriptor-owns-ACL ("verified the hard way"). Boundaries: writes-only
  restriction; temp root outside workspace, disjoint both directions.
- **OQ 3 (e2b)** — no SandboxPolicy concept exists in the family (all 5
  core files read: `e2b/src/index.ts` 182 ln; `subprocess-e2b/src/{remote,
  process 698, environment, index}.ts`). Isolation is the VM; what
  travels is ENVIRONMENT policy — `SENSITIVE_ENV_PATTERN` imported from
  `@deepseek-ai/dsh-subprocess` (shared dialect). Three-layer scrub:
  ASCII-envelope ambient read (`TODO(e2b-replace-environment)` — E2B
  merges overrides instead of replacing); scrub of `DSH_*` + sensitive;
  bootstrap empty-string tombstones. Control plane: randomized
  per-invocation `HOME` (profile-injection defense), quoted args for the
  unavoidable shell layer, chmod-700 root. Failure classification is
  STRUCTURAL: published PGID/exit files parsed as hostile input (refuse
  `pid <= 1`, non-safe-integers); `SandboxNotFoundError` = quiescence;
  ONE shared already-gone tolerance; escalation ladder ending in a
  zombie-excluding `ps` probe as the authority; `AggregateError` on
  double failures; unpublished-group rollback exploits the
  `exec env -i … setsid` chain (command PID IS the provisional PGID);
  exit truth from the out-of-band status file, preferred over SDK
  self-report; conceded numeric-PGID-reuse race
  (`TODO(e2b-pgid-identity)`).
- **OQ 4 (code-runtime)** — execution model is a fresh
  `node:worker_threads` Worker per run: "containment, not a security
  boundary: model code has bash-equivalent trust"
  (`code-runtime-worker-thread/src/index.ts:1-7`; `isolation` is
  informational). Own seam vocabulary (SIX orthogonal failure kinds; "an
  error is a FIELD on a resolved result, never a rejection"), harness
  idioms at the edges (`MAX_TIMER_DELAY_MS`; `snapshotJsonValue`;
  explained-empty companions). Hardening: `env: {}`, `execArgv: []`, heap
  cap → `worker-exit`; strip failure spawns no worker; budget is
  MEASURED busy-time (ELU every 25 ms, "fair and ungameable") with
  `maxWallMs` backstop; hostile-peer port (rebuild inbound field-by-
  field; `Object.hasOwn` bindings; JSON-snapshot replies);
  `PORTABLE_RESERVED_WORDS` = ECMAScript ∪ Python keywords; bounded
  LogBuffer ("captured output survives a mid-run termination"); exact
  byte accounting; captured intrinsics; disposal awaits worker exits;
  abort contract — "the runtime only stops asking".
- **OQ 7-residual (refusing reader)** — CLOSED: the no-clobber log
  protocol is SESSION-owned. `packages/storage/storage-json/src/atomic.ts:1-12`
  states the contrast ("unlike the session-log backend's link()+unlink()
  no-clobber protocol"); the storage family never reads, writes, or gates
  the session log.

### New findings

windows-acl (`packages/sandbox/sandbox-windows-acl/src/`): **F-A** the
ported POC was fail-open (silently ran children UNRESTRICTED) — the port
exists to fix that (`errors.ts:1-7`). **F-B** non-share-delete locks
close the lock-substitution hole. **F-C** exact-ACE skip is a
performance-correctness coupling (drift converts O(1) reuse into tree
re-propagation; inference). **F-D** init-failure cleanup distinguishes
end state from artifact. **F-E** `dispose()` under `manageDacls: false`
revokes nothing ("must not revoke under live children"). **F-F**
explained-empty companion (E29 pattern). **F-G** empirical failure modes
recorded as first-class docs with exact codes.

e2b family: **F-H** output = self-describing ASCII protocol with a
reserved completion frame; decoder throws on duplicates/data-after/
non-canonical base64 (round-trip re-encode)/truncation. **F-I** terminal
bootstrap strips login-shell noise via a per-spawn random marker,
without parsing arbitrary output; runner `rm -f`s its private files
FIRST. **F-J** terminal teardown session-keyed, mirrors the process
ladder (zombie-excluding, `group <= 1` refused); PTY leader = provisional
session leader for rollback; `signalForeground` REFUSES to SIGKILL the
terminal shell; `inputWaiting: false` with an honest capability note.
**F-K** fs-e2b reuses the ASCII envelope for PATHS; content-INDEPENDENT
version keys. **F-L** fs-e2b atomic write: per-targetKey mutex → staged
dir → `ln -T` no-clobber create / `rename` update; `FS_STALE_VERSION`
CAS; streamed byte bound "stops a post-stat grower". **F-M** both
adapters explained-empty with package-specific reasons.

subprocess seam: **F-N** the canonical env-scrub is ONE exported
heuristic (`SENSITIVE_ENV_PATTERN = /KEY|PASSWORD|SECRET|TOKEN/i`,
`subprocess/src/index.ts:37-44` + `scrubbedParentEnv()` dropping
`DSH_*`, case-insensitive); explicit env merges AFTER the scrub; exported
plain so all spawners share it — the ownership behind the lsp env-scrub
correction. **F-O** the LOCAL provider closes the PID-reuse race e2b
concedes: `ProcessIdentity = {pid, started}` (`/proc` field 22) makes
every check exact-identity; zombie exclusion; three-valued liveness
honesty; unsupported platforms throw at LOAD. **F-P** Linux
`isStdinWaiting` is a full syscall-level stdin-wait proof (fd-set decode
out of `/proc/<pid>/mem`) — the capability e2b honestly lacks.
**F-Q** local terminal teardown has an ADOPTION GATE (adopt descendants
only while the root pid provably retains the shell's start identity);
TERM → grace → KILL → survivor re-proof; `terminateForHostExit` "does
not claim quiescence"; terminate memoizes but resets on rejection;
documented sync-only scalability boundary.

fs-local (`packages/fs/fs-local/src/`, 4/4): **F-R** atomic write =
per-targetKey mutex + staged publication (`0o700` dir, `open('wx',
0o600)`, fsync); `createIfAbsent` via HARD-LINK no-replace (EEXIST →
`FS_NOT_OBSERVED`); Windows `ReplaceFileW` ACL-preserving; stale guard
runs BEFORE literal matching; only POST-commit staging-cleanup failures
swallowed. **F-S** DACL copied onto the EMPTY temp with
`PROTECTED_DACL_SECURITY_INFORMATION`. **F-T** version tokens
content-INDEPENDENT (`dev:ino:size:mtimeNs:ctimeNs`) — one stale-guard
vocabulary local + remote; `readTextForDiff` = "presentation read must
not fail the write". **F-U** explained-empty companion.

storage: **F-V** the hub performs NO IO by design; the KV unit
explicitly does NOT serialize concurrent writes (one write chain per
unit at the domain layer — same rule as fs-local's `withLock`, one layer
up); stale-disposer guards. **F-W** `storage-domain`'s companion is a
REAL runtime cross-check (`domain/changed` snapshot == current state);
the empties give distinct reasons (durability belongs to the conformance
suite). **F-X** storage-json's write is deliberately simpler and says
why (rename + parent-dir fsync; Windows skips); synchronous slot
reservation; one name validator serving file names AND SQL identifiers.

### Transferable ideas added

(packet carries #1-13; new #14-#24) — **#14** base64 ASCII envelope for
control-plane reads; **#15** empty-string env tombstone when the lower
layer only MERGES; **#16** `env -i` full-replacement at BOTH layers;
**#17** randomized per-invocation HOME vs profile injection; **#18**
publish-then-verify status file (exit truth out-of-band); **#19** refuse
ids ≤ 1 from attacker-influenceable files; **#20** shared already-gone
tolerance helper (one place, both ladders); **#21** AggregateError on
double failure; **#22** zombie-excluding liveness probe; **#23** PID
start-identity adoption gate; **#24** swallow only post-commit cleanup
failures.

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| `sandbox/sandbox-windows-acl` | README-level | 100% (12/12 src) |
| `e2b/e2b` + `e2b/subprocess-e2b` + `e2b/fs-e2b` | partial/unread | 100% each (2/2, 7/7, 2/2) |
| `code-runtime` + worker-thread | unread | 100% (10/10 src) |
| `fs/fs-local` | ~30% family | 100% (4/4 src) |
| `subprocess/subprocess` | ~50% family | 1/3 src (types-only conv.; invariant unread) |
| `subprocess/subprocess-local` | ~50% family | 2/5 src (index/spawn/invariant unread) |
| `storage/storage` | README-level | 3/4 src |
| `storage/storage-json` | README-level | 3/5 src |
| `storage/storage-domain` | README-level | 1/6 src |
| `storage/storage-sqlite` | README-level | 0/4 src |

### Contradictions and corrections from this pass

- **fs-e2b path correction** (cross-round ledger): fs-e2b lives at
  `packages/e2b/fs-e2b/`, NOT `packages/fs/fs-e2b/` — the dispatch
  work-list path was wrong; evidence location moved, not content.
- **Env-scrub ownership** (cross-round ledger): the ambient
  KEY/PASSWORD/SECRET/TOKEN + `DSH_*` scrub is owned by the
  `dsh-subprocess` seam (`subprocess/src/index.ts:37-66`, F-N), imported
  by e2b and lsp-stdio — relevant wherever the frozen body discusses env
  scrubbing.
- No new code contradictions. One flagged GAP (not a contradiction): the
  seam promises tree-scoped termination "on every platform" while
  `createProcessInspector` throws off linux/darwin — the non-terminal
  spawn path is UNVERIFIED (`subprocess-local/src/spawn.ts` unread).
