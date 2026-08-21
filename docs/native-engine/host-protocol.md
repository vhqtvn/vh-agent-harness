# Native Engine — Host Protocol (v1)

Status: implemented (slice 5) · `internal/protocol` · `ProtocolVersion = 1`

The stable communication interface of the headless native engine:
**versioned JSON-RPC over newline-delimited JSON (NDJSON) on stdio**.
Frontends are external clients (no first-party UI); vh-solara-class
consumers are the motivating example. Design refs:
`researches/sources/deepseek-harness/llm-protocols-tools.md` (dsh SDK
transport, F-SDK-1/F-PIPE-2, malformed-line skip, close ladder) and
`solution-brief v3` (operator decisions 2 & 5; Architecture Map
"Frontend" row; risks R5′/R9).

## 1. Framing

- One JSON object per line, UTF-8, `\n`-terminated. Max line: 16 MiB.
- Both directions share the grammar. **stdout is protocol**: the server
  writes nothing else to it (diagnostics go to stderr).
- Trust boundary: **stdio-local**. The two pipe endpoints are
  co-trusted (same user, one process tree). No authentication,
  authorization-on-transport, or multi-tenant isolation is provided or
  implied — that is a stated non-goal for v1.

## 2. Message grammar

| Shape | Meaning |
|---|---|
| `{"jsonrpc":"2.0","id":N,"method":"…","params":{…}}` | request (response expected) |
| `{"jsonrpc":"2.0","id":N,"result":{…}}` | success response |
| `{"jsonrpc":"2.0","id":N,"error":{"code":C,"message":"…","data":…}}` | error response |
| `{"jsonrpc":"2.0","method":"…","params":{…}}` | notification (no response) |

- `id` is an integer ≥ 0, minted by the requester; correlation is by
  exact id. String/null ids are invalid in v1.
- `result` and `error` are mutually exclusive; exactly one is present
  on a response. `data` is optional, reserved for diagnostics.
- Requests are answered in any order (handlers run concurrently).

**Inbound strictness (fail-closed):** unknown envelope fields, wrong
`jsonrpc` value, contradictory shapes, and non-integer ids are
**invalid** — attributable lines (readable id) get `-32600`; the rest
are skipped with a `protocol/error` notification. Unparseable lines
are skipped with `protocol/error` `-32700` and the connection **stays
alive** (dsh malformed-line skip). Unknown `params` fields for a known
method are `-32602`.

## 3. Version negotiation

`initialize` MUST precede every other request. Params:
`{"protocolVersion":1}`. Response:
`{"protocolVersion":1,"serverInfo":{"name":"vh-agent-harness"},"capabilities":{"approval":true,"jobs":true,"eventNotifications":true}}`.
Mismatch ⇒ `-32002` with `data {"server":S,"client":C}` and **no
partial state** (retry with the right version is legal). Pre-init
requests ⇒ `-32001`. `initialize` is idempotent.

## 4. Methods

| Method | Params | Result | Notes |
|---|---|---|---|
| `initialize` | `{protocolVersion}` | §3 | handshake |
| `session/create` | `{path?, sessionId?}` | `{sessionId, path}` | new durable log (file); becomes the single active session; a later create supersedes; `path`/`sessionId` confined per §4a |
| `session/subscribe` | `{types?}` | `{subscribed:true}` | live-only event stream (see §5); filter = session event types |
| `session/prompt` | `{text}` | `{content, toolCalls[], results[]}` | ONE synchronous tool turn (RunTurn); retries/multi-turn stay engine-internal |
| `session/dispatch` | `{kind, payload?}` | `{jobId}` | enqueue receipt, returns **before** execution (see §6) |
| `session/surface` | `{}` | `{messages[]}` | current DeriveMessages snapshot; emits pending job reports first |
| `approval/respond` | `{approvalId, allow, reason?}` | `{resolved:true}` | one-shot answer to an approval/request |
| `jobs/status` | `{}` | `{jobs[]}` | fold-derived, enqueue order; `{jobs:[]}` without a session |
| `subagent/spawn` | `{role?, prompt, mode, seedFromParent?}` | `{childId}` | enqueue receipt, returns **before** the child's first turn (§7b); `mode` = `oneshot\|continuable`; depth auto-derived (never client-supplied) |
| `subagent/send` | `{childId, message}` | `{queued:true}` | one follow-up inbox message + one queued child turn; continuable, not-yet-settled children only |
| `subagent/list` | `{}` | `{children[]}` | fold-derived snapshot (running/waiting/settled + `contentSeq`); `{children:[]}` without a session |
| `schedule/add` | `{name, kind?, after?\|at?, every?, payload?}` | `{name, kind?, at, every?, payload?, nextRun}` | registers one schedule (§4c); UTC-canonicalized; registration persists immediately |
| `schedule/list` | `{}` | `{schedules[]}` | dispatch-priority order with UTC `nextRun` cursors; `{schedules:[]}` without a session/seam |
| `schedule/remove` | `{name}` | `{removed:true}` | unregisters + persists atomically; unknown name ⇒ `-32602` (§4c) |

