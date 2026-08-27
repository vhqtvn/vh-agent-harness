# Native Engine — Host Protocol (v1)

Status: implemented (surface: session, approval, jobs, subagent, and
schedule families) · `internal/protocol` · `ProtocolVersion = 1`

The stable communication interface of the headless native engine:
**versioned JSON-RPC over newline-delimited JSON (NDJSON) on stdio**.
Frontends are external clients (no first-party UI); vh-solara-class
consumers are the motivating example. Design refs:
`researches/sources/deepseek-harness/llm-protocols-tools.md` (dsh SDK
transport, F-SDK-1/F-PIPE-2, malformed-line skip, close ladder) and
`solution-brief v3` (operator decisions 2 & 5; Architecture Map
"Frontend" row; risks R5′/R9). The reference CLI client
(`cmd/vh-agent-client`) is the canonical protocol-consumer example for
this wire.

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
| `session/resume` | `{sessionId}` | `{sessionId, path, events, messages[], title, usage, unsettledJobs?}` | open the EXISTING log (never creates; the only truncation is the torn-tail committed-prefix trim — absent ⇒ typed `-32602` `not-found`), recover a torn tail, replay, derive the surface, make it active with create's supersede semantics for a DIFFERENT id (§4e); the engine's CURRENTLY-ACTIVE id ⇒ typed `-32602` `session-active` (a second live writer on the open log would corrupt the stream); child sessions (header `parentSessionId`) refused naming the parent |
| `session/list` | `{}` | `{sessions[]}` | resumable top-level sessions under the session dir: `{sessionId, title, events, lastActivity}`, newest-activity first (§4e); no active session required |
| `session/subscribe` | `{types?}` | `{subscribed:true}` | live-only event stream (see §5); filter = session event types |
| `session/prompt` | `{text}` | `{content, toolCalls[], results[]}` | ONE synchronous tool turn (RunTurn); retries/multi-turn stay engine-internal |
| `session/dispatch` | `{kind, payload?}` | `{jobId}` | enqueue receipt, returns **before** execution (see §6) |
| `session/surface` | `{}` | `{messages[]}` | current DeriveMessages snapshot; emits pending job reports first |
| `approval/respond` | `{approvalId, allow, reason?}` | `{resolved:true}` | one-shot answer to an approval/request |
| `jobs/status` | `{}` | `{jobs[]}` | fold-derived, enqueue order; `{jobs:[]}` without a session |
| `jobs/output` | `{jobId, offset}` | `{jobId, state, chunk, offset, nextOffset, hasMore, written, evictedBytes}` | offset-cursor read of a job's captured output (§4g); requires an active session (`-32003`); strict params; typed `-32602` errors for unknown job / behind-retention / ahead-of-output |
| `subagent/spawn` | `{role?, prompt, mode, seedFromParent?}` | `{childId}` | enqueue receipt, returns **before** the child's first turn (§7b); `mode` = `oneshot\|continuable`; depth auto-derived (never client-supplied) |
| `subagent/send` | `{childId, message}` | `{queued:true}` | one follow-up inbox message + one queued child turn; continuable, not-yet-settled children only |
| `subagent/list` | `{}` | `{children[]}` | fold-derived snapshot (running/waiting/settled + `contentSeq`); `{children:[]}` without a session |
| `schedule/add` | `{name, kind?, after?\|at?, every?, payload?}` | `{name, kind?, at, every?, payload?, nextRun}` | registers one schedule (§4c); UTC-canonicalized; registration persists immediately |
| `schedule/list` | `{}` | `{schedules[]}` | dispatch-priority order with UTC `nextRun` cursors; `{schedules:[]}` without a session/seam |
| `schedule/remove` | `{name}` | `{removed:true}` | unregisters + persists atomically; unknown name ⇒ `-32602` (§4c) |

