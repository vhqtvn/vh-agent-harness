# DeepSeek Harness (dsh) — Source Packet Index

Seven research source packets about DeepSeek Harness (dsh), mined for
harness-design ideas transferable to vh-agent-harness. This file indexes the
set; each packet is a frozen research record, not active repo guidance.

## Provenance

- **Subject**: local read-only checkout `refs/deepseek-harness/` (never
  modified) of `deepseek-ai/deepseek-harness` ("DeepSeek Harness", dsh) at
  commit `47f943859bef60e4160492346772ded9b24f765a` (2026-08-13, PR #2519),
  version v0.1.0-rc.5.
- **Method**: studied statically on 2026-08-17 — no builds, no execution, no
  network; produced by 7 researcher passes plus per-slice write-up/top-up
  passes (each packet documents its own pass structure). Gap-fill passes ran
  2026-08-17 at the same pinned commit; their open-question resolutions,
  findings, and corrections are appended to each packet as a
  `## Gap-fill pass (2026-08-17)` section — the frozen original bodies above
  each appended section stay byte-identical.
- **Anti-guess rule** (binding for every packet): every claim cites a
  repo-relative path under `refs/deepseek-harness/`; unread areas are stubbed
  as unread, never described from inference.
- **Status**: reference material for maintainers. "Transferable ideas" lists
  are candidates for future debate/planner passes, not adopted policy.

## Slice map

| Slice | Packet | Coverage after gap-fill (2026-08-17) | Headline transferable ideas |
|---|---|---|---|
| Kernel architecture | `kernel-architecture.md` | agent-loop 100% (all src); agent ~70%; tools ~65%; bundles base/headless 100%, web-app ~85%; hooks ~90%; boot ~85%; host ~75%; extensions ~70% (README depth); api/context/preset/util ~60% (group READMEs) | reversible effect registrations + dispose-to-quiescence; event dispatch modes as public contract, catalog-verified; append-only session log + derived projection with `ignorable: true` forward-compat; deny-only guard signatures; package-owned invariant companions; disable-not-delete overlays; named plane-ownership criteria; media-type CSRF gate |
| Session cognition | `session-cognition.md` | docs ~97%; session-projection + cache 100%; session-query ~85%; goal-driver, command-compact, compaction summarizer, jobs, tool-todo, plan-mode index 100%; subagent trio 100% (family bulk remains); log retention confirmed absent by design | `surfaceOp`/`replaceGeneration` surface model; KV-prefix-preserving summarization; own-suffix seq gates for fork-seeded caches; null-sentinel no-value folds; "an id names a slot, not a lifecycle" witnesses; cache-damage renders no verdict |
| LLM / protocols / tools | `llm-protocols-tools.md` | llm core / llm-pi-ai / acp / credentials / sdk / llm-retry src 100%; token-meter 90%; lsp family 100% (17 src files); gen-tool-catalog full; committed tool catalog proven first-party-only (OQ-8) | generated + gated tool catalog; 13-stage tool pipeline; retry as fresh durable turns; credentials as references-not-secrets; reject-stale-keys doctrine; secret-hygiene diagnostics; two-level write serialization; "boot produced nothing" tripwire; explained-empty companion census (13) |
| Execution safety | `execution-safety.md` | sandbox-windows-acl 100% (12/12 src); e2b family 100% (all three packages); code-runtime 100% (10 files); fs-local 100% (4/4); subprocess seam partial (1/3, 2/5); storage partial (3/4, 3/5, 1/6, sqlite 0/4) | typed runner-failure classification; Landlock launcher contract (ABI ceiling gracefully capped); Landlock/ACL fail-closed probes; base64 ASCII control-plane envelopes; `env -i` at both layers + tombstones; zombie-excluding liveness; PID start-identity adoption gates; post-commit-only cleanup swallowing |
| Client runtime | `client-runtime.md` | contract 7/7; sessions 22/22; connection non-fixture complete (fixture excluded by design); ui-slots store/renderer/invariant; web kernel trio; locale runtime read; 4+2 packet-cited invariants spot-confirmed against specs | events+views parallel arrays; higher-seq-wins projection stores; SlotCore shadowing + one-shot abdication; kernel-signal self-sufficiency; revision-bumped late dictionaries; presence-marks-override settings; rpcId echo verification; drain-await backpressure |
| Web & UX surfaces | `web-ux-surfaces.md` | interaction 5/5 subpackage srcs; settings src full (types.ts unread); all 4 shipped preset bodies; agent-presets index.ts (generation mechanics); website/docs.ts closed end-to-end | `--dump-config` with per-row provenance; settings layering with redaction-safe path-ops; teaching error taxonomies at delegation boundaries; pin-by-construction (folds, not snapshots); reconcile-by-installed-state plugin management; bounded escalating shutdown; presets ship curated skill content |
| Engineering platform | `engineering-platform.md` | generators ~95% (9 of 12 in depth); test-support ~70%; knip.json + install-lefthook 100% (full); release lane ~85% (baseline + exe full); BENCHMARK 100%; python smoke ~90% | gen/verify duality — output is a candidate until the paired verifier passes; alignment-soundness granularity ladders; model-request-plane mock assertions; detached-worktree packing; full lock-safety contract (inode re-stat, bounded tolerance, never auto-delete) |

## Cross-cutting themes

- **Model-visible ⟺ logged**: whatever reaches a model request must be
  reconstructable from the session log; everything else is a derived
  projection. (Gap-fill: mechanically enforced in dsh by per-package
  invariant companions.)
- **Session log as single source of truth**: sandbox mode, approvals, and
  presets fold from events ("replay IS state") — no side-channel config
  store to drift.
- **gen-*/verify-* self-verifying catalogs**: generated docs are verified by
  re-running the generator in check mode and byte-diffing; catalogs cannot
  silently rot.
- **Fail-closed defaults**: unknown session events (unless `ignorable: true`),
  unanswerable approvals, unavailable sandbox runners, unresolvable boot
  entries — refuse, never guess.
- **KV-cache-aware design**: summarization inputs built as genuine conversation
  prefixes; a repo-wide "Model Experience" documentation duty (token effect,
  KV-cache effect).
- **Postmortem discipline**: numbered incidents → root cause → one mechanical
  guardrail each; shipped-bug classes graduate into `defensive-patterns.md`.
- **Explained-empty audit companions** (gap-fill addition): every package's
  `./invariant` either checks something or states why not, naming the absent
  surface — no silent no-op checkpoints.

## Known gaps (one block per slice, post-gap-fill)

- `kernel-architecture.md`: `core/agent` src beyond inbox.ts (dispatch,
  consumed-work, model-selection) and `core/tools` src bulk (schema,
  code-mode, presentation, json-schema, py-/ts-types) — deprioritized as
  answered-adjacent; extensions/host/web-app src; module-graph +
  config-catalog bulk (generator-enforced, low read value).
- `session-cognition.md`: `packages/workflow/` src; subagent family bulk
  (~140 files beyond projection/index/list-children);
  session-query-sqlite reconciliation internals (`index.ts`/`query.ts`);
  `session-log-export`; small tails (telemetry-otel / checkpoint-policy /
  title-cadence READMEs, plan-mode types/invariant/client,
  tool-session-query operations/presentation/input).
- `llm-protocols-tools.md`: token-meter `types.ts` (types-only convention);
  sdk findings banked from a prior session's full reads (3 line-cited claims
  spot-verified this pass, the remainder rest on that session); sdk
  backpressure absent at both ends (honest design gap, not a contradiction);
  lsp test suites + the subprocess-seam side of tree-kill/env-scrub
  (README-level sourcing).