`session/prompt`, `session/dispatch`, `session/surface`, `subagent/spawn`,
`subagent/send`, `schedule/add`, `schedule/remove` require an active
session (`-32003`). `subagent/spawn`/
`subagent/send` on an engine built without a subagent executor fail closed
`-32000`; manager refusals (depth fence, unknown/settled child, one-shot
send) are `-32602` carrying the manager's descriptive text.
`schedule/add`/`schedule/remove` on an engine built without scheduler
wiring fail closed `-32000` (spec-validation and refusal texts are
`-32602` carrying the scheduler's descriptive text).

## 4a. Confinement contract (server-side, wire shape unchanged)

Every client-controlled string that can steer a filesystem path is
validated server-side BEFORE any filesystem or server state changes.
Rejected inputs surface as `-32602` (`*SessionPathError`) with no
partial state — no file created or truncated, no session superseded.

- **`sessionId`** — validated as a strict single filename component
  `^[A-Za-z0-9][A-Za-z0-9._-]*$` (also rejecting `.` / `..` / any path
  separator) on EVERY `session/create`, regardless of branch: the
  default-path branch composes `<sessionDir>/<sessionId>.jsonl`, and
  `filepath.Join` lexically cleans, so an unvalidated id
  (`"../../victim"`) escapes the session root. The id also names the
  per-session subagents directory, so it is confined even when an
  explicit `path` is supplied. The same grammar is re-enforced at the
  subagents FileStore (defense-in-depth against forged logs: child
  ids folded from `subagent/spawned` events are validated before
  `<root>/<parentSessionID>/<childID>.jsonl` is composed, on create
  AND reopen).
- **explicit `path`** — confined symlink-safe to the engine's declared
  session root: lexical containment (no `..` climb, not the root
  itself, absolute paths only when already inside), then
  `EvalSymlinks` on the root and the target's parent (a symlinked
  parent resolving outside is rejected; unresolvable locations reject
  — an unresolvable location is an unknown location), and a symlink AT
  the final component is rejected outright (`os.Create` is `O_TRUNC`
  and would truncate the symlink's target).
- **engine-minted session ids** — `sess-<16 hex>` from `crypto/rand`,
  fail-closed: an entropy failure refuses `session/create` (`-32000`)
  rather than degrading to a guessable time-derived id.
- **error-after-create hygiene** — any engine failure after the
  session file is created closes the descriptor and REMOVES the
  partial file; a refused or failed create never abandons a truncated
  log.
- **`run_shell` `workdir`** — the workdir argument steers where the
  command executes, so it carries the same confinement discipline:
  empty workdir = the engine working directory (unchanged); relative
  workdirs must stay lexically inside it (no leading `..`); absolute
  workdirs are REJECTED unless the daemon configured them under a
  `WorkdirRoots` entry and they resolve symlink-safe inside one
  (conservative default: the daemon configures no roots, so absolute
  workdirs are refused). Job kinds, schedule names (slug-validated),
  and prompt artifact names (content-hash-derived) never reach
  filesystem paths.

## 4b. Subagents (B2)

- **spawn** validates params, enforces the depth fence fail-closed (a
  parent at max delegation depth gets `-32602` before any durable
  effect), creates the child's OWN session log (header carries
  `parentSessionID` + `delegationDepth`), records the durable
  `subagent/spawned` descriptor in the parent log, delivers the initial
  prompt as the child's first `subagent/message` (inbox), and queues the
  initial turn. One-shot children auto-settle when their run completes
  (`result:completed|failed` + the run error text as `reason`);
  continuable children stay `waiting` — settlement is manager-owned.