`session/prompt`, `session/dispatch`, `session/surface`, `subagent/spawn`,
`subagent/send`, `schedule/add`, `schedule/remove` require an active
session (`-32003`). `session/resume` and `session/list` do NOT — they
are the discovery/establishment surface (a client lists, then resumes);
a successful `session/resume` MAKES a session active. `subagent/spawn`/
`subagent/send` on an engine built without a subagent executor fail closed
`-32000`; manager refusals (depth fence, unknown/settled child, one-shot
send) are `-32602` carrying the manager's descriptive text.
`schedule/add`/`schedule/remove` on an engine built without scheduler
wiring fail closed `-32000` (spec-validation and refusal texts are
`-32602` carrying the scheduler's descriptive text).

### Concurrency & session replacement (v1 contract)

Handlers run concurrently (§2: responses may arrive in any order) — the
read loop never blocks behind a slow request. On top of that fan-out,
the server guarantees the following per-session semantics (the log's own
lock yields valid JSONL and contiguous seq for ANY concurrent appends;
these rules are what make a TURN atomic, not just its records):

- **Concurrent (no ordering guarantees, safe to overlap):** every
  read-only or receipt-shaped method — `initialize`,
  `session/subscribe`, `session/surface`, `session/dispatch`,
  `jobs/status`, `jobs/output`, `subagent/spawn`, `subagent/send`, `subagent/list`,
  `schedule/add`, `schedule/list`, `schedule/remove`,
  `approval/respond` — plus all background job and child-turn event
  appends. Background job events may interleave WITHIN an open turn
  bracket (that is the §7 async contract, unchanged).
- **Serialized per session:** `session/prompt`. At most ONE
  `turn/begin…turn/end` bracket is in flight per session log
  (a per-session turn gate in the engine layer); concurrent prompts
  against the same session QUEUE and execute one bracket at a time, so
  a replayed log never shows interleaved turn brackets; surface
  derivations may run concurrently with an in-flight turn and observe
  its already-committed events (whole-record granularity, never
  interleaved brackets). Different sessions' turns run concurrently
  with each other.
- **Serialized against each other:** `session/create` AND
  `session/resume`. The whole create/resume critical section — engine
  session construction or recovery (including any engine-decorator
  active tracking, which happens synchronously inside
  `engine.NewSession`/`engine.ResumeSession`) and the server's
  active-pointer swap — runs as one non-interleaved stage. Concurrent
  creates/resumes therefore cannot leave the engine, tracker, and
  server disagreeing about which session is active; every created
  session is complete on disk (valid header, no partial log) when its
  create returns, and every resumed session has recovered its torn
  tail (if any) before it becomes active.
- **Replacement semantics (create while a prompt is in flight).** A
  prompt's ADMISSION — which session it runs against — is one atomic
  active-session resolution. A create that supersedes after admission
  does NOT cancel or wait for the admitted prompt: the prompt's turn
  completes atomically on ITS session (its bracket lands wholly in that
  session's log), and the new session becomes active as soon as the
  create's critical section ends (creates do not queue behind turn
  gates). The superseded session's subagent manager stops accepting
  spawns (§7b); its in-flight turn is unaffected.
- **Child sessions** (subagents) serialize per child through the
  manager's serial dispatch loop (§7b — one child turn at a time per
  session), so child logs never interleave brackets either.

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
  `WorkdirRoots` entry and they resolve symlink-safe inside one. The
  daemon's `--workdir-roots DIR[,DIR...]` flag (comma-separated
  absolute paths to existing directories — a non-directory entry,
  even via symlink, refuses at startup — canonicalized at startup;
  default = the daemon's working directory resolved absolute)
  configures exactly that set.
  Job kinds, schedule names (slug-validated), and prompt artifact
  names (content-hash-derived) never reach filesystem paths.