- `execution-safety.md`: `subprocess-local/src/{spawn,index,invariant}.ts`
  unread — the seam's tree-scoped-termination "on every platform" clause is
  UNVERIFIED outside linux/darwin terminal paths; pwsh-sandbox beyond
  helpers.ts (parity beyond the four mirror functions unverified);
  sandbox-windows-acl test suite; storage leftovers (storage-json format/
  unit, storage-domain bodies, storage-sqlite all 4, storage error.ts);
  landlock-run beyond entry main.c.
- `client-runtime.md`: `connection/src/client/fixture.ts` (3,188 ln)
  intentionally unread (browser test fixture); feature plugins README-level
  except locale (LanguageRow.tsx, settings-store.ts, `locales/`
  dictionaries unread); real-core slot abdication has no directly-located
  spec (asserted at the web-react layer against a mirrored host);
  `packages/client/web/src/` beyond the three kernel files.
- `web-ux-surfaces.md`: agent-presets src bodies beyond index.ts (discovery,
  authoring, mount, session, metadata, preset, types); settings-file src +
  settings types.ts; apps/web test bodies; ui-agent-preset /
  ui-permission-presets browser halves.
- `engineering-platform.md`: `scripts/release/` families-spine bodies;
  acp-snapshot + client-runtime test-support interiors (runStep dispatch,
  refresh stabilization, launcher handshake; renderSlot lifecycle);
  llm-mock-server / llm-replay src; python-smoke golden snapshot bodies;
  gen-cordis-api / gen-cordis-inspect-catalog / gen-module-graph full bodies.

---

Indexed 2026-08-17; gap-fill merge applied 2026-08-17. Packet bodies above
each `## Gap-fill pass (2026-08-17)` section are frozen research records;
further corrections belong in a new slice pass appended the same way, not in
in-place edits.