- **seedFromParent `<n>`** (fork turn-prefix seeding, the dsh fork
  pattern): copies the parent's last-n **COMPLETED** turns' surface
  messages into the child log as seed events BEFORE the child's first
  turn. Seed vocabulary is closed and replay-deterministic: exactly the
  existing message-bearing events `session/prompt`, `llm/response`,
  `tool/result` — re-appended with verbatim payloads and append
  surfaceOps; no new event kinds. Turn selection spans
  `turn/begin…turn/end` brackets whose `turn/end` kind is `""`/`"ok"`
  (errored/unterminated brackets are NOT seed material). Fewer turns
  available ⇒ fewer seeded. The `subagent/spawned` payload records the
  number of TURNS actually seeded as `seedTurns` (additive, omitempty —
  absent on pre-B2 records).
- **send** appends one `subagent/message` to the child inbox and queues
  exactly one turn (FIFO); the child's turn answers the accumulated
  inbox, not a fresh `session/prompt`.
- **list** is a pure fold of the parent log (spawn order): state
  `running` (one-shot, unsettled) > `waiting` (continuable, quiescent)
  > `settled`; `contentSeq` is the highest child-side origin seq already
  relayed by a `subagent/report` (0 = nothing reported).
- **Events**: the child's `subagent/report` and `subagent/settled`
  records are ordinary parent-log session events and reach subscribers
  through the existing `session/event` fan-out (§5) — no new
  notification kind.

## 4c. Schedules (B3)