- **file tool family (`read`/`write`/`edit`/`glob`/`search`)** — the
  model-facing file tools (gap-table rows 1-4 of the parity program)
  confine every user-supplied path against the SAME `--workdir-roots`
  set, re-using the proven session-path discipline (lexical
  Rel-containment; `EvalSymlinks` on the resolved root and the
  candidate's parent; a symlink AT the final component rejected
  outright — an in-root symlink target is indistinguishable from an
  escape at check time): relative paths resolve against the FIRST
  root, absolute paths must sit under some root, and every rejection
  is a typed `isError` tool result naming the rule (`[escape]`,
  `[outside-roots]`, `[symlink-escape]`, `[symlink-final]`, …) with
  ZERO filesystem effects — a rejected `write` leaves no file and
  creates no parent directories, and the glob/search walks never
  follow symlinked directories. `write` lands content atomically
  (temp file in the target directory + fsync + rename) and creates
  missing parents only when the resolved parent is inside a root.
  These ride the existing `tool/call`/`tool/result` choreography
  inside turns (ProtocolVersion stays 1 — no new wire methods); the
  confinement lives in `internal/tools/filetools` (a dedup candidate
  against the engine's `confineSessionPath` is noted there).

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
- **Model-facing spawn tools (child-of-child)**: the daemon also arms
  the same capability as MODEL-FACING tools — `subagent_spawn`
  (`mode: oneshot|continuable`) and `subagent_send` — resolved through
  a session→manager registry, so ANY session's model can delegate,
  recursively: a child spawns grandchildren, a grandchild
  great-grandchildren, up to the same depth fence (the executing
  session's header depth is authoritative; the daemon strips the
  family from a depth-maxed session's ADVERTISED tools — capability
  absence — while a hallucinated call still gets the typed fence
  refusal as an `isError` tool result, zero durable effects). The
  one-shot tool blocks until the child settles and returns the report
  (the report independently lands as a user-role `subagent/report`
  event, same shape as the wire family); wire surface unchanged
  (ProtocolVersion stays 1 — these are tool calls inside turns, not
  new methods).

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
  preview. Paging arithmetic: the notice's `length` is the DELIVERED
  window size (the final page may be short), so the next window
  `offset = offset+length` lands at most exactly at `SIZE`; the
  terminal call at `offset == SIZE` returns an empty window with a
  `[window complete]` notice, and `offset` past `SIZE` fails closed
  with the size in the error.) Hash validation stays FULL-FILE: a tampered
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

## 4e. Session lifecycle over the wire (P4): resume, list, derived identity

`session/resume` makes sessions survivable across daemon restarts, and
`session/list` makes them discoverable. Both are additive
(ProtocolVersion stays 1); the fixtures under `internal/protocol/
testdata/` lock their shapes.

- **Resume is not fork and not create.** `session/resume {sessionId}`
  opens the EXISTING log at `<sessionDir>/<sessionId>.jsonl` through
  the crash-recovery seam (`session.ResumeFileTee`): torn-tail
  recovery (an uncommitted final fragment is dropped, the file is
  truncated to the last committed record), replay, and append-mode
  continuation of the SAME durable stream — same file, same session
  id, `seq` continues from the recovered tail. Contrast
  `session/create` with an existing id, which truncates via
  `os.Create` (the two entrances are deliberately distinct; the
  reference client never fakes resume through create).
- **Fail-closed on every branch, before any state changes.** Absent
  log ⇒ typed refusal `not-found` (`-32602`) — resume never creates a
  file; the only truncation it can perform is RecoverTail's trim of a
  TORN uncommitted tail back to the last committed record (the
  crash-recovery seam create never needs). An EMPTY log (file exists,
  zero bytes — no header; `session/list` skips it too) is the same
  typed `not-found` refusal: an empty file is not a session, and the
  refusal stays client-actionable instead of an untyped `-32000`. The
  engine's CURRENTLY-ACTIVE session id ⇒ typed refusal
  `session-active` (`-32602`): a same-id resume would open a second
  live writer on the open log, and one more append through either
  writer mints a duplicate `seq` that `validateStructure` rejects on
  every later replay (unresumable, `--verify-log` fails,
  `session/list` fails the dir closed). The active session stays
  servable through its existing seams (`session/surface`,
  `session/prompt`); resume targets a SUPERSEDED id — the
  daemon-restart story it exists for. `sessionId` is validated by the
  same strict filename-component grammar as create (§4a). A log whose
  header carries `parentSessionId` (a subagent child) ⇒ typed refusal
  `child-session` naming the parent — v1 resumes TOP-LEVEL sessions
  only; children resume through their parent's manager. A log whose
  header `sessionId` disagrees with the requested id ⇒ typed refusal
  `id-mismatch` (the header is the durable identity). Structurally
  damaged logs fail closed exactly like `--verify-log`.
