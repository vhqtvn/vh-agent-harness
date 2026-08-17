Durable home: researches/sources/deepseek-harness/client-runtime.md (landed 2026-08-17).

# DeepSeek Harness (dsh) — Client Runtime Source Packet

## 1. Provenance

- **Repo**: local checkout `refs/deepseek-harness` → upstream `deepseek-ai/deepseek-harness`
- **Commit**: `47f943859bef60e4160492346772ded9b24f765a` (2026-08-13)
- **Slice**: `packages/client/` — the ENTIRE client tree (1,140 files excl. `node_modules`/`dist`, verified via `find … | wc -l`). This is the browser/UI-side runtime "face" per the host/client `DSH_BUILD_FACE` split.
- **What dsh is**: a Cordis-based everything-is-a-plugin agent harness (pnpm monorepo). Cordis (Koishi's DI/container system) provides the plugin lifecycle; every UI feature ships as a plugin registering into slots.
- **Audience**: maintainers of vh-agent-harness (Go repo-resident agent harness, OpenCode-first TUI/Web UX) mining dsh's client runtime for rich-client harness design ideas.
- **Method**: static reading only. Phase 1 = orientation (all package READMEs, `packages/client/AGENTS.md`, root `scripts/verify-client-domain-graph.ts` + `scripts/gen-client-catalog.ts`, connection wire contract, per-package role inventory). Phase 2 = full reads of the 11 load-bearing modules in §4. Line counts of every cited file were re-verified against the tree at transcription time (all matched).
- **Packet status**: transcribed from a completed-but-unwritten evidence base (prior pass hit step budget before writing). Top-up reads folded in where §8 says so.

## 2. Client architecture orientation

### 2.1 Faces and the build-face split

The monorepo builds two faces of the same package graph (`DSH_BUILD_FACE`): the **host** face (node/server runtime — agent loop, tools, persistence) and the **client** face (browser runtime — `packages/client/`, this slice). The client is a thick, stateful runtime, NOT a thin view layer: it owns session state machines, an incremental conversation derivation pipeline, per-key seq-ranked projection stores, and transient mirrors of host state — all rebuilt from the wire on reconnect.

Within `packages/client/`, the layering (enforced, see §2.2) separates:

- **Wire/kernel**: `connection` (transport), `modules` (dual-face module system), `hmr` (dev-only reload), `web` (shell kernel/boot), `web-react` (React glue).
- **Core services**: `runtime` (SlotRegistry, SessionRuntime: scope tree + object layer) — the load-bearing heart, deep-read in §4.
- **Pure cores**: `ui-slots` (slot registry pure core), `ui-primitives` / `ui-attachment` (zero-cordis React atoms), `schema-form` (settings draft model).
- **Feature plugins**: the `ui-*` family, each registering into slots rather than owning layout.

### 2.2 Governing rules — `packages/client/AGENTS.md` (full text verified this session)

The client tree has its own AGENTS.md ("Web client stack"). Key rules (now verified against the full text, obtained via environment injection when reading under `packages/client/`):

- **Slot and props discipline** — ONE composition API: `ctx.slots.register({ name, children?, store?, inject? }, Component)`; no separate slot-definition call, no face-minting helper; the shell alone renders `'root'`. `children` = declaration + authorization: the slots a component may render are exactly the keys of its register call's `children` table (spec `kind`/`scope` per key); rendering an undeclared slot or double-declaring one **fails at load** — "the conflict is the design speaking". Slot names mirror the composition path: `<domain>.<entry>.<hole>` (e.g. `'tool.call.toolview'`).
- **Four props shares, all derived**: `PropsRuntime<K>` (SlotMap owner params + `useSession`/`sessionId` on session scope + global `useSessions`/`useWorkspaces`) & `PropsRenderSlots<S>` (children keys) & `PropsStore<H>` (store seat) & the inject face. Never hand-write a member a share already derives. (SlotCore's `ComposedProps` additionally composes the locale `t` seat and the chain `matched` share — see §4.12.)
- **Five standing hooks**: `useSession`, `useSessions`, `useWorkspaces`, `useStore`, `renderSlot`, plus renderer-bound `use<Name>` from provide contributions and `hooks` compartments. Business code never mints hooks/selectors as prop values.
- **Live data has exactly three channels**: parent knows it → owner props at the renderSlot site; only the component knows it → local state; shared across entries or surviving remounts → a store declared at register. Derived data is `useMemo` over framework-hook data, never its own subscription.
- **Stores: read `props.useStore`, write `props.actions.*`** — declared actions are the complete mutation API; stores are exported `createXXXStore()` factories (module-level handles forbidden — no de-facto singletons); production never calls the factory outside `apply` (tests may).
- **Reactive-read discipline**: everything a render reads that can change outside React arrives through a framework hook; business components contain NO subscription machinery (no `useSyncExternalStore`, no snapshot mirroring); an observable source keeps two identities stable (the source object, and the snapshot between changes); whoever rebuilds a published value republishes through the same source in the same step.
- **Layering red lines** (one-way knowledge, three layers): (1) data object layer (`runtime`, React-free — zero React imports, grep-assertable): ConnectionController → SessionManager → Session own all business state + the snapshot-store engine; (2) render machinery (`web-react`, shell-only glue): all ctx-to-React integration; (3) presentation components (plugin `src/client/`, pure props). Non-negotiables: **business data lives in the object layer, never a store** (entry-declared stores carry only viewing/interaction state); **rpcId strictly bidirectional** (initiator mints, responder echoes; business signatures see only `RpcRequest<P>`); **notifier publication discipline**: `notifyNow` ONLY the direct echo of a user gesture, structural updates microtask-batched `markDirty`, streaming chunks cumulative `markFrameDirty`; **the web layer is pure presentation** — nothing "how to draw" enters the session log, the host computes such data per frame or pushes it live, replay recomputes with generic fallback; a new model-visible input requires a session event (repo-wide "model-visible ⟺ logged").
- **Conversation Node discipline**: a Chat feature registers one `ConversationNodeDefinition` + its keyed `conversation.chat.node` renderer — never an event switch on/fold into `Session`/`SessionManager`/a central dispatcher. `match(event)` reads only the current event; every event in a multi-event Context carries or independently derives the same stable business id; `update` folds one Match into State and stays deterministically replayable by log `seq`; the append hot path and renderers never scan the full event window — accumulate in State, publish through `buildLocationData()`.
- **Directory regime**: one UI feature = one plugin package; multi-domain packages split into `contract/` (only shared API) + domain dirs that never import siblings + `apply.ts` as single assembly point (`ui-conversation` is the example); `scripts/verify-client-domain-graph.ts` enforces the levels.
- **Export/ctx discipline**: `/client` entrypoint exports only cordis-loading needs (`apply`/`inject`/`Config`) + type-only store factories; cross-package imports of another plugin's symbols forbidden (slot system or ctx services only); components never see `ctx` — everything arrives through the four shares.
- **Testing**: client source packages sit inside the per-file **100% coverage gate** (`pnpm run test:coverage`); unreachable defensive arms need a justified `/* v8 ignore -- <reason> */`; component specs assert user-visible behavior, not class names/hook internals; check ladder: `pnpm run test:gui` (inner loop, seconds) → `DSH_SNAPSHOT=replay pnpm run test:web` for visible-output changes (keyless replayed e2e) → pre-push skill picks narrow checks per diff.
- **New plugin package checklist**: three registration surfaces all required (tsconfig aggregate `references`, `dsh.client` row in `cordis.patch.yml`, bundle `package.json` dependency) — missing any one fails at a different later point; `dsh.client` manifest: `immediately: true` ONLY for stage-one-prefetch infrastructure rows, `inject` edges are informational (activation order is cordis fiber inject waiting on services, nothing else); registering into another package's slot uses `ctx.slots.inject(name, () => ctx.slots.register(...))` — waits on the actual declaration, removes the contribution when that declaration collapses, reruns after redeclaration, leaves with the caller's fiber; a generator yields each registration when several must install/roll back atomically.
- **Styling**: `--dsw-*` tokens + CSS Modules + `clsx`; no literal colors, no component library, no Tailwind; product copy Chinese, code comments English.

### 2.3 Domain-graph verification and catalog generation

- Root `scripts/verify-client-domain-graph.ts` — verifies the client dependency/domain graph (layering rules as a checkable artifact, run in CI-style verification rather than prose-only).
- Root `scripts/gen-client-catalog.ts` — generates the client catalog from the package graph (keeps docs/inventory in sync with the actual tree; the inventory in §3 matches its output domain).

### 2.4 Wire contract with the host (`connection` package)

Package description: "Wire consumer layer: HTTP-up/WebSocket-down client, ConnectionController dual streams with reconnect, and fixture api". Files read (Phase 1, small files — effectively full):

- `connection/src/rpc.ts` (63 ln) — RPC wire envelope shape.
- `connection/src/client/connection.ts` (202 ln) — ConnectionController: dual upstream/downstream streams with reconnect; exposes connection state ('reconnecting' etc.) consumed by the runtime (§4.11).
- `connection/src/client/api.ts` (54 ln) — API surface; a fixture api exists for tests/dev.
- Also present (not deep-read): `api-path.ts`, `api-request-trust.ts`, `http-bridge.ts`, `invariant.ts`, `loopback-hostname.ts`, `rpc-host.ts`, `websocket-downlink.ts`.

Dispatch semantics (as exercised in `session.ts` / `index.ts`, §4): the client receives muxed envelopes; `handleMuxEnvelope` routes `session/event`, `session/queue`, `session/subscribed`, `approval/*`, `question/*` frames; `host/remote-event` frames go to `ctx.remote.$dispatch` with `$on` fan-out (Remote events as a cordis service). HTTP carries requests up; WebSocket carries the downlink (events/frames). A `'connection/reset'` event fires on `onConnected`; `handleDisconnected` runs when the controller enters `'reconnecting'` — generation death is the only safe moment to drop generation-scoped state (§4.11).

## 3. Domain/module inventory — all 39 packages

39 package.json-owning directories (verified: `find refs/deepseek-harness/packages/client -name package.json -not -path '*/node_modules/*' | wc -l` → 39; the task brief's "40" is count drift, see §7). Roles below are the packages' own `package.json` descriptions (re-verified at transcription time), grouped by domain.

### Wire & kernel (8)

| Package | Role (package.json description) |
|---|---|
| `connection` | Wire consumer layer: HTTP-up/WebSocket-down client, ConnectionController dual streams with reconnect, fixture api |
| `modules` | Client module system, dual-face: node half composes the `__DSH_BOOT__` entry graph (incremental `dsh.client` scan, bundle route, index tap, webPlugins service); browser half is the lazy-CJS module table the vendored cordis Loader consumes as its internal seam |
| `hmr` | Dev-only hot-reload driver for script-loaded client entries: SSE rebuilt frames → invalidate/prefetch → fiber swap through the vendored Loader entry |
| `web` | Web shell kernel: `bootWebShell` (module system holding + seed table + two-stage boot + AppRoot gate + app-shell assembly entry), consumed by the `apps/web` vite entry |
| `web-react` | Shell-side React glue: `createSlotRenderer`, `SessionProvider`, `bindSnapshotSelector` (uSES bridge), `useInvoke` |
| `runtime` | Client core services: SlotRegistry, SessionRuntime (scope tree + object layer) — §4 deep dives live here |
| `locale` | Locale plugin: Host-backed zh/en preference, browser-derived fallback, locale snapshots, typed namespace dictionaries |
| `schema-form` | Schema/draft model layer for settings editors: rehydrates a serialized schemastery schema, validates drafts, edits immutably by path |

### Shell & chrome (4)

| Package | Role |
|---|---|
| `ui-layout` | Shell plugin: three-column AppFrame with drag handles, `ctx.layout` viewing-state service (navigation + panels) |
| `ui-sidebar` | Session multi-level tree, search, grouping, state dots |
| `ui-theme` | Host bootstrap for pre-plugin palette; DOM-free ThemeRuntime (light/dark/system); `--dsw-*` token styles; Appearance settings row |
| `ui-workspace` | Workspace picker: one WorkspacePicker into the sidebar + empty-state workspace slots |

### Conversation surface (7)

| Package | Role |
|---|---|
| `ui-conversation` | Conversation domain: skeleton, ordered chat flow, composer with Host-backed busy-Enter preference, details host |
| `ui-tool` | Tool call-tree renderer + keyed per-tool presentation slot |
| `ui-trajectory` | Trajectory event ledger with interactive timing overview; pure-consumer plugin registering into the conversation ViewMap (no service) |
| `ui-deliverables` | Produced-files turn tail + clickable final-response file references |
| `ui-attachment` | Pure React attachment atoms: draft-image rail, message image gallery, original-image lightbox (zero cordis) |
| `ui-skill` | Web skill references + dedicated skill tool row |
| `ui-workflow-run` | Durable workflow-run Conversation Node + nested member disclosure |

### Composer & input control (7)

| Package | Role |
|---|---|
| `ui-input-trigger` | '/' and '@' detection, candidate menu, pick routing to registered sources |
| `ui-commands` | Global directory cache, '/' source, three command UI kinds, popupSelect registry |
| `ui-goal` | GoalBar docked above the composer, read from the goal session projection |
| `ui-plan` | Plan-mode composer control: `conversation.input.plan` seat over the plan projection + `/plan` command channel |
| `ui-model-selection` | `/model` popupSelect over `session.models` / `session.selectModel` |
| `ui-agent-preset` | Agent-preset surfaces: default for later sessions, this session's seat, composition editor |
| `ui-permission-presets` | New-session default in General settings + current-session `/permission` popup over the permissions projection |

### Interaction & agents (2)

| Package | Role |
|---|---|
| `ui-user-questions` | `ask_user_question` feature: host tool mount + composer-takeover question UI |
| `ui-subagent` | Subagent conversation catalog, continuation routing UI, '@' reference source |

### Settings family (5)

| Package | Role |
|---|---|
| `ui-settings` | Settings domain base plugin: settings-namespace scope service + canonical settings slot-type contract |
| `ui-settings-general` | General section, shell trigger/header chrome content, settings dictionaries, versioned welcome notice (ownerless-copy + onboarding) |
| `ui-settings-models` | Models settings + shared onboarding dialogs over settings/credential joins |
| `ui-settings-plugins` | Plugins settings section: feature-owned tabs + configurable host-plane plugin cards |
| `ui-settings-plugin-inventory` | Read-only Cordis Loader inventory tab in Web Plugins settings |

### Pure cores & misc (6)

| Package | Role |
|---|---|
| `ui-slots` | Slot registry pure core: SlotMap declaration merging, single-register composition API, four-share props types, store-seat types, renderer install seam (1,192 ln `src/index.ts` — full read, §4.12) |
| `ui-primitives` | Pure React atoms: controls, icons, markdown, JSON inspectors (zero cordis) |
| `ui-jobs` | Session-header background-job list: live registry state mirrored from `session/jobs` frames |
| `ui-message-feedback` | Per-message feedback controls in the assistant-message action strip, backed by the messageFeedback Host Remote |
| `ui-directory-picker-browse` | In-app directory browsing: workspace directory-flow owner rendering host listing/creation primitives |
| `ui-directory-picker-native` | Renderless directory-picker: workspace directory-flow occupant driving the host's OS chooser |

**Total: 39.** Runtime-internal sub-domains (inside `runtime/src/client/`, not separate packages): `sessions/` (22 files — the §4 core), `conversation/` (definition/event/view registries), `contract/` (typed wire contract: `conversation.ts`, `sessions.ts`, `session.ts`, `sessions-port.ts`, `settings-scope.ts`, `store.ts`, `workspaces.ts`), `agents/scope.ts`, `workspaces/` (manager, path, service, workspace), plus `slots.ts`, `index.ts`, `ordered-baseline.ts`, `time-zone.ts`.

## 4. Deep dives — load-bearing modules

All under `packages/client/runtime/src/client/`; `sessions/*` unless noted. Line counts re-verified.

### 4.1 `sessions/session.ts` (805 ln) — Session lifecycle & event ingestion

- **Dual arrays**: Session owns `events` + `views` parallel arrays — the RAW event log stays model-visible next to derived views (derivation is auditable against its input; nothing is destroyed on derive).
- **Open lifecycle**: `OpenState` cold → loading → open → error; `openGeneration` counters invalidate stale async opens (a late reply from a superseded open cannot clobber current state).
- **Live stitching & gap repair**: `liveBuffer` stitches streaming deltas; gap repair runs resync-lite — NO loading flash (repair fetches the missing range and splices, instead of resetting the view to a skeleton).
- **Paging**: `PAGE_MESSAGES = 50`.
- **Prompt semantics**: `prompt()` sets `promptAttempted` synchronously BEFORE awaiting (UI can render "engaging" immediately, crash-safely). The blank→content flip happens only on an ACCEPTED first prompt; a REJECTED first prompt keeps the session reusable (no zombie blank state).
- **Subagent routing**: one-shot read-only for non-continuable subagents; image attachments unsupported for continuations; `subagents.interrupt` for continuable ones.
- **Rename**: settles the `'title'` projection cell directly; a later push frame for it is a no-op replay (seq-rank absorbs it) — no flicker.
- **`handleMuxEnvelope` dispatch**: `session/event`, `session/queue`, `session/subscribed` (clears queue mirror race-free), approval/question `requested` → mint `PendingWait` / `resolved` → settle it.
- **Phase derivation**: `composerPhase` ∈ blank|engaging|active derived at exactly ONE site (`derivePhase`) — no other code path computes UI phase.

### 4.2 `sessions/conversation-assembler.ts` (808 ln) — derivation pipeline

- **`InternalContext`** carries `{matches, state, revision, dependencies}` — derivations run against a revision-tagged context, not naked state.
- **Match protocol**: match → accept-or-replay; `start-Match` uniqueness + seq-append invariants THROW on violation (invariant failures are loud, not silent).
- **`mergeMatches`** for prepend (loading older history merges into the front).
- **`ConversationLocationIndex`**: boundaries at turn/start|end and step/start|end — stable addresses for scroll/anchor/navigation.
- **Dependency tracking**: `ConversationContextReader.previous(kind)` with revision/windowGap keys → dependent derivations replay as a cascade when their inputs change.
- **`flush()`**: replace mode (full swap) vs incremental upserts — chosen per flush, not globally.
- **Publication ranks**: none < animation-frame < immediate (consumers choose urgency; see notifier §4.3).
- **`buildLocationData` validation**: kind/key/turn/step safe-integer checks (bad location data is rejected at construction, not at render).
- **"Withdrawn materialized target" rule**: THROWS if a withdrawal targets something already materialized — the rule is keep-the-key-and-hide, never delete a materialized node out from under live references.

### 4.3 `sessions/notifier.ts` (103 ln) — notification scheduling tiers

- `markDirty` = microtask; `markFrameDirty` = RAF-cumulative (one RAF batches many marks); `notifyNow` = same-tick.
- **Controlled-input caret rule**: same-tick `notifyNow` exists so controlled inputs don't lose caret position to a deferred render.
- **`ensureFresh`** rebuilds WITHOUT notifying — freshness and notification are separate bits (you can make state consistent silently).
- Lazy rebuild when no listeners (no wasted derivation).

### 4.4 `sessions/pending.ts` (82 ln) — PendingWait (approvals/questions)

- `PendingWait` = kind + key + payload + private rpcId; key is `` `<p>:<rpcId>` `` — a STABLE React key (list reconciliation survives re-render).
- `respond()` backfills rpcId into the client-response envelope; throws if called after settlement (double-respond is loud).
- Settlement = pending-map membership ONLY (no separate settled flag to drift).

### 4.5 `sessions/projection-store.ts` (201 ln) — per-key seq-ranked projections

- Shape: key → `{value, seq}`; **higher-seq-wins is the single conflict rule** (last-writer per key by sequence, no merges).
- `seed()` clears absent keys only when not superseded (a seed cannot erase a newer live value).
- **`truncate(lastSeq)` on `session/subscribed`**: drops rows past the host's durable baseline — restart-lost transient state can't pin phantom values in the UI after a resubscribe.
- Identity-stable per-key faces + `subscribeAny` coarse channel (fine-grained per-key subscriptions or a global "anything changed" channel).

### 4.6 `sessions/queue-mirror.ts` (74 ln) — transient queued prompts

- Transient snapshot of the host's queue (not durable state); preview = first 200 chars.
- `acceptDurable` retires a steering row when the matching user/message lands (`messageId` + placement match) — optimistic queue rows are reconciled against durable events, not trusted forever.

### 4.7 `sessions/manager.ts` (1,131 ln) — SessionManager

- `Map<SessionId, Session>` resident instances.
- **`pendingBuffers`**: pre-instantiation approval/question/queue frames are buffered identity-compacted and replayed at `get()` — an approval arriving before the session object exists is not lost.
- `pendingInteractions` map feeds sidebar dots; cleared per connection generation in `handleDisconnected`.
- **`completedNotifications`**: the running→idle EDGE of a NON-SELECTED session arms a green dot; eager reconciliation after EVERY mutation (edge detection that can't drift from missed events).
- **Projection stores outlive instantiation** — the title-snapshot precedent generalized: keyed projections (title etc.) exist before a Session object is created and survive its teardown.
- `listMutations` replay over an in-flight `refreshList` (local list edits are not clobbered by a racing refresh).
- `create()`/`fork()` reconcile workspace-attach-failed partial success by publishing the REAL sessionId as Ungrouped (a session is never orphaned invisible because its grouping failed).
- `questionInteractionStatus` = plan-review binary routing mirrored at the wire boundary.
- **`entryCache` identity recovery**: wire refreshes mint all-new objects → a memo/WeakMap would miss; the cache re-binds identity so downstream identity-dependent code keeps working.
- **`host/session-removed`**: durable subagent → `status: idle` (Activation detach ≠ deletion) vs ordinary session → remove + clear `pendingBuffers`/`pendingInteractions`/`projectionStores`. The durable/subagent distinction is enforced at removal time.
- Catalog refresh: single-flight + 50 ms debounce + in-flight mutation folding (`expandableRows`/`activityRows`) + `parentAvailableOverride` replay.

### 4.8 `sessions/tool-call-tree.ts` (200 ln)

- Code-Dispatch parent index; `MAX_TOOL_CALL_TREE_DEPTH = 256`.
- Cycle + depth guards in `acceptEdge` (a malformed call graph cannot loop or blow the stack).
- **Revision-cached structural sharing**: `projectNodes`/`projectRunningCalls` return the SAME reference when unchanged — React memoization works for free downstream.

### 4.9 `sessions/conversation.ts` (477 ln) — ConversationNode union & snapshots

Complete ConversationNode union:

- `user`
- `assistant` — `messageId` ABSENT on interruption-frozen partials + fractional synthetic seq (a partial assistant turn is addressable without a server id)
- `steering`
- `context` — carries provenance + form
- `model-retry` — client-derived `retryState`
- `turn-error`, `turn-max-tokens`
- `tool-result` — `callView`/`resultView` HOST render intents + `subCalls`
- `command` run/done — paired by `commandId`, with window-cut soft-fall (a `done` whose `run` fell outside the loaded window degrades gracefully, not an error)
- compaction marker — shadowed counts + never-renders framed payload
- unknown-surface fallback (forward compatibility: unrecognized kinds render, not crash)

**`ChatSnapshot`** = order + stable live `ChatNodeStore`/`ChatLocationNodeIndex` + legacy slice. **`ConversationSnapshot`** full shape includes queue, subagent address + `parentAvailable`, `composerPhase`, blank.

### 4.10 `slots.ts` (471 ln) — SlotRegistry

- `SlotRegistry` = cordis Service over `SlotCore` (the pure core in `ui-slots`, §3).
- **register must be a PROTOTYPE method**: caller-context binding routes disposal to the caller's fiber (plugin unload disposes exactly the caller's registrations — Cordis lifecycle integration).
- `inject()` = declaration-lifetime nested effect: epoch-reconciled; a failed injection RETIRES the declaration permanently; generator = transactional.
- `install()`/`installLocale()` = boot-once (idempotent install seams).
- `renderSlot` only for `'root'` + 3 fail-loud guards.
- **Store instance axis**: handle × scopeKey → create/cache; `pruneStoreScope` clears persisted per-session state (transient materialization exists JUST to clear storage).
- Root SlotMap DO-NOT-REGISTER doc: a dynamic root entry would SHADOW AppFrame (the composition root is protected by convention + docs).

### 4.11 `runtime/src/client/index.ts` (233 ln) — runtime wiring

- `apply()` wiring = SlotRegistry + ConversationEvent/View Registries + SessionRuntime + typert agent-scope registration + WorkspaceRuntime + `connection.start` loop.
- `host/remote-event` frames → `ctx.remote.$dispatch` (`$on` fan-out) — host-pushed Remote events as a service bus.
- `'connection/reset'` event fired on `onConnected`.
- `handleDisconnected` on the `'reconnecting'` state: **generation death is the only safe moment to drop generation-scoped state** (anything dropped earlier races in-flight frames).

### 4.12 `ui-slots/src/index.ts` (1,192 ln) — SlotCore, the slot registry pure core (top-up read)

Pure core: zero runtime dependencies (React types only); no cordis — event emission and renderer-install contract live in the runtime Service wrapper (§4.10). `SlotMap` and `LocaleNamespaceMap` are declare-merge tables living in the entry module (lexical-merge requirement — consumers' `declare module` merges must hit the augmented module, not re-exports).

- **Type system**: `SlotKind` = single | list | keyed | chain; `SlotScope` = root | session-maybe | session. One `SlotEntryDef` per key: kind/scope axes + optional owner props, `keyProps` (keyed per-key prop share), `hookContext` (opaque per-render-occurrence context), slot-level `inject` face.
- **`register(options, component)`** — the ONE composition API. Load-time validation throws: registering into an undeclared slot; re-declaring an already-declared child key (one declarer per slot; error names the first declarer); mounting one shared store handle under slots of different scopes ("one handle, one scope"); kind constraints (keyed requires `key`, list requires `id`, chain requires `select`).
- **Shadowing semantics** (single/keyed/list): a *cell* is the slot itself / a `key` / an `id`. Entries in one cell coexist at distinct priorities, sorted ascending, ties keep registration order; **the cell's lowest live entry renders**. A second registration at an occupied cell's exact priority (default 0) THROWS naming the occupant — priority-less composition keeps historical one-occupant-per-cell fail-loud; shadowing is the explicit escape.
- **Chain semantics**: each entry carries a pure `select: (owner) => M | null` routing selector (MUST be pure — function of owner props only; the decline decision lives in the selector, never in a mounted component probing its props). Selectors run in chain order (ascending priority, default 0, lower tries first); first non-null elects its entry and the result becomes the component's framework-injected `matched` prop; all-null renders the owner's fallback. `ChainRenderOpts.overlay` keeps the fallback permanently mounted (an election hides it via `display:none` instead of unmounting) so **fallback-held state (composer drafts, DOM state) survives a takeover** — sole consumer today: the `'conversation.composer'` chain.
- **`ComposedProps`** = `PropsRuntime` & `PropsRenderSlots` & `PropsStore` & `InjectFace` & `MatchedShare` & `PropsLocale` — every share derives from its single source of truth; components reference the composition, never re-type it. The reserved `hooks` compartment of an inject face arrives component-side as synthesized `use<Name>` selector hooks bound to each bare observable's snapshot type; all other members pass through verbatim.
- **Compile-time enforcement**: `RendersCheck` — an entry declaring `children` whose component consumes no `renderSlot`/`renderSlotChain` is a TYPE error ("declaring is claiming"); the `__renders` phantom contravariance anchor enforces component key set ⊆ children declaration at the register call site.
- **Change propagation contract**: `getVersion` bumps + `onMutate` fire SYNCHRONOUSLY per mutation (registry consistent when they fire); `subscribe` notifications batch per MICROTASK (N same-tick mutations → one notification per touched key); `subscribeDeclaration` fires synchronously per declaration-lifetime boundary; a children table commits every sibling declaration before its first notification (synchronous listeners may register into or redeclare a sibling).
- **'root'** is the one a-priori declaration (single/root, `declaredBy: '(built-in)'`) — the render tree's root hole, matching §4.10's DO-NOT-REGISTER protection (type-level: `root` is not in any consumer's `SlotMap` merge, so typed callers cannot name it).
- **Records are never removed**; `declarationEpoch` is monotonic across redeclarations (entry add/remove does not move it; creation and collapse each advance it) — the epoch `slots.inject()` reconciles against (§4.10).
- **Crash containment / abdication**: `reportEntryError(key, entry, error, {abdicate})` — for shadowing kinds (single/keyed/list) an abdicating report RETIRES the entry from its cell one-shot (`WeakSet`), the version bumps through the ordinary mutation channel so outlets re-project onto the cell's next survivor, and a repeat abdicating report no-ops. Chain crashes report `abdicate: false` (election alternatives resolve at select time). The registration itself stays on the ledger — disposal authority remains with the registrant; `onEntryError` is the supervision seam for hosts mirroring contribution health.
- **Lifecycle cascade**: the register disposer removes the contribution AND collapses every child slot it declared (specs clear, child entries empty recursively, stale disposers become no-ops) — ledger rows, slots, contributions, and store mounts die together on one lifecycle axis. Shared-handle scope pins release when the mount count hits 0.
- **Read surfaces**: `entries(key)` returns the cached array reference — stable between mutations, uSES-`getSnapshot`-safe, empty for undeclared keys (renderers may probe ahead of plugin load order); `entriesOfSlot(key)` projects shadowing winners per cell (fresh array per call — a render-body read, NOT a getSnapshot source); `isLive(entry)` is the stale-authorization probe (a retained `renderSlot` binding whose entry left the ledger must not render); `snapshot(root?)` exports the live declaration topology as JSON-safe `LiveSlotNode[]` (occupants, priorities, active) — the diagnostics surface.
- **Locale integration**: `SlotLabel` = string | `() => string` thunk re-evaluated per read so registration-time text follows the active locale without re-registration; a `locale:` namespace declaration synthesizes the typed `t` seat on props (keys compile-checked against the namespace dictionary union).
- **flush()**: resets the scheduled flag BEFORE iterating so a mutation from inside a listener re-schedules — no lost notification.

### 4.13 `web/src/boot.tsx` (238 ln) + `web/src/app-shell.ts` (50 ln) — two-stage boot (top-up read)

`AppWebEntry.run()` — module face first, then plugin face:

1. Parse `window.__DSH_BOOT__` into the two-view `BootManifest` (wire boundary).
2. Build the `ClientModuleSystem` over the module-view rows (seed statics + the app-shell assembly as the only shell-own static module).
3. **Adoption handoff for the modules package itself** (bootstrap identity: the module system cannot arrive through itself): its client half is shell-bundled, statically registered under its bare package name (= graph row id = entry name), and the instance goes on the `__DSH_MODULES__` kernel slot the wrapper's apply reads to provide `ctx.modules`. The plugin-row loop must SKIP that row — the vendored `Group.create` does not dedupe by name and a second fiber would provide `'modules'` twice.
4. Render the loading page (`AppRoot` gated on a `settled` signal + a loader-status store + an error signal).
5. **Prefetch tier**: every `immediately` row prefetches in parallel WITH mounting the vendored cordis Loader; per-row prefetch failures resolve silently (the create-side import reloads and owns the loud failure) — one bad bundle never becomes a boot-wide fail-fast.
6. Inject `loader.internal = modules` BEFORE any entry exists: `tree.import`'s bare-import fallback in a browser is a guaranteed loud failure — "correct as a tripwire, never as a path".
7. Await the prefetch barrier, THEN create entries (concurrently; order carries no semantics — cordis fiber inject waiting owns activation order): the barrier exists because entry materialization runs synchronous cross-package require edges (e.g. locale → runtime/client) that fiber inject waiting cannot protect — a bundle's factory must be registered before any dependent materializes.
8. `loader.await()` + **full fiber sweep** (`assertEntriesActive`): an entry without a fiber failed its import; a fiber not ACTIVE is FAILED (apply threw) or PENDING (a required service never arrived — **cordis inject waiting has no timeout, so this sweep is the fail-loud compensation**, listing who/what/which service).
9. Flip `settled` → AppRoot switches to the real UI in one pass.

- **Shell self-sufficiency rule**: everything in the kernel is machinery that cannot itself be a loader entry, and none of it value-imports a plugin package — "the loading page must work while — especially when — plugins fail". The one sanctioned exception is `modules` (bootstrap identity).
- **Boot failures resolve (not reject)**: the loading page stays up and renders the failure report (the fail-loud surface the kernel owns). Rejects only when the boot manifest is missing/malformed — nothing to boot against.
- **Status projection**: every `internal/status` fiber transition re-projects that entry's row from its ROOT fiber (child plugin fibers share the entry); a failed import leaves the entry fiberless and is manually projected `failed`.
- **Composition lives in the host graph; the shell makes zero composition decisions** — the app-shell assembly is itself a graph entry riding the same entry lifecycle so sweep and status cover it uniformly.
- `app-shell.ts`: pseudo package id `@deepseek-ai/dsh-client-app-shell` exists only in the host graph/shell registry (no npm package). `inject = ['slots', 'sessions', 'layout']` — the inject set guarantees the renderer install lands after the runtime entry is active: `ctx.slots.install(createSlotRenderer())` happens HERE (renderer install is shell territory, but `ctx.slots` exists only once the runtime entry is active). `renderApp` is built once (`??=`) so the closure is identity-stable across AppRoot re-renders, provided via `ctx.reflect.provide('appShell', …)`.

## 5. Findings

No test-derived claims are included in this packet — every finding below comes from full source reads (§4 files), package metadata (§3, re-verified live), or Phase-1 README/rule digest (labeled). Test suites exist (e.g. `modules/tests/`) but were not read; see §8.

- **(finding)** The client is a thick stateful runtime: session state machines, incremental derivation with dependency-tracked replay, seq-ranked projection stores, and transient mirrors all live client-side and rebuild from the wire. source=`sessions/*` full reads (§4.1–4.9), confidence=high, type=fact.
- **(finding)** Conflict resolution across the client is dominated by ONE rule — higher-seq-wins per key (projection-store) plus seq-append invariants that throw (assembler) — giving idempotent replay everywhere: rename push-after-settle, `session/subscribed` truncation, and seed-vs-supersede all reduce to it. source=`projection-store.ts`, `conversation-assembler.ts`, `session.ts` (805/808/201 ln full reads), confidence=high, type=fact.
- **(finding)** Notification scheduling is a deliberate 3-tier system (microtask / RAF-cumulative / same-tick) with freshness decoupled from notification; the same-tick tier exists specifically for controlled-input caret preservation. source=`notifier.ts` (103 ln full read), confidence=high, type=fact.
- **(finding)** Permission/question interactions are first-class runtime objects (PendingWait) with stable React keys, single settlement authority (pending-map membership), and loud double-respond failure; approval/question frames are buffered pre-instantiation and replayed. source=`pending.ts` (82 ln), `manager.ts` (1,131 ln) full reads, confidence=high, type=fact.
- **(finding)** Connection-generation lifecycle is explicit: state is dropped only at 'reconnecting' (generation death); `session/subscribed` truncates projections to the host durable baseline; `connection/reset` fires on reconnect. source=`index.ts` (233 ln), `session.ts`, `projection-store.ts` full reads, confidence=high, type=fact.
- **(finding)** Derived projections use revision-cached structural sharing (same reference when unchanged) and dependency-tracked replay cascades — the React-memo-friendly derivation core. source=`tool-call-tree.ts` (200 ln), `conversation-assembler.ts` (808 ln) full reads, confidence=high, type=fact.
- **(finding)** The ConversationNode union is a closed-but-forward-compatible content model: interruption-frozen partials (no messageId, fractional synthetic seq), host render intents on tool-results, window-cut soft-fall for command pairing, shadowed-count compaction markers, and an unknown-surface fallback node. source=`conversation.ts` (477 ln full read), confidence=high, type=fact.
- **(finding)** Slot composition is lifecycle-integrated: prototype-method registration routes disposal to the caller's fiber; injection is an epoch-reconciled declaration-lifetime effect with permanent retirement on failure; the root slot is protected against dynamic shadowing. source=`slots.ts` (471 ln full read), confidence=high, type=fact.
- **(finding)** Subagents have different removal semantics than ordinary sessions (durable subagent → idle on host/session-removed; detach ≠ deletion), enforced centrally in the manager. source=`manager.ts` full read, confidence=high, type=fact.
- **(finding)** UX affordances are edge-derived and reconciliation-backed: green dots on running→idle of non-selected sessions with eager reconciliation after every mutation; queue steering rows retired by messageId+placement match against durable events. source=`manager.ts`, `queue-mirror.ts` (74 ln) full reads, confidence=high, type=fact.
- **(finding)** Domain-graph layering is machine-verified (root `scripts/verify-client-domain-graph.ts`) and the package catalog generated (`scripts/gen-client-catalog.ts`) — architecture rules as CI artifacts, not prose. source=Phase-1 digest + file presence verified, confidence=medium (semantics from prior pass; not re-line-read this session), type=fact.
- **(finding)** Client-tree governance (slot discipline, four props shares, layering red lines, notifier discipline, Conversation Node discipline, 100% coverage gate, check ladder) is codified in `packages/client/AGENTS.md` — FULL TEXT verified this session (environment-injected on read under `packages/client/`), summarized in §2.2. source=`packages/client/AGENTS.md` full text, confidence=high, type=fact.
- **(finding)** SlotCore is a zero-dependency pure registry with load-time fail-loud validation (undeclared-slot throw, one-declarer-per-slot throw, one-handle-one-scope throw, exact-priority-collision throw), priority-based cell shadowing where the lowest live entry renders, pure-selector chain election with a `matched` prop, synchronous version bumps with microtask-batched subscribe notifications, monotonic declaration epochs, and one-shot abdication crash containment that keeps disposal authority with the registrant. source=`ui-slots/src/index.ts` (1,192 ln full read), confidence=high, type=fact.
- **(finding)** The web boot is a two-stage (module face → plugin face) kernel that stays self-sufficient while plugins fail: silent per-row prefetch failures, an internal-contract injection before any entry exists, a prefetch barrier protecting synchronous cross-package require edges, and a post-`loader.await()` fiber sweep that compensates for cordis's timeout-less inject waiting by failing loud with per-entry missing-service lists. source=`web/src/boot.tsx` (238 ln) + `web/src/app-shell.ts` (50 ln) full reads, confidence=high, type=fact.
- **(finding)** Package inventory = 39 packages with self-describing roles in package.json; file total = exactly 1,140 (excl. node_modules/dist). source=`find` + `grep '"description"'` run this session against the checkout, confidence=high, type=fact.

## 6. Transferable ideas → vh-agent-harness

(Map: dsh client mechanism → vh-agent-harness surface. These are synthesis/judgment grounded in §4/§5 facts — type=inference, not commitments.)

**TUI/Web interaction surfaces (OpenCode-first UX):**

1. **Events+views parallel arrays with the raw log model-visible** (§4.1): any vh rich client (Web face of the harness) can keep derived UI auditable against the raw event log — same philosophy as vh's "model output is a candidate" invariant, applied to client derivations.
2. **Three-tier notification scheduling with freshness≠notification** (§4.3): a TUI redraw scheduler could use the same tiers (immediate for focused-input correctness, frame-batched for bulk updates, lazy when nobody listens) — the controlled-input caret rule has a direct terminal analogue (cursor jump on redraw).
3. **Structural-sharing revision-cached projections** (§4.8): cheap for a Go-driven web client too — identity-stable snapshots let React/TUI-diff memoization work without deep compares.

**Session UX:**

4. **Gap repair without loading flash** (§4.1 resync-lite): on reconnect/window moves, splice the missing range instead of flipping to a skeleton — directly applicable to OpenCode session views over flaky transports.
5. **Green-dot edge notification with eager reconciliation** (§4.7): "session finished while you were elsewhere" as a running→idle EDGE of non-selected sessions, reconciled after every mutation — better than polling or event-trusting.
6. **Projection stores outlive instantiation + pendingBuffers** (§4.7): frames arriving before a session UI object exists are buffered and replayed; title/projection state survives teardown — eliminates a whole class of "event raced the UI" bugs.
7. **Queue mirror with durable retirement** (§4.6): optimistic queued-prompt display retired by messageId+placement match — the pattern for any vh optimistic UI (pending commands, task cards) that must reconcile against durable truth.
8. **Durable-subagent detach≠delete** (§4.7): removing a durable subagent from view sets idle rather than deleting — relevant to vh's subagent/session registries and to `/task-delete` semantics debates (transport vs truth).
9. **Blank→engaging→active phase at ONE derivation site** (§4.1 `derivePhase`; composerPhase in the snapshot): single-site UI phase derivation prevents the classic multi-site "is it loading or empty" bug; `promptAttempted` set synchronously pre-await is the crash-safe version.

**Permission prompting:**

10. **PendingWait as the single interaction object** (§4.4): kind+stable-key+payload+rpcId, respond-backfills-rpcId, settle=map-membership-only, throw-on-double-respond. vh's approval/question flows (permission prompts in TUI/Web) could adopt the same object shape at the wire boundary — including `questionInteractionStatus` binary routing mirrored at the wire (§4.7).
11. **Buffered pre-instantiation approvals** (§4.7): a permission request arriving before the UI session exists must not be dropped — pendingBuffers is the precedent.

**Context visibility:**

12. **Compaction marker with shadowed counts** (§4.9): show THAT context was compacted and how much, never the framed payload — matches vh's drive for honest context accounting in agent UX.
13. **Trajectory ledger as a pure-consumer ViewMap plugin** (§3 `ui-trajectory`): an interactive timing overview registered without owning any service — the cheapest possible way to add execution visibility to an existing conversation surface.
14. **ConversationLocationIndex turn/step boundaries** (§4.2): stable addresses for navigation/anchoring — reusable for vh checkpoints, task cards, or session transcripts.
15. **Machine-verified layering + generated catalog** (§2.3): `verify-client-domain-graph.ts` is the client-side analogue of vh's ownership classification — if vh grows a rich client, its layering rules should be a checkable script from day one.

**Added after top-up reads (SlotCore + boot):**

16. **Cell shadowing with exact-priority collision throw** (§4.12): priority-less composition stays one-occupant-per-cell fail-loud, while explicit priorities give a disciplined override mechanism — a sharper alternative to last-registration-wins or silent merge when vh needs overlay/extension precedence (cf. vh's overlay packs and permission precedence).
17. **Abdication crash containment** (§4.12): a crashing UI contribution retires itself from its cell one-shot, outlets re-project onto the next survivor, and disposal authority stays with the registrant — a model for plugin-contributed TUI/Web surfaces where one bad contributor must not take down the shell (mirrors vh's "model output is a candidate" isolation instinct, applied to UI contributions).
18. **Chain election with overlay-preserved fallback state** (§4.12): selector-routed UI takeover (e.g. question UI seizing the composer) that keeps the fallback mounted hidden so user drafts survive the takeover — directly relevant to OpenCode composer-like surfaces with plugin takeover modes.
19. **Timeout-less inject waiting compensated by a boot-time sweep** (§4.13): dependency waiting without timeouts plus one fail-loud sweep that names each pending entry's missing services — a pattern for vh plugin/runtime startup where silent partial activation is the failure mode to avoid.
20. **Loading-page self-sufficiency + resolve-not-reject boot failures** (§4.13): the boot surface must render the failure report itself; the kernel imports no plugin — applicable to any vh Web face boot and to TUI fallback rendering when the harness loads partially.
21. **`model-visible ⟺ logged` + host-computed render intents** (§2.2, §4.9): presentation data never enters the durable log; the host computes per-frame render intents and replay recomputes them with generic fallback — the clean split for vh session transcripts feeding both TUI and Web faces from one log.
22. **Per-file 100% coverage gate with justified ignores + tiered check ladder** (§2.2): `test:gui` as a seconds-scale inner loop, keyless snapshot replay for visible-output changes — a mature shape for vh's client-side verification if it grows a browser face.

## 7. Contradictions

- **Package count drift**: the task brief says "all 40 packages" / "~40"; the tree has **39** package.json-owning directories under `packages/client/` (verified by find). Immaterial to conclusions; recorded for count hygiene.
- **Root-slot protection, reconciled**: the prior pass recorded "root SlotMap DO-NOT-REGISTER doc (dynamic entry would SHADOW AppFrame)" as a docs/convention-level guard; the SlotCore read shows 'root' IS a declared single-kind slot at the runtime level (a dynamic caller registering at a fresh priority would not throw). The protection is TYPE-level (`root` absent from consumer `SlotMap` merges, so typed callers cannot name it) plus documentation — defense in depth, not a runtime refusal. Not a defect; recorded because the two passes described the guard at different layers.
- No other contradictions detected in the covered range: all line counts and paths cited by the prior evidence base re-verified exactly (805/1131/471/233/808/477/103/82/201/74/200 + connection 63/202/54); file total 1,140 matches the brief's ~1140.

## 8. Coverage

Legend: **examined** = full file read(s) this evidence base; **partial** = named files read, rest README-level; **README-level** = Phase-1 README/package.json digest only (explicitly labeled per task rule).

| Sub-package | Disposition | Reason / evidence |
|---|---|---|
| `runtime` | **examined** | 11 load-bearing files fully read (§4: sessions 8 files, slots.ts, index.ts); `contract/`, `conversation/` registries, `agents/`, `workspaces/`, `ordered-baseline.ts`, `time-zone.ts`, remaining `sessions/*` files (assistant-timing, context-provenance, conversation-context, conversation-location-index, failure-display, lineage, partial, provide, remotes, request-inspection, service, steering-history, subagent-lineage) NOT line-read — README/role-level only |
| `connection` | **partial** | `rpc.ts`, `client/connection.ts`, `client/api.ts` read (Phase 1 wire-contract pass); 7 sibling files (http-bridge, websocket-downlink, rpc-host, api-path, api-request-trust, invariant, loopback-hostname) README-level |
| `web` | **examined (boot path)** | `boot.tsx` (238 ln) + `app-shell.ts` (50 ln) fully read (top-up, §4.13); remaining web/src files (AppRoot.tsx, seed.ts, loader-status.ts, app.tsx, base.css) README/role-level from usage inside the read files |
| `ui-slots` | **examined (core)** | `src/index.ts` (1,192 ln SlotCore) fully read (top-up, §4.12); sibling files (`store.ts`, `renderer.ts`) role-level — re-exported through the read entry, semantics summarized from types/imports |
| `modules` | README-level | Phase-1 digest; `tests/` not read |
| `hmr` | README-level | Phase-1 digest (dev-only surface) |
| `locale` | README-level | Phase-1 digest |
| `schema-form` | README-level | Phase-1 digest |
| `web-react` | README-level | Phase-1 digest |
| `ui-layout`, `ui-sidebar`, `ui-theme`, `ui-workspace` | README-level | Phase-1 digest |
| `ui-conversation`, `ui-tool`, `ui-trajectory`, `ui-deliverables`, `ui-attachment`, `ui-skill`, `ui-workflow-run` | README-level | Phase-1 digest |
| `ui-input-trigger`, `ui-commands`, `ui-goal`, `ui-plan`, `ui-model-selection`, `ui-agent-preset`, `ui-permission-presets` | README-level | Phase-1 digest |
| `ui-user-questions`, `ui-subagent` | README-level | Phase-1 digest |
| `ui-settings`, `ui-settings-general`, `ui-settings-models`, `ui-settings-plugins`, `ui-settings-plugin-inventory` | README-level | Phase-1 digest |
| `ui-primitives`, `ui-jobs`, `ui-message-feedback`, `ui-directory-picker-browse`, `ui-directory-picker-native` | README-level | Phase-1 digest |

Deep-read coverage by volume: 4,439 ln in `runtime` (the 11 §4.1–4.11 files) + 1,480 ln of top-ups (`ui-slots/src/index.ts` 1,192, `web/src/boot.tsx` 238, `web/src/app-shell.ts` 50) = **5,919 ln fully read** across the client core. The derivation/session core, the slot registry core, and the boot path are covered; registries, workspace/agent plumbing, contract TypeScript shapes, and all feature-plugin internals are not.

## 9. Open questions

1. ~~**SlotCore internals** (`ui-slots/src/index.ts`, 1,192 ln)~~ — **RESOLVED by top-up read**: §4.12 (shadowing/chain/abdication/epoch semantics fully transcribed). Sibling `store.ts`/`renderer.ts` internals remain type-level only.
2. ~~**Two-stage boot / AppRoot gate** (`web/src/boot.tsx`, `app-shell.ts`)~~ — **RESOLVED by top-up read**: §4.13 (stages, barrier, sweep, self-sufficiency rule fully transcribed). `AppRoot.tsx`/`seed.ts`/`loader-status.ts` internals unread.
3. Host-side counterparts: how the HOST face publishes `session/subscribed` durable baselines and approval/question envelopes (`packages/host/` — outside this slice).
4. `runtime/src/client/contract/*.ts` wire types — read at name level only; exact envelope TypeScript shapes not transcribed.
5. Remaining `sessions/*` files (lineage, subagent-lineage, steering-history, partial, request-inspection…) — named and role-inferred from usage inside the deep reads, but not independently read.
6. `connection` siblings: trust model (`api-request-trust.ts`), loopback hostname handling, websocket-downlink backpressure.
7. Testing posture of the client runtime: AGENTS.md codifies the 100% per-file gate and check ladder (§2.2), but which specific invariants (seq-append throw, truncate-on-subscribed, abdication) have dedicated unit tests was not verified — test files not read within budget.
8. i18n mechanics (`README.i18n.yaml` per package + locale snapshots) — digest only; the SlotCore locale seat/thunk-label mechanics ARE verified (§4.12).

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the client-runtime gap-fill addendum. Method unchanged: static reading
only, `refs/` never modified, every claim cites a repo-relative path.

### Open-question resolutions

- **OQ 4 (contract wire shapes)** — RESOLVED: all 7
  `runtime/src/client/contract/*.ts` files read (955 ln). These are the
  typed outward service faces (the byte envelope lives in `connection`).
  `session.ts`: prompt errors mirrored into `snapshot.promptError`;
  cancel preserves pending queued work; `ProjectionsFace.faceOf` =
  identity-stable observables (absence = `undefined` snapshot).
  `sessions.ts`: fork boundary = first `turn/end` at-or-after the anchor
  (inside an open turn = unavailable, never clipped backward).
  `sessions-port.ts`: deliberately narrow cross-domain face, satisfied
  structurally; blank sessions REUSED. `conversation.ts`: full
  ConversationNodeDefinition protocol (Engine rejects a second Context
  publishing the same Location key); length-prefixed collision-free keys.
  `store.ts`: NOT types — the store ENGINE (zustand persist "spreads
  state into an object, exploding primitive state", so persistence is
  hand-rolled; `flush:'sync'` default for controlled inputs).
  `settings-scope.ts`: `base`/`user` layers where field PRESENCE marks
  override; only the latest settlement may publish; rejected latest
  writes RELOAD host state. `workspaces.ts`: `archiveSession` hides from
  grouping but "session log and accounting slot remain". 4 of 7 files
  share the thesis "Widening this interface is the explicit act of
  widening what features may do".
- **OQ 5 (remaining sessions/* files)** — RESOLVED: all 13 read (2,144
  ln; directory now 22/22). `service.ts` (791 ln): stage model ("Staging
  IS the open signal"; frozen scope across masked gaps — "tearing down on
  the gap would destroy exactly the frozen scope the mask exists to
  preserve"); persisted selection validated-against-list, never pruned;
  synchronous addressability at create()/fork() resolution ("structural
  rather than an accident of microtask ordering"); fractional fork
  anchors floor to real seqs. `provide.ts`: transactional roster
  (splice-out + re-apply + rethrow). `conversation-location-index.ts`
  (516 ln): owner-checked location data; timeline-level structural
  sharing; changed-seq-scoped replays. `steering-history.ts`: durable
  steering identity from inbox events alone (the machine behind §4.6's
  `acceptDurable`). `partial.ts`: block-level immutability +
  visible-change gating. `request-inspection.ts`: `tools` = "Complete
  tool catalog sent with the request, including tools that were never
  called". `context-provenance.ts`: no-client-table rule. 
  `failure-display.ts`: AUTH → fixed copy (credential-echo guard). Plus
  lineage, subagent-lineage, conversation-context, assistant-timing,
  remotes.
- **OQ 6 (connection siblings)** — RESOLVED: all 10 non-fixture files
  read (976 ln). Trust (`api-request-trust.ts`): Host-header fence on
  EVERY request ("Host is the one header rebinding cannot forge");
  cross-site refused; canonical-entry enforcement (no `0x7f.0.0.1`,
  zero-padded ports, unbracketed IPv6) — "not an auth layer".
  Backpressure: SSE loop awaits `drain` with `close` also resolving
  ("a mid-wait disconnect can't park this loop forever"); 160 MiB body
  bound = per-request resident bound; disconnect detection on
  ServerResponse close; downlink WebSockets one-way (client message →
  `close(1008, 'downlink only')`); slow consumers backpressure the source
  iterator naturally. `rpc-host.ts`: `/api` reserved channel (one
  interceptor); traversal-guarded endpoints; rpcId echoed even into
  `bad-request`. Browser carrier: double-zod-validated frames dropped
  fail-soft; `client/rpc.ts` mints the rpcId and VERIFIES the echo;
  fixture mode via `?fixture`; one-consumer-only `start()`.
  `invariant.ts` justified-empty.
- **OQ 7 (invariant spot-checks)** — RESOLVED: 4 confirmed + 2 bonus
  across 6 spec files. seq-append throw CONFIRMED
  (`session-persistence-jsonl/tests/jsonl.spec.ts:760`; sqlite twin;
  client repull companion). truncate-on-subscribed CONFIRMED
  (`runtime/tests/projection-store.client.spec.ts:159-180`). Slot
  abdication CONFIRMED at the renderer layer
  (`web-react/tests/scoped-slots.client.spec.tsx:330,364,388`) with one
  nuance: the suite mirrors the ledger against a mock host, and the real
  `SlotCore.reportEntryError` one-shot path
  (`ui-slots/src/index.ts:1098-1105`) has no directly-located spec.
  PendingWait double-respond CONFIRMED
  (`runtime/tests/session.client.spec.ts:637-646`). Bonus:
  store-identity pinning (`ui-slots/tests/core.client.spec.ts:190,199,214`);
  stale authorization (`web-react/tests/stale-authorization.client.spec.tsx:100-118`).
- **OQ 8 (i18n mechanics)** — largely resolved:
  `packages/client/locale/src/client/index.ts` fully read (392 ln):
  lookup chain ends at THE KEY ITSELF ("missing text stays visible, fail
  loud in the UI rather than blank"); zh fallback; revision-bumped late
  dictionaries (`locale/change` reserved for actual switches);
  `window`-gated detection (not `navigator`); provisional browser value
  replaced by host preference; feature-owned settings row; bilingual
  balance enforced at registration; `README.i18n.yaml` pairing records
  store git blob hashes per side. Residual: LanguageRow.tsx,
  settings-store.ts, `locales/` dictionaries unread.

### New findings

- `contract/store.ts` is the store ENGINE implementation (identity keys
  instance sharing; storage failures only disable persistence).
- SessionRuntime stage model: frozen read-only scope across masked gaps
  and post-removal; persisted selection validated-against-list.
- Synchronous addressability via explicit `projectList()`; fork anchor
  flooring.
- ConversationLocationIndex: per-key ownership + structural sharing +
  changed-seq replays.
- Steering identity replayable from the log alone (queue-mirror
  optimistic rows retire durably).
- AUTH messages sanitized at the UI projection boundary; raw diagnostic
  stays in the session log.
- Context provenance: no-client-table rule; unreadable shapes degrade,
  never drop.
- Provide channel transactional + shared verbatim between production and
  test bench.
- PartialAccumulator visible-change gating (usage/finish skip
  notification).
- 4-of-7 governance thesis with wire-pump entry points on concrete
  classes.

Cluster D (ui-slots): `ActionsDecl` = the complete auditable write set;
module-level handle exports FORBIDDEN ("a disguised singleton across
plugin reloads") — `StoreFactory` per entry×scope instead;
`clearPersisted()` ("a pruned session must not leave orphaned storage
keys behind"); `StoreInstance` React-free; locale freshness
revision-carried ("a locale switch hands out NEW function references");
`SlotRendererHost` shadowing winners as "a render-body read, not a uSES
getSnapshot source"; one-shot entry retirement ("the registration stays
on the ledger either way"); `sessions.provideInfo` ONE atomic source "so
a stable current id cannot strand mounted entries"; justified-empty
companion (`ui-slots/src/{store,renderer,invariant}.ts`).

Cluster E (web kernel): `AppRoot` = zero-dependency boot gate; failed
boot KEEPS the loading page and lists fiber states ("the fail-loud
presentation must not depend on the system whose failure it reports");
`seed.ts` platform-singleton module table (10 words, `satisfies` pin —
"fails to compile instead of drifting into a runtime require miss");
kernel hand-rolls boot signals ("the loading page has to work while (and
especially when) plugins fail"); `FIBER_STATE` const-enum value mirror;
copy-on-write `LoaderStatusStore.set` for the uSES contract
(`web/src/{AppRoot,seed,loader-status}.ts`).

Cluster F (locale): folded into OQ 8 above (lookup chain,
revision-bumped registration, window-gated detection, provisional
value, feature-owned settings row, stable `bind(ns)`, identity-match
disposers, per-listener containment).

### Transferable ideas added

(packet carries 1-22; new 23-27) — **23** kernel-signal self-sufficiency
(hand-rolled observables + const-enum mirror so the loading page works
when plugins fail); **24** revision-bumped late dictionaries (boot-storm
prevention); **25** presence-marks-override settings semantics; **26**
rpcId echo verification (both directions); **27** drain-await
backpressure (SSE + per-frame awaits).

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| `runtime/src/client/contract/` | 0/7 | 7/7 (955 ln) |
| `runtime/src/client/sessions/` | 9/22 | 22/22 |
| `connection/src/` (non-fixture) | partial | all 10 remaining read (976 ln) |
| `packages/client/ui-slots/src/` | index.ts | + store.ts, renderer.ts, invariant.ts |
| `packages/client/web/src/` kernel | unread | AppRoot, seed, loader-status |
| `packages/client/locale/src/client/` | unread | index.ts full (392 ln) |
| packet-cited invariants vs specs | unspot-checked | 4 confirmed + 2 bonus |

### Contradictions and corrections from this pass

- **rpc.ts citation drift** (cross-round ledger): frozen §2.4 cites
  `connection/src/rpc.ts (63 ln)`, but the pinned tree has root
  `rpc.ts` = **77 ln** (shared host/client channel contracts) and
  `connection/src/client/rpc.ts` = **63 ln** (the browser caller that
  mints/verifies the rpcId). The prior pass cited the client file under
  the root path; both fully read this pass; no conclusion changes.
- **README.i18n.yaml count** (cross-round ledger): the dead session's
  "~40 files" note is stale — the repo-wide glob returns **100+ matches**
  before display truncation (verified at repo root and `docs/i18n/`).
- No other contradictions detected.