The `schedule/*` family is the wire surface of the session scheduler
(internal/jobs' Scheduler): registration, snapshot, removal. The
scheduler itself is **engine wiring** — constructed, started, and
drained by the composition root (vh-agentd: state file
`<session-dir>/scheduler-state.json`, tracker-routed to the active
session's jobs.Manager, started before Serve, drained Stop at
shutdown) — never owned by the protocol package. The per-session seam
(`EngineSession.Schedules`) is what the handlers drive.

- **add** `{name, kind?, after?|at?, every?, payload?}` validates
  fail-closed (the jobs-side table-tested rules: lowercase-slug `name`
  and `kind`, exactly one start — `after` (relative delay, integer
  nanoseconds) XOR `at` (RFC3339 instant; a non-UTC zone is
  canonicalized to UTC, a naive local time is rejected as
  machine-dependent) — positive `every` for recurring, valid-JSON
  `payload`) and registers the schedule. The result is the CANONICAL
  record — `after` resolved into `at`, `at` in UTC — plus `nextRun`,
  the first due instant (UTC). An omitted `kind` stays omitted in the
  record: the dispatched job derives it as `sched-<name>` at dispatch
  time. Durations travel as integer nanoseconds (the persisted state
  form). A duplicate name is `-32602` (the name is the schedule's
  identity). Registration persists immediately (atomic
  temp+fsync+rename) — a schedule survives a crash right after add.
  A persist/infrastructure failure (state file unwritable) is `-32000`
  carrying the underlying text: the params were valid, the engine's
  state layer failed (only validation refusals and unknown names are
  `-32602`).
- **list** `{}` returns `{schedules:[…]}` in dispatch-priority order
  (`nextRun`, then name), each entry the canonical record + `nextRun`
  (UTC). Without an active session — or on an engine built without the
  schedule seam — it is an honest empty list (the `jobs/status`
  mirror: absent wiring means absent schedules).
- **remove** `{name}` unregisters and persists atomically
  (`{removed:true}`) — persist-first: a failed persist changes neither
  the live snapshot nor the state file (the schedule lands in both or
  neither), returning `-32000`; an unknown name is `-32602` carrying
  the typed `scheduler: schedule not found` text. Remove+re-add is the
  v1 pause path (a `schedule/pause` method is a stated non-goal).

**Dispatch semantics (engine-side, never wire params):** the idle
gate, one-dispatch-per-pass, and catch-up collapse are the scheduler's
documented contract (internal/jobs). Dispatch waits for executor idle
(queued|running jobs of the owner); at most ONE dispatch fires per
pass; fixed-rate catch-up collapses to the LATEST due occurrence (one
dispatch per due gap — no storm replay after downtime). A due schedule
dispatches as an ordinary `job/enqueued` (`<kind>-N`, kind defaulting
to `sched-<name>`) through the active session's jobs.Manager, so
settlement and reporting ride the EXISTING job/* event stream,
`jobs/status` fold, and reported-flag discipline — no new notification
kind, no scheduler-specific event vocabulary. Wire handlers touch only
the scheduler's non-blocking registration seams; Tick/dispatch stay on
the scheduler's own goroutine.

**Persistence / at-least-once:** schedule cadence lives in
`<session-dir>/scheduler-state.json` (v1 schema), NOT in the session
log — the log stays the source of job truth, the state file is
scheduler-owned. State survives daemon restart: a fresh scheduler
adopts the persisted spec list and next-run cursors. The cursor is
persisted AFTER the dispatch decision (at-least-once): a crash between
dispatch and persist re-dispatches on restart; duplicate suppression
is the job layer's first-wins settlement + reported-flag notices, not
the scheduler's. Idempotency of the underlying work is the dispatched
job body's concern.

**Versioning (B3 decision, mirroring B2):** `ProtocolVersion` stays
1 — the three methods are NEW method names (additive under §8); no
field was added to any existing method's params or result, and the
`initialize` capabilities object is untouched (a capability field-add
would itself be a breaking change).

## 4d. Oversize-result spill (dsh session-cognition pattern)

Spill replaces lossy truncation **at commit time**: it is engine
wiring around the session log, NOT a wire surface. `run_shell`'s
per-stream capture cap (64 KiB + in-band truncation marker) is
unchanged and fires FIRST; spill then applies to the serialized tool
result when the session log is armed with a `SpillPolicy`
(vh-agentd: `--spill-max-inline`, default 65536; `0` disables —
library default, no store configured, is byte-identical to the
pre-spill behavior).

- **Decision point.** When a committed `tool/result` payload's content
  exceeds `MaxInlineBytes`, the content is written to a
  content-addressed spill file under
  `<session-dir>/<session-id>.spill/` (dir 0700, files 0600, name
  `<kind>-<sha256[:16]>` — identical output de-duplicates to one
  file). Files materialize by **temp + atomic rename**: the full
  content is staged in a private temp file (`tmp-<sha16>-<rand>`,
  0600, exclusive-create), fsynced, then renamed onto the
  content-addressed name — the final name never hosts a partial file,
  concurrent same-content writers converge by a harmless
  byte-identical overwrite, and rename replaces (never writes
  through) a pre-existing path. The logged content becomes a
  **preview** — first `cap-1-len(notice)` bytes plus a notice line
  `... [spilled <N> bytes: <locator-JSON> — read via
  spill_read]` (the notice bytes are reserved INSIDE the inline cap),
  and the payload carries additive `spilled` + `spillLocator` fields.
  On spill-write failure the FULL content stays inline — silent but
  deterministic fallback (a spill failure must never fail the tool
  result), so the event shape stays identical to today's.
- **Locator opacity.** `SpillLocator {file, sha256, size}` is an
  opaque JSON token: the model echoes it verbatim into `spill_read`;
  nothing about the on-disk layout is part of its contract. `Read`
  validates basename + size + sha256 and fails closed on mismatch.
- **Retrieval (windowed paging).** `spill_read` is an ordinary tool
  in the daemon catalog (guards, logging, and pipeline apply). Input:
  the locator object plus optional `offset` (int64, default 0) and
  `length` (int; 0 = the default window = the ACTIVE policy's
  `MaxInlineBytes`, resolved server-side; explicit values clamp to
  the same cap). Output: ONLY the `[offset, offset+length)` window of
  the stored content — a bounded section read; the full file is
  streamed through sha256 for validation but never buffered whole.
  Unless the window covers the whole content, a trailing `[window
  offset=O length=L of SIZE bytes — adjust offset/length to page]`
  notice rides in-band, its bytes reserved INSIDE the cap (the same
  discipline as the spill preview) — so **a retrieval result always
  fits inline by construction**: it can never be re-spilled by the
  commit-time policy, and retrieval pages REAL spilled bytes into the
  model's context. (Default-compat note: with no offset/length the
  FIRST window comes back — this IS a behavior change from the
  pre-windowing full-content return, which was a model-visible no-op:
  an oversize retrieval was re-spilled at commit, content addressing
  deduped it to the SAME locator, and the model saw a byte-identical
  preview. Paging arithmetic: the next window is
  `offset = offset+length`; `offset` past `SIZE` fails closed with
  the size in the error.) Hash validation stays FULL-FILE: a tampered
  or truncated store file refuses even a healthy window. When the
  daemon config is absent the tool still registers (for pipeline
  tests) and fails closed on unknown files.
- **Replay independence.** Spill files are durable SIDECAR state, not
  log inputs: `Replay` and `DeriveMessages` are a pure fold over the
  log and never touch the filesystem. Log replay is byte-identical
  regardless of spill-file existence; losing the spill directory
  degrades retrieval (`spill_read` fails closed), never replay
  integrity.
- **Divergences, stated.** Child/subagent session logs are NOT
  spill-armed (v1 arms only the parent session the daemon composes);
  `spill_read` locates files by a bounded walk of the session dir
  (O(session-dir), not a global index); invalid-UTF-8 bytes are
  lossy through JSON strings — the sha256 is computed over the exact
  bytes written, so validation still holds.

**Versioning (spill decision, mirroring B2/B3):**
`ProtocolVersion` stays 1 — no wire method changed. The additive log
payload fields `tool/result .spilled`/`.spillLocator` are omitempty
(old logs byte-stable, verified); `tools.Result` on the WIRE is
untouched (§8's no-new-fields rule on existing method shapes holds);
`spill_read` travels through the existing `tool/call`/`tool/result`
events, so no new fixtures were needed.

## 7b. Async contract (subagents, B2)

Same discipline as §7 jobs: `subagent/spawn` returns its `{childId}`
receipt immediately — never blocked on the child's turn (dsh async
contract). Child turns run on the session's subagent dispatch goroutine
(serial FIFO, one child turn at a time per session), so a long child
turn cannot block the protocol loop, a concurrent `session/prompt`, or
spawn/send receipts. Lock order is one-directional (manager lock →
parent-log write lock; child turns hold no manager lock while
executing), which keeps spawn-from-protocol concurrent-safe. A FAILING
child executor settles cleanly: the settle notice lands
(`result:"failed"`, reason = the executor error) and the protocol loop
stays responsive — no hang, no lost settlement.

## 5. Event notifications

| Method | Params | Source |
|---|---|---|
| `session/event` | one full session `Event` record (`{seq,type,payload,…}`) | every appended session-log event (dsh `session.event` unfiltered stream; `types` filter optional) |
| `approval/request` | `{approvalId, call:{id,name,args}, reason}` | an ask verdict reaching the approval bridge |
| `protocol/error` | `{code, message}` | malformed/unattributable inbound line |

The stream is a **projection** of the durable log (the log is the
source of truth; it never fails because a subscriber is broken). It is
live-only: events appended before subscribing are not replayed —
history is read via `session/surface` or log replay. Session event
types map 1:1 to the log's closed type set (`session/header`,
`session/prompt`, `llm/request`, `llm/response`, `tool/call`,
`tool/result`, `turn/begin`, `turn/end`, `compaction/*`, `llm/retry*`,
`job/*`, `subagent/*`).

## 6. Approval model (fail-closed)

`ask` (pre-execute waterfall) → `approval/request` notification →
client answers `approval/respond` → decision. Every unanswerable
direction denies (dsh F-PIPE-2: absent/unanswerable approval = deny):

- no response within the server's approval timeout ⇒ **deny**;
- connection closed while pending ⇒ **deny all pending**;
- respond for unknown/expired/answered id ⇒ `-32004` (never re-opens a
  decided approval — one-shot, `allow-once`/`reject-once` only);
- no approver configured (engine built without the bridge) ⇒ deny at
  the pipeline (slice-3 semantics unchanged).

## 7. Async contract (jobs, R9 on the wire)

`session/dispatch` → receipt `{jobId:"<kind>-N"}` immediately →
`job/enqueued`, `job/started`, `job/settled` events stream to
subscribers → `jobs/status` reflects the fold (`queued|running|settled`
+ `completed|failed`). Settlement is first-wins; the `reported` guard
emits the model-facing notice exactly once (at the next
`session/prompt`/`session/surface`, before the surface is derived).
**Recovery is visible**: after a crash, the R9 recovery pass re-dispatches
never-started jobs (at-least-once), synthetically settles torn tails
(`result:"failed"`, reason `recovered-after-crash`), and emits pending
reports — all as ordinary appended events on the same stream.

## 8. Extensibility & versioning

- Unknown **method** ⇒ `-32601`. Unknown **notification** ⇒ ignored.
  Unknown `session/event` payload types ⇒ clients ignore them.
- Adding fields to existing params/results is a **breaking** change
  under v1 strictness (unknown fields are rejected) ⇒ bump
  `ProtocolVersion`.
- Wire shapes are locked byte-exact by golden fixtures
  (`internal/protocol/testdata/`, `compat_test.go`): any shape change
  without regenerating fixtures (a reviewable diff) fails CI; the
  initialize fixture pins the version constant (R5′ mitigation).
- **B2 decision (subagent/* family): ProtocolVersion stays 1.** The
  three methods are NEW method names — additive under the rule above.
  No field was added to any existing method's params or result, and the
  `initialize` capabilities object is untouched (a capability field-add
  would itself be a breaking change ⇒ version bump). The additive log
  payload field `subagent/spawned.seedTurns` is omitempty, so existing
  bytes (and pre-B2 logs) are unaffected; old fixtures verified
  byte-stable.
- **B3 decision (schedule/* family): ProtocolVersion stays 1** — same
  rule, same verification (§4c; old fixtures byte-stable, new methods'
  shapes locked by their own fixtures).

## 9. Error codes

| Code | Meaning |
|---|---|
| `-32700` | parse error (line skipped, `protocol/error` emitted) |
| `-32600` | invalid request (shape/unknown envelope field/closing) |
| `-32601` | method not found |
| `-32602` | invalid params (incl. unknown params fields) |
| `-32000` | engine error (internal handler failure) |
| `-32001` | initialize required |
| `-32002` | protocol version mismatch |
| `-32003` | no active session |
| `-32004` | unknown/expired approval id |

## 10. Close semantics

Server: `Close()` (or ctx cancel / EOF) ⇒ new requests rejected with
`-32600 "server is closing"` and the transport closed IMMEDIATELY
(close-before-drain) ⇒ pending approvals deny at once (fail-closed;
this is why `Close` precedes the drain — with approval-timeout 0 a
drain-first ladder would hang forever) ⇒ in-flight request handlers
drain. Background jobs are durable and continue per §7 (their
lifecycle belongs to the log, not the connection). Client close ladder
mirrors dsh simplified for stdio: close stdin/write side → EOF → done
(no signals required).

## Non-goals (v1)

HTTP/WS transport; streaming token deltas (committed events only);
multi-session muxing beyond create/subscribe (single active session);
transport auth (stdio-local trust boundary, §1); server→client
requests (dead capability, like dsh); job cancellation over the wire.