- **Supersede semantics identical to create — for a DIFFERENT id.**
  The whole resume runs under the same server critical section as
  create (§4), and the engine additionally serializes its ENTIRE
  transition (validate → file open → surface derive → subagent
  supersede → active-id publish) under its own lifecycle lock, so the
  active-id record is published atomically with the transition and
  engine-direct callers outside the wire critical section get the same
  guarantee — a successful resume ALWAYS returns with its id recorded
  (the session-active refusal above cannot be bypassed by a racing
  create). The surface is derived BEFORE the supersede: a
  surface-invalid (but structurally valid) log refuses with the
  subagent registry/manager state untouched. The previous active
  session's subagent manager stops accepting spawns and its registry
  binding moves to the resumed session; an in-flight turn on the
  superseded session completes atomically on its own log. Same-id
  resume is NOT a supersede case: when the requested id IS the active
  session, the request is the `session-active` refusal above (same-id
  ≠ supersede target). Appended events reach the live notification
  fan-out from the resume point onward (the stream stays live-only —
  replayed history is never re-emitted).
- **Result shape** `{sessionId, path, events, messages[], title,
  usage, unsettledJobs?}`: the recovered event count, the derived
  surface snapshot (the same projection `session/surface` serves —
  multi-turn continuation operates on it unchanged), and the derived
  identity below. `unsettledJobs` lists fold-visible jobs with no
  terminal `job/settled` event: they are REPORTED, never silently
  re-dispatched and never synthetically settled on the interactive
  path — those are the R9 startup-recovery decisions
  (`jobs.Recover`), which daemon startup does not currently run
  (wiring it there is the documented follow-up; resume reports and
  leaves the durable state untouched).
- **Titles are DERIVED, never stored.** The session title is the
  FIRST user prompt (`session/prompt`), whitespace-collapsed to one
  line, truncated to 60 runes with a trailing ellipsis
  (`session.DeriveTitle`). No title events, no `session/setTitle`
  method in v1 (it would be new durable state; noted as future work).
  A log with no user prompt derives the empty title.
- **Token status is replay-derived.** `usage` is the SUM of every
  `llm/response` usage envelope in the log (`session.SumUsage`). The
  envelope has been logged on every response since slice 1 (the field
  is non-omitempty), so ALL logs — pre-P4 included — derive real
  totals; only providers that report no usage honestly sum to zero.
  No schema change was needed for P4.
- **`session/list` enumerates the resumable surface:** every `*.jsonl`
  directly under the session dir (children excluded by header
  topology even if parked at the top level; non-logs ignored). Each
  entry carries `{sessionId, title, events, lastActivity}` where
  `lastActivity` is the log file's mtime — session events carry no
  timestamps (the slice-1 determinism design), so the filesystem
  mtime is the only activity signal the engine can state honestly.
  Ordering: newest activity first, ties broken by `sessionId`
  ascending (deterministic). A structurally corrupt log fails the
  whole listing closed naming the file; a TORN final record is
  tolerated (the crashed session still lists).

## 4f. Compaction (P5): post-turn surface-pressure trigger

Sessions stop growing unbounded: after every successfully completed
PARENT-session `session/prompt` turn (never mid-turn, never on subagent
child turns — children are short-lived by design), the daemon checks
surface pressure against the context budget (`--context-tokens N`,
default 128000; `--compact-threshold R`, default 0.8; the estimate is
surface chars/4 anchored by the last provider `llm/response` usage
report). At or above the threshold the daemon compacts:

- **Surface-only, log-preserving.** The head of the derived message
  surface (`[0, len-retain)`, retain-tail 2) is shadowed behind ONE
  user-role summary message citing every shadowed event's seq
  (`compaction/start` → `compaction/summary` → `compaction/end` on the
  durable log; the fold's `replaceGeneration` advances). The event log
  is NEVER rewritten — replay determinism holds, and `Unfold` recovers
  the pre-compaction surface from the citations at any time.
- **KV-prefix summarize call.** The summary is ONE LLM call through
  the same adapter, built as a genuine prefix of the running
  conversation: the SAME system prompt + tool advertisements + the
  shadowed-region messages, plus exactly ONE appended user-role
  instruction — everything before the instruction is byte-identical
  to the running conversation's request, so the provider's KV cache
  absorbs it (the dsh `region.ts buildSummarizationInput` pattern).
- **Failure semantics.** A failed or refused compaction (provider
  error, empty answer, or a summary not strictly smaller than the
  shadowed span) NEVER fails the turn or the wire call: the surface
  stays un-compacted, the unmatched `compaction/start` remains as the
  durable lock, the daemon logs one stderr line, and the NEXT turn
  boundary retries — the boundary loop is the retry ladder (there is
  deliberately no retry inside the summarizer).
- **Serialization.** The trigger is a TurnRunner decorator executing
  INSIDE the handler's per-session turn gate (§4): compaction is
  serialized with turns by construction, and the `compaction/*`
  events reach subscribers before the `session/prompt` response.

Known audit gap (disclosed): the summarize call is not a turn and the
event vocabulary has no record for non-turn LLM calls, so it writes no
`llm/request` event — the `compaction/start`+`compaction/end` bracket
(pressure snapshot, shadowed range, citations, generation) is the
log-side audit trail; the request itself is observable only on the
provider plane. `--context-tokens 0` disables the subsystem outright.

## 4g. Job output tailing (P6): jobs/output + background run_shell

The daily-driver loop — run a long shell command in background, keep
chatting, tail its output, get settlement — lands as TWO additive
surfaces (ProtocolVersion stays 1; see §8):

**`jobs/output`** `{jobId, offset}` → `{jobId, state, chunk, offset,
nextOffset, hasMore, written, evictedBytes}` — the offset-cursor read
of a job's captured output, in the `spill_read` family:

- `chunk` is the served byte window (a string; may be empty — an
  honest poll answer, NOT an error) and `nextOffset = offset +
  len(chunk)` is EXACT: paging at the returned cursor never re-serves
  a byte and never skips one; a terminal read (`nextOffset ==
  written`, `hasMore:false`) has consumed the whole stream.
- `state` is the fold state (`queued|running|settled`); reads of a
  queued job or a running job with no bytes yet are empty responses.
- The chunk is bounded server-side (16 KiB); `hasMore:true` marks a
  chunk-bound cut with more unread bytes available NOW. Truncation is
  signaled STRUCTURALLY (`hasMore`, `evictedBytes`), never by injected
  marker bytes — in-band markers would corrupt byte-offset reassembly.
- Typed `-32602` errors: `unknown job` (the fold does not know the
  id); `offset behind the retention window` (data
  `{kind:"output-evicted", evictedBase, evicted}` — the oldest bytes
  were dropped by the ring; re-sync to `evictedBase`); `offset ahead
  of the produced output` (data `{kind:"output-ahead", written}` — a
  client arithmetic bug; `nextOffset` never exceeds `written`).

**RETENTION POSTURE (deliberate v1, disclosed):**

- **In-memory, non-durable.** Captured output lives in the session's
  jobs manager (an accelerator), NOT in the session log. Across a
  daemon restart the job's SETTLEMENT facts survive (they are log
  events) but its captured output does not: a read at offset 0 reports
  `written:0`, and a read at a stale pre-restart offset (beyond
  `written:0`) is the typed `output-ahead` error above — the client's
  one-shot clamp resolves it and completes the drain over the honest
  absence. Durability would need a per-job spill store plus recovery
  wiring — deferred; the log stays byte-stable.
- **Tail-keeping ring, 256 KiB per job** (the most recent bytes are
  retained; older bytes evict as the producer wraps). Post-settle the
  buffer freezes: reads keep serving the tail within retention for
  the session's lifetime. Buffer memory is bounded per job (≤256 KiB)
  and by the per-owner in-flight cap while running; settled tails
  live until the session is superseded.
- **One combined stream.** A job has ONE byte stream (a background
  shell's stdout and stderr interleave in write order). Per-stream
  cursors would double the read surface; the sync run_shell result
  already captures per-stream for its frozen outcome.
- **jobs/status stays fold-pure.** Exposing `hasMore`/`nextOffset`
  there was considered and DECLINED: output cursors are in-memory
  accelerator state, not fold state — coupling the fold projection to
  non-durable memory would blur the log-is-truth discipline.

**Background `run_shell`:** the tool gains a `background:true` arg
(schema-documented). The tool body runs the SAME validation
(command policy, workdir confinement), then dispatches a durable job
(kind `shell`, id `shell-N`) whose body runs the SAME exec path (env
scrub, sandbox `WrapCommand`, workdir policy, process-group teardown —
reused, not duplicated) with the child's combined output streaming
into the capture channel above. The tool result is the immediate
receipt `{background:true, jobId, command, effectiveTimeoutMs}` (the
turn never blocks); `job/settled` and `job/report` carry the exit
facts in a compact `detail` (`cause=exit exitCode=0 durationMs=…
outputBytes=… sandbox=…`) and the report notice enters the surface as
`background job shell-1 completed (…)`. Timeout semantics: the SAME
per-call vocabulary and 600000ms hard cap, but an OMITTED `timeout_ms`
defaults to the CAP in background mode (long-running intent; a
surprise 30s kill would defeat the feature) — expiry kills the process
group by the identical teardown and settles the job FAILED with the
timeout reason. Non-zero exits are normal outcomes (job settles
`completed` with the facts). Sandbox posture: fail-closed as ever —
a configured sandbox that is UNAVAILABLE at dispatch refuses the
dispatch typed (no job created, never unconfined); per-call setup
failures inside the job settle it failed, never unconfined.

Client posture (honest scope): the one-shot client DRAINS observed
background jobs to settlement after the conversation — polling
`jobs/status`/`jobs/output` (every non-empty chunk rendered, plus a
ONE-TIME empty terminal record when a job is settled and fully
consumed — the deterministic end-of-tail marker; in `--json` mode
these are CLIENT-SYNTHESIZED `{"kind":"job-output"}` NDJSON records, a
  shape the daemon never emits) and calling `session/surface` once at
  the end so pending `job/report` notices land. On the typed
  output-evicted `-32602` the drain re-syncs its cursor FORWARD to
  `data.evictedBase` (one note names the honestly-absent prefix;
  `evictedBytes` carries it structurally), so a settled job whose early
  output fell behind retention still completes its drain over the
  retained tail; on the typed output-ahead `-32602` it clamps BACK to
  `data.written` — including the recovered-job case after a daemon
  restart, where the server types the stale pre-restart cursor as ahead
  with `written:0` and the clamp plus the one-time empty terminal
  record at 0 complete the drain with the honest-absence note. Both
  re-syncs are one-shot per call. REPL-mode tailing
(slash-commands) is a deferred slice: in the REPL, background jobs
keep running and settle on the log, but the client does not tail them
interactively.

## 4h. MCP host (P8): engine-side tools, no wire changes

The daemon becomes an MCP HOST (`--mcp-config PATH`,
`--mcp-timeout-ms MS`): it consumes the operator's opencode MCP
config (local stdio servers + remote Streamable-HTTP servers),
launches/initializes each server at daemon startup, discovers its
tools, and registers them in the guarded tool registry namespaced
`mcp_<server>_<tool>`.

**NO WIRE CHANGES — stated explicitly.** MCP tools ride the EXISTING
tool events end-to-end: the model calls them as ordinary tool calls;
`tool/call` (pre-execution) and `tool/result` land on the durable log
exactly as for every built-in tool; approvals (when the server is
ask-listed) use the existing `approval/request`/`approval/response`
bridge; results replay from the log via the unchanged
`--verify-log` prover (an MCP call needs NO server at replay time —
the logged content IS the result, same as every tool). The protocol
`ProtocolVersion` stays 1; every pre-P8 fixture replays byte-identical
(a server that never advertised MCP tools sends requests that are
byte-identical to before — the tool array only grows when the
operator's config actually yields servers). Zero client changes: MCP
tools render as normal tool calls.

**Posture (the load-bearing invariants):**

- **External candidate input.** MCP tools are EXTERNAL CANDIDATE
  INPUT under the FULL approval/guard waterfall — by construction:
  they register as ordinary `ToolDefinition`s, so the guard layer,
  the allow/deny/ask waterfall, the approval bridge, and the P3
  policy classes apply to every `mcp_*` call exactly as to
  `run_shell`. No MCP result is trusted authority (results are
  logged tool content, nothing more).
- **Policy default = ASK.** MCP tool names match no shipped allow
  rule; under a client `--policy` (or interactive) an MCP call ASKS
  and falls to the human responder. Operators allow explicitly by
  tool name (e.g. `allow mcp_vhmcp_web_search`).
- **Credentials redacted like provider keys.** URL (path-embedded
  tokens included), header values, and env values never reach a log
  line (startup lines carry names, transport kinds, and counts only)
  and are scrubbed from every error surface via the same
  `adapters.RedactSecret` discipline.
- **Fail-closed, never hung.** Every exchange (initialize,
  tools/list, tools/call) is bounded by `--mcp-timeout-ms` (default
  60000; validated 0<ms≤600000, the run_shell cap). A server that
  will not start, times out, or returns garbage DEGRADES: it
  contributes no tools, its `mcp_<server>` namespace is RESERVED as
  one sentinel tool whose call returns the typed "server degraded"
  error (the advertised set stays stable; a hallucinated full tool
  name still reports unknown), surfaced at startup and at call time.
  An unmappable tool schema (non-object `inputSchema`) skips THAT
  tool with a startup warning — the server stays up (fail-closed
  per-tool, never per-daemon).
- **v1 scope (honest).** Tools only — no resources, prompts,
  sampling, or elicitation; no OAuth (static headers/env); servers
  launch at startup with no mid-life supervision (a dead server
  stays degraded until a daemon restart — documented, deliberate);
  the tool list refreshes at startup only (a `Refresh` seam exists
  in `internal/mcp`, unwired at the daemon).

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

P6 extends the contract with a PROGRESSIVE OUTPUT channel (§4g):
jobs whose executor produces output stream it into a bounded in-memory
retention buffer, readable mid-flight through `jobs/output` with exact
offset-cursor semantics. The model-facing entry is `run_shell`
`background:true` — the tool result is the enqueue receipt and the
settlement notice (`job/report`) carries the exit facts in `detail`;
the OUTPUT itself is host-side (the wire read), not model-facing.
Captured output is non-durable across restart (§4g posture).

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
- **P6 decision (jobs/output + background run_shell): ProtocolVersion
  stays 1.** `jobs/output` is a NEW method name — additive under the
  rule above; no field was added to any existing method's params or
  result (its shape is locked by its own fixtures). The additive LOG
  payloads (`job/settled`/`job/report` gain an omitempty `detail`;
  surface derivation appends it only when present) keep pre-P6 logs
  replaying byte-identically, and old fixtures were verified
  byte-stable. The `run_shell` tool SCHEMA gained the `background`
  arg — a tool-adapter surface, not a wire-method shape; tool schemas
  are advertised per request and consumed per provider.

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

## 11. Deploying and running the daemon

Operational findings that the offline self-test battery cannot surface
(its image talks only to the local mock over plain HTTP):

- **Runtime images need `ca-certificates` for real HTTPS providers.**
  Bake the system CA bundle into the image (`apt-get install
  ca-certificates` at IMAGE build time — never ad-hoc in a running
  container). Without it every real provider call over `https://`
  fails TLS verification ("unknown authority"). A battery-green image
  is not proof that real providers work.
- **Under Docker, the kernel sandbox needs `--security-opt
  seccomp=unconfined`.** Docker's DEFAULT seccomp profile blocks the
  Landlock syscalls, so `--sandbox read-only|workspace-write` calls
  fail closed with typed sandbox-unavailable errors inside a
  default-profile container. Run the daemon container with
  `--security-opt seccomp=unconfined` (the same rationale as
  `docker/selftest/run.sh`): the flag lifts only Docker's profile —
  the engine's own trampoline (Landlock + seccomp) then enforces the
  confinement (see README.agent.md → the daemon's deployment notes).
