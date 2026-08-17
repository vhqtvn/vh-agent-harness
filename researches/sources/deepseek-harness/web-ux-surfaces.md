Durable home: researches/sources/deepseek-harness/web-ux-surfaces.md (landed 2026-08-17).

# DeepSeek Harness (dsh) — Web & UX Surfaces: Source Packet

## 1. Provenance

- **Repo**: `refs/deepseek-harness` — local checkout of `deepseek-ai/deepseek-harness`.
- **Commit**: `47f943859bef60e4160492346772ded9b24f765a` (2026-08-13).
- **Slice**: `web-ux-surfaces` — packages `web`, `interaction`, `attachment`, `settings`; `apps/` (cli, web); `website/`; `docs/user/`; `docs/web-styling.md`.
- **Audience**: maintainers of vh-agent-harness (Go single-static-binary harness, OpenCode-first UX, own docs system) mining dsh for UX/docs/product-surface ideas.
- **Method**: static reading only (no builds, no network, no git mutations). A prior pass gathered ~70% of the evidence and hit its step budget before writing; this session transcribed that evidence here first, then performed the top-up reads (§9) and folded them in.
- **What dsh is**: Cordis-based everything-is-a-plugin agent harness (pnpm monorepo). Entry `npx @deepseek-ai/dsh web` serves a Web UI at `127.0.0.1:3080`.

## 2. How a user reaches dsh (CLI → web app → client runtime → host)

1. **CLI is a profile launcher** (`apps/cli/README.md`, `apps/cli/reference/README.md`, `apps/cli/src/args.ts`, `apps/cli/src/profile-boot.ts`).
   `dsh web` is a hardcoded alias for `--profile web`. The launcher composes a layer stack:
   bundle patches → profile `cordis.patch.yml` → home `$DSH_HOME/cordis.patch.yml` → `--patch` overlays.
   Later layers **replace the whole row config** (no deep-merge) — a replace-not-merge composition contract.
   `--dump-config` prints the composed tree boot-free, with **per-row source comments** showing which layer won.
2. **Flag-via-service precedence** (same CLI sources + `packages/bundle/web-app/`): the launcher stops parsing at the first unrecognized token; app plugins parse the shared immutable `ctx.cmdlineArgs` snapshot and provide services (e.g. `webStartup`). Flag-configured rows use `!!js ctx.webStartup.port ?? 3080`, so a **flag beats its neighbor literal** — replacing such a row's config with plain literals silently removes the runtime read (a doc-able footgun).
3. **Structural discovery**: `packages/web` is the *web tools* family (search/fetch providers), NOT the web app. The actual web app = `apps/web` (thin ~10-line Vite entry) over `packages/bundle/web-app` (the bundle) + `packages/client/web` (shell library `AppWebEntry`).
4. **Two-stage browser boot** (`packages/client/web/src/boot.tsx`): parse `window.__DSH_BOOT__` manifest → render loading page (works even while plugins fail — shell self-sufficiency rule) → prefetch `immediately` tier ∥ mount Loader → inject `internal` before any entry → post-barrier entry creation → `loader.await()` + full fiber sweep that **fails loud, listing who/which service** (compensates for cordis inject having no timeout).
5. **Host plane holds the registries** (`packages/bundle/web-app/cordis.patch.yml`): agent-plane/host-plane split — the Web **disables** ~24 model-facing tool rows (disable, not delete: the base is shared with the TUI). Stated criterion: "a Service a row outside its realm READS belongs to the host plane". Sessions mount agent presets instead (shipped read-only `system` trust + `$DSH_HOME/.agent-presets` writable).
6. **Model-visible web orientation**: an `app:web-surface` prompt section (order −98) tells the model the GUI URL, the "this page" referent, that there is no implicit DOM context, the HMR update contract, and "don't start replacement servers". `DSH_WEB_URL` is a registered bash env var.
7. **Browser trust fence** (CLI reference + client boot): `--host 0.0.0.0` is intentionally rejected ("would expose RCE to the network"); LAN trust = port-less IPv4 literals sampled once at bind + repeatable `--trusted-host`; the URL line prints only after the Loader settles (readiness signal supervisors RPC on).

## 3. Per-area deep-dives

### 3.1 `packages/web` — web tools family (search + fetch)

- One shared seam for search+fetch with **execution-time provider selection**: structured `WebError` codes; `available()` is never called by the tool itself.
- **Tools register stably even when providers are missing** — schema stability for the model; provider failures become error tool results instead of absent tools.
- `max_results` is deliberately **not model-facing**; timeouts ride `ToolDefinition.timeoutMs`.
- `web-fetch-http/src/policy.ts` (top-up, §9.3): fetch policy details folded in below.

### 3.2 `packages/interaction` — the human-collaboration plane (5 subpackages)

- **commands**: slash-command registry; `command/run` + `command/done` log-only lifecycle.
- **user-approval**: **fail-closed** waterfall; `ask|never` modes; cache-safe policy snapshots.
- **user-questions**: single provider; `intent: plan-review` presentation tags; **live-root authority** — durable lineage is NOT authority (the live in-memory tree decides).
- **tool-ask-user**: `ask_user_question` tool; "(Recommended)" option listed first.
- **permission-presets**: preset bundles of sandbox+approval knobs; **session creation PINS knobs** (later preset edits don't drift an open session).

### 3.3 `packages/attachment` — attachment pipeline

- Content-addressed `sha256:` refs; **never paths/URLs/base64 in events**.
- Admission = full `sharp` decode; reads = **header-only probe** (cheap repeated reads).
- Batch flow: **validate-all-then-save**.
- Unsent drafts are deliberately **browser-owned**.
- fsync-durability walking with **per-process home proof** ("already existed" ≠ "already durable").

### 3.4 `packages/settings` — settings layering

- Layer order: **schema defaults → composition base → user doc**.
- `mutate()` path-ops exist specifically so **redacted-view UIs can't wholesale-delete secrets**.
- `expectedRevision` **optimistic concurrency**.
- `settings-file`: locked read-modify-write with **leaf-level YAML diffs (comment preservation)**, atomic `0600` symlink-proof rename, watcher with reconcile-on-ready.
- The browser sees only a **boolean capability**, never a path.

### 3.5 `apps/` — cli + web

- `apps/cli`: profile launcher (§2.1); reference README documents flags, layering, `--dump-config`. `src/{plugin,dump-config,process-shutdown}.ts` bodies were partially unread at transcription (behavior known via reference README); `dump-config.ts` read in top-up (§9.4).
- `apps/web`: thin Vite entry over the bundle; `apps/web/tests/` (~50 e2e/snapshot files) doubles as a **feature inventory**: goal-bar, plan-review, approval-composer, produced-files, skill-user-invoke, onboarding flows, pwa-manifest, sidebar-subagent-activity, etc.

### 3.6 `website/` — docs-as-product

- `website/docs.ts` is a **canonical publication manifest**: markdown stays in owning tiers, projected into a `.generated` VitePress tree (zh root / en pair projection, **no file copying**).
- Edit links derive from `editSource` frontmatter (**throws if absent** — provenance enforced at build).
- Gates: `verify-doc-site-fragments` (checks built HTML because VitePress slug algorithm ≠ Markdown's), `verify-doc-budgets` (`wc -w` ceilings, ratchet with 5% headroom, **missing file = failure**), `verify-md-wrap`, `doc-typecheck` (fenced ts compiles; type-equiv manifests).

### 3.7 `docs/` — taxonomy, slop checklist, user prose

- `docs/AGENTS.md` tier taxonomy "**one home per fact**" + a **slop checklist**: no narrated history, no implementation-status annotations, no reasoning transcripts — a transferable docs governance model.
- Every package README carries a **"Model Experience"** section (What the model sees / Token effect / KV Cache effect) — KV-cache stability is a first-class documented design concern across the repo.
- Privacy/telemetry defaults: any **non-empty** `DSH_TELEMETRY_DISABLED` disables ("prefers off-by-mistake over on-by-mistake"); credentials resolve env → `.credentials.yaml` → `.envs`, never materialized into `process.env`; **no MCP server enabled by default**.
- `docs/user/` prose: top-up skim in §9.2. `docs/web-styling.md`: top-up in §9.5.

## 4. Findings

- **(finding)**: `dsh web` is a hardcoded `--profile web` alias in a launcher that composes bundle → profile → home → `--patch` layers with replace-not-merge semantics; `--dump-config` prints the tree with per-row source comments. source=apps/cli/README.md + reference/README.md + src/args.ts + src/profile-boot.ts, confidence=high, type=fact
- **(finding)**: Flags reach configured rows via services: rows read `!!js ctx.webStartup.port ?? 3080`, so a flag beats its neighbor literal, and literalizing a row's config silently drops the runtime read. source=apps/cli sources + packages/bundle/web-app config, confidence=high, type=fact
- **(finding)**: `--host 0.0.0.0` is intentionally rejected ("would expose RCE to the network"); LAN trust = port-less IPv4 literals sampled once at bind + repeatable `--trusted-host`; the URL line prints only after Loader settles (readiness signal). source=apps/cli reference + packages/client/web boot, confidence=high, type=fact
- **(finding)**: Two-stage browser boot keeps a working loading page while plugins fail; internal injection precedes any entry; loader.await() + fiber sweep fails loud naming the culprit service. source=packages/client/web/src/boot.tsx, confidence=high, type=fact
- **(finding)**: The Web bundle disables ~24 model-facing tool rows rather than deleting them (base shared with TUI); host plane keeps registries per the stated "row outside its realm READS" criterion; sessions mount agent presets (read-only `system` trust + writable `.agent-presets`). source=packages/bundle/web-app/cordis.patch.yml, confidence=high, type=fact
- **(finding)**: An `app:web-surface` prompt section (order −98) orients the model: GUI URL, "this page" referent, no implicit DOM context, HMR contract, no replacement servers; `DSH_WEB_URL` is a registered bash env var. source=packages/bundle/web-app prompt config, confidence=high, type=fact
- **(finding)**: Interaction package = human-collaboration plane with 5 subpackages; approval is fail-closed (ask|never, cache-safe snapshots); user-questions treats the live root as authority, not durable lineage; permission presets are pinned at session creation. source=packages/interaction/*/README.md, confidence=high, type=fact
- **(finding)**: Settings layer schema defaults → composition base → user doc; path-op mutations protect redacted-view secret deletes; expectedRevision optimistic concurrency; locked RMW with leaf-level YAML diffs, atomic 0600 symlink-proof rename, reconcile-on-ready watcher; browser sees a boolean capability, never a path. source=packages/settings/* READMEs + symbol grep of settings/src/index.ts, confidence=high, type=fact
- **(finding)**: Attachments are content-addressed sha256: refs only (no paths/URLs/base64 in events); full decode on admission vs header-only probe on read; validate-all-then-save batches; unsent drafts browser-owned; fsync walk with per-process home proof. source=packages/attachment/* READMEs, confidence=high, type=fact
- **(finding)**: Web tools share one seam with execution-time provider selection, structured WebError codes, stable tool registration even with providers missing, non-model-facing max_results, ToolDefinition.timeoutMs timeouts. source=packages/web/* READMEs, confidence=high, type=fact
- **(finding)**: website/docs.ts is a canonical publication manifest with projection (no copying), editSource-forced edit links, and four gates: doc-site-fragments (built HTML), doc-budgets (word ceilings, ratchet + 5% headroom, missing = fail), md-wrap, doc-typecheck (fenced ts + type-equiv manifests). source=website/docs.ts + verify scripts, confidence=high, type=fact
- **(finding)**: docs/AGENTS.md codifies "one home per fact" tiers plus an anti-slop checklist (no narrated history, no implementation-status annotations, no reasoning transcripts). source=docs/AGENTS.md, confidence=high, type=fact
- **(finding)**: Package READMEs document a "Model Experience" section (What the model sees / Token effect / KV Cache effect) — KV-cache stability is a first-class, repo-wide documented concern. source=packages/*/README.md, confidence=high, type=fact
- **(finding)**: Telemetry off-by-mistake-preferred (any non-empty DSH_TELEMETRY_DISABLED disables); credentials env → .credentials.yaml → .envs and never materialized into process.env; no MCP server enabled by default. source=repo-level docs/READMEs, confidence=high, type=fact
- **(finding)**: apps/web/tests/ (~50 e2e/snapshot files) functions as a de-facto feature inventory of the web UX surface. source=apps/web/tests/ file list, confidence=high, type=fact
- **(finding)**: `--dump-config` is boot-free AND `!!js`-free (rows print unevaluated), anchors on the same empty root file real boot includes, and has a `--default-only` recovery mode that omits user layers so a broken `cordis.patch.yml` is never parsed. source=apps/cli/src/dump-config.ts (read in full), confidence=high, type=fact
- **(finding)**: HTTP fetch policy: credentials embedded in URLs are rejected (`WEB_BLOCKED_URL`); cross-origin redirects are refused so each origin forces a fresh tool call / permission decision; declared-charset decoding fails loud on unknown charsets; **SSRF/private-network blocking is explicitly deferred** with a pointer to the owning Agent Note. source=packages/web/web-fetch-http/src/policy.ts + packages/web/AGENTS.md, confidence=high, type=fact
- **(finding)**: docs/user is a two-audience published tree: `guide/` (end-user onboarding: model key takes effect without restart; workspace must be selected before the session composer unlocks; **API keys are write-only** — UI holds a redacted descriptor, never the literal secret, and settings store only a credential reference into `.credentials.yaml`) and `develop/` (extension-author tutorials: plugin → config → tool → publish as bundle/profile → framework → three-role capabilities → LLM adapter). source=docs/user/** skim, confidence=high, type=fact
- **(finding)**: Bundle vs profile two-manifest distribution model: bundle = npm package shipping a config layer (`dsh.bundle`); profile = `$DSH_HOME/profiles/<name>` directory (`dsh.profile`); "Nothing is both"; installed via `dsh plugin add`. source=docs/user/develop/basic/publish.md, confidence=high, type=fact
- **(finding)**: Web styling has an ownership contract: `ui-theme` owns all global tokens (`--dsw-*` + semantic aliases + light/dark); feature components use CSS Modules + semantic aliases only — no component library, no Tailwind, no literal colors; unwrapped terminal/diff lines; keyboard-focus and reduced-motion preservation are stated rules. source=docs/web-styling.md, confidence=high, type=fact
- **(inference)**: The whole UX surface is designed around "degrade loudly, never silently": fail-closed approvals, fail-loud fiber sweeps, error-tool-results-not-missing-tools, throwing doc gates, fail-loud charset decoding, honest deferred-SSRF note. A coherent philosophy, not isolated choices. source=synthesis across all areas, confidence=medium, type=inference

## 5. Transferable ideas → vh-agent-harness

1. **`--dump-config` with per-row source comments** (binary distribution): vh-agent-harness composes corpus → overlays → profile → local state; a boot-free "composed view with provenance per row" would make ownership/drift disputes self-service. Maps onto the existing ownership-classification model.
2. **Two-stage web boot + fail-loud dependency sweep**: if vh-agent-harness ever grows a web console, the pattern (self-sufficient shell loading page → barrier → named-culprit sweep) beats a blank page on plugin failure.
3. **Flag-via-service row config** (`!!js ctx.x ?? default`): instructive AND cautionary — runtime-overridable defaults keep flags composable, but the replace-not-merge layer stack makes literalizing a row a silent capability removal. Document the analogous hazard for overlay `opencode-append.jsonc` deep-merge semantics.
4. **Browser trust fence**: reject `0.0.0.0` by default with an explicit "RCE to the network" rationale; trust-by-sampling-once + repeatable `--trusted-host`; print the URL only at readiness. Directly applicable to any future harness-served UI.
5. **Settings layering** (redaction-safe path-ops + optimistic concurrency + leaf-level diff writes + comment preservation): the strongest single import candidate for the `opencode.jsonc`/profile/overlay stack — leaf diffs preserve comments (today JSONC comments are at risk on any programmatic rewrite), and `expectedRevision` prevents clobbering concurrent sessions.
6. **Docs-as-product**: canonical publication manifest with projection instead of copying, provenance-forced edit links, hard CI gates (word-budget ratchets, fenced-code typecheck, wrap checks, built-HTML fragment checks). The generated `docs` command surface + `README.agent.md` freshness rule could adopt budgets + typechecked fences cheaply.
7. **"Model Experience" README section as a repo-wide convention** (What the model sees / Token effect / KV Cache effect): cheap, high-leverage documentation discipline for a harness whose primary user IS a model; aligns with the agent-operability-is-a-feature rule.
8. **Anti-slop docs checklist** ("one home per fact"; no narrated history / implementation-status / reasoning transcripts): directly transferable into docs/AGENTS.md-style governance.
9. **Feature inventory via e2e test names**: naming the e2e suite after user-visible features makes it double as product documentation.
10. **Fail-closed human-collaboration primitives** (approval waterfall, live-root authority, pinned session knobs): applicable wherever vh-agent-harness grows interactive gates (task review, skill-proposal accept/reject).
11. **Write-only credential UX**: the settings UI never re-displays a secret — it stores a **credential reference** and shows a redacted descriptor; settings docs hold references, a dedicated credentials file holds values. Directly applicable to any future vh-agent-harness settings surface (and consistent with its "credentials via env vars, never inline" rule).
12. **Styling ownership contract for a web console**: one theme package owning global tokens + semantic aliases, CSS Modules in feature components, no literal colors, light/dark owned centrally, a11y rules (focus visibility, reduced-motion) stated in the contract. Cheap insurance if `vh-agent-harness` ever ships UI beyond terminal output.
13. **Bundle vs profile two-manifest distribution**: "what a package contributes" vs "which packages compose this setup" as separate manifests with a hard "nothing is both" rule — a clean model for vh-agent-harness overlays (pack = contribution) vs profile (vh-harness-profile.yml = composition), including the recovery-mode config dump that skips user layers.

## 6. Contradictions

- **None detected** within this slice's evidence. (Minor tension, not a contradiction: the base bundle is shared between Web and TUI while the Web patch disables ~24 rows — the disable-not-delete design exists precisely to keep that shared base coherent.)

## 7. Coverage

| Path | Status | Reason |
|---|---|---|
| `apps/cli/` (README, reference, args.ts, profile-boot.ts) | examined | full read, prior pass |
| `apps/cli/src/{plugin,process-shutdown}.ts` | partial | behavior known via reference README; bodies unread |
| `apps/cli/src/dump-config.ts` | top-up | read this session (§9.4) |
| `apps/web/` (entry + tests/) | examined | entry read; tests inventoried by filename |
| `packages/bundle/web-app/` | examined | cordis.patch.yml + prompt sections read |
| `packages/client/web/` | examined | boot.tsx + AppWebEntry read |
| `packages/web/` (search/fetch subpackages) | examined+top-up | READMEs read; `web-fetch-http/src/policy.ts` read this session (§9.3) |
| `packages/interaction/` (5 subpackages) | partial | all READMEs read; `*/src` bodies unread |
| `packages/attachment/` | examined | READMEs read (pipeline semantics documented there) |
| `packages/settings/` | partial | READMEs read; `settings/src/index.ts` symbols grepped, full body unread |
| `apps/cli/config/agent-presets/*.yml` | skipped | preset bodies not load-bearing for this slice's questions |
| `website/` (docs.ts + gates) | partial | docs.ts read except lines 340–460 |
| `docs/user/**` | examined (skim) | 13 en files skimmed this session; one-liners in §9.2 |
| `docs/web-styling.md` | examined | read in full this session (§9.5) |
| `docs/AGENTS.md` (tier taxonomy, slop checklist) | examined | full read, prior pass |

## 8. Open questions

1. What exactly do `apps/cli/src/plugin.ts` and `process-shutdown.ts` add beyond the reference README's account (e.g. shutdown ordering vs browser clients)?
2. `website/docs.ts` lines 340–460 — which projection/gate details live only there?
3. Interaction `*/src` bodies: does user-approval's "cache-safe policy snapshot" survive a mid-session preset edit (pinned knobs suggest yes, but the mechanism is unread)?
4. `packages/settings/src/index.ts` full body: is the redacted view enforced host-side or is it a client convention?
5. Agent preset YAML bodies: what does the shipped `system` trust preset actually allow?

---

## 9. Top-up reads (this session)

Transcribed packet above FIRST from the prior pass's evidence base, then performed these top-ups.

### 9.2 `docs/user/**` prose skim (file-by-file one-liners)

Structure: `index.md` is a meta-refresh redirect into `guide/`; the published user tree serves BOTH end users (`guide/`) and extension authors (`develop/`) — a contributor on-ramp lives inside the product docs, not a separate CONTRIBUTING ghetto.

- `index.md` — redirect shell to `guide/` (VitePress `layout: false` + meta refresh).
- `guide/index.md` ("Use the Web UI") — onboarding canon: start via root README (server prints its URL); **Settings → Models** for API key (takes effect on next request, **no server restart**); **Choose workspace** (fresh Web UI has NO selected workspace; session composer unavailable until one is selected; `dsh` uses its invoking directory as default filesystem location); then run a task.
- `guide/providers.md` — model configuration: DeepSeek card has one API-key field; **keys are write-only** — after saving the page receives a **redacted descriptor, never the literal secret**; key stored in `$DSH_HOME/.credentials.yaml` while settings retain only its **credential reference**; catalog providers (Anthropic, OpenAI, …) supply endpoint/protocol/model list; native-auth providers (Bedrock/Vertex/Azure/Codex) need native credentials, not the API-key field; custom OpenAI-compatible endpoints supported.
- `guide/python-sdk.md` — programmatic alternative to the Web UI: published Python SDK + bundled runtime; Python ≥3.10, Linux x64/arm64, macOS ≥14 arm64; runs a checked-in agent composition.
- `develop/basic/index.md` — "Your first plugin": plugin = TS module exporting `apply(ctx)` + `name`; registers capabilities via ctx.
- `develop/basic/config.md` — plugin configuration: export `Config` type + same-named Schemastery schema; **defaults live on schema fields**.
- `develop/basic/tool.md` — build a tool: `defineTool()` + `ctx.tools.register()`; `inject = ['tools']`.
- `develop/basic/publish.md` — packaging: **bundle vs profile two-manifest model** — a bundle is an npm package shipping a config layer (`dsh.bundle` manifest, "what does this package contribute?"); a profile is a dir under `$DSH_HOME/profiles/<name>` (`dsh.profile` manifest, "which bundles compose this setup, in what order?"); install via `dsh plugin add`; "A bundle is what you author and distribute; a profile is what a user boots with `dsh --profile <name>`. **Nothing is both.**"
- `develop/framework/index.md` — Cordis plugin lifecycle: **fiber state machine** PENDING → LOADING → ACTIVE | FAILED; ACTIVE → UNLOADING → DISPOSED.
- `develop/framework/events.md` — event system: `ctx.on` / `ctx.emit` as the loosely-coupled extension mechanism.
- `develop/framework/service.md` — services: `tools`, `llm`, `agents` are services mounted on ctx; `inject` declares requirements; any plugin can provide one.
- `develop/practice/index.md` — **three-role capability design**: Service Definition / Service Provider / Consumer; "The complete capability is its seam. No individual role is a seam."; Bash worked example (dsh-shell / dsh-bash-local / dsh-tool-bash).
- `develop/practice/llm-adapter.md` — connecting a new LLM provider: extend `LlmAdapter`, implement `stream()` translating provider-neutral requests ↔ provider API.

### 9.3 `packages/web/web-fetch-http/src/policy.ts`

Pure, network-free half of the local HTTP(S) fetch provider (the provider's `fetch()` composes these with transport):

- `validateFetchUrl`: http(s) only, **no embedded credentials in URLs** (`WEB_BLOCKED_URL`), bounded URL length (`WEB_INVALID_URL`). **SSRF / private-network blocking is explicitly deferred** — a code-comment pointer to the package Agent Note (an honest, documented gap, not an absence).
- `isSameOrigin`: **cross-origin redirects are refused** so each new origin requires a fresh tool call — and thus a fresh provider/permission decision. Reinforced by `packages/web/AGENTS.md`: "Reject redirects on credential-bearing provider requests… prove that the redirect target is not contacted."
- `classifyContentType`: `html` (text/html, xhtml+xml) vs `text` (text/*, json, xml, +json, +xml) vs unsupported binary.
- `parseCharset` + `decoderForCharset`: decode with the DECLARED charset (not silent UTF-8 mangling); unrecognized charset → `WEB_UNSUPPORTED_CONTENT_TYPE` — "better to fail loudly than return mojibake".

### 9.4 `apps/cli/src/dump-config.ts`

Confirms and sharpens §2.1:

- The dump composes patch layers through the include plugin's patch algorithm **without booting AND without evaluating `!!js`** — stronger than "boot-free": JS-bearing rows print as-is, unevaluated.
- Layer labels are concrete: bundle **package names**, the profile's own patch path, the `$DSH_HOME` patch file, and **absolute** `--patch` overlay paths (in argv order).
- `--dump-config-default-only` (the `defaultOnly` param) **omits the user layer and `--patch` overlays** — documented as "the recovery diagnostic for a broken `cordis.patch.yml`, which is then never parsed". A first-class broken-config recovery mode.
- The dump "anchors on the same empty root file the boot includes" — dump and boot share one composition entry point, so the dump cannot drift from real boot composition.

### 9.5 `docs/web-styling.md`

25-line style-ownership reference for browser client packages (deliberately does NOT duplicate the token inventory — links the owning `ui-theme` sheets; one-home-per-fact in action):

- **Ownership**: `ui-theme` owns the `--dsw-*` static scale, semantic aliases, typography, motion, gradients, shadows, scrollbar styles, light/dark preference; `ui-layout` applies the resolved theme snapshot to the document; feature packages consume **semantic aliases only** and define no second global theme.
- **Component rules**: CSS Modules + `clsx`; **no component library, no Tailwind**; `--dsw-alias-*` semantic tokens in feature components — no copied palette values or literal colors; light/dark overrides belong to the theme owner only; font sizes paired with line heights; source text / terminal output / diff lines stay **unwrapped** (column preservation) with shared scrollbar styles; presentation lives in CSS (inline React styles may pass component-local custom properties but must not encode theme branches); **preserve keyboard focus visibility and reduced-motion behavior** when adding transitions or hover-only controls.
- **Changing the system**: add/change a shared token in the owning `ui-theme` sheet, consume its alias, update the owning package reference; rationale lives in a dated Agent Note.

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the web-ux-surfaces gap-fill addendum. Method unchanged: static reading
only, `refs/` unmodified, every claim cites a repo-relative path.

### Open-question resolutions

- **OQ 1 (plugin.ts / process-shutdown.ts)** — `apps/cli/src/plugin.ts`
  is NOT a cordis plugin: the `dsh plugin` subcommand, a pnpm forwarder
  that reconciles the `dsh.profile.bundles` layer list against INSTALLED
  state (not dependency diffs — `pnpm update` auto-activates a package
  that gained `dsh.bundle`); template bundles protected; relative fs
  specs re-anchored to the invoking cwd (self-link footgun); git-hosted
  failures print the pnpm≥10 `allowBuilds` remedy; Windows `shell: true`
  post-CVE-2024-27980. `apps/cli/src/process-shutdown.ts`: bounded
  escalating exit controller — 5 s grace force-exit, coalesced
  `shutdown()`, `interrupt()` escalation (second signal = immediate
  force exit), injectable test seams; browser-client teardown lives in
  the app-tree dispose it wraps.
- **OQ 3 (approval snapshot vs preset edit)** — YES, and the mechanism
  is that there is no snapshot to drift: permission knobs are whole-value
  session-log events — "Execution, prompt narration, and replay keep
  reading their knob folds"; the preset TABLE is consulted only at switch
  time; seeded sessions preserve effective values; display derives
  `custom` when knobs match no table entry. "Cache-safe" meant KV cache:
  the policy sentence rides a context section AFTER retained history "so
  switching policy does not rewrite the stable system-prompt cache
  prefix"; the model learns of switches via an injected user message.
  Sharp edge: the invariant validates historical `permission/preset`
  events against the CURRENT table — removing a referenced name fails on
  reload. Supporting: `'never'` decided inside `ApprovalService.request`
  BEFORE dispatch; approval pairs turn-enclosed; user-questions teaching
  errors (`CALLER_NOT_LIVE`, `DELEGATED_CALLER` — the text prescribes
  the recovery); commands' `command/run`+`done` deliberately turn-less;
  `CommandId` never repeats on resume; every interaction package owns
  `./invariant` (real or explained-empty).
- **OQ 4 (settings redaction)** — mechanism host-side, enforcement a
  documented MUST at the seam, structural backstop on the write path:
  `redactSecrets` is a host-side walk with a `RedactedSecret[]` sidecar;
  "Every wire surface MUST pass this; the verbatim default exists for
  same-process configuration UIs only" (a forgetful surface leaks);
  `mutate()` path-ops exist because "a wholesale `replace` rebuilt from a
  redacted document silently deletes every secret the wire never
  returned". Honest fail-open edge (`TODO(settings-wire-redaction)`): a
  secret reachable only via union/intersection/transform nodes returns
  verbatim; the contract forbids modeling secrets that way. Also:
  `revision` bumps on raw-document change; `expectedRevision` at the
  queue front; writes chain past failed predecessors; INVARIANT errors
  harness-fatal after all listeners ran; `documentPath` is availability
  metadata only (paths resolved host-side).
- **OQ 5 (preset bodies)** — premise corrected (see corrections): the
  four shipped presets are `standard`/`code`/`cordis`/`minimal` under
  `apps/cli/config/agent-presets/`. `standard` (251 ln): plan-mode pins
  ("conversational agreement approves nothing"; catalog stable "for
  request-cache stability"); codex + claude-code providers present but
  `disabled: true` ("Copy this preset, then remove `disabled`");
  `tool-web fetch: false`. `code`: `tool-presentation mode: code`
  ("five round trips becomes one"); fails at MOUNT without `codeRuntime`.
  `cordis`: + `tool-cordis` AND two shipped SKILL.md files (capability +
  curated skill content). `minimal`: persona `complete: true` +
  `includeRuntimeContext: false` (later listeners cannot add prompt
  text); persistent PTY bash; entry-local `fs-local` realm-shadowing the
  host provider. Lifecycle: standing mounts with `(mtimeMs, size)`
  generations — edits start a new generation for FUTURE sessions,
  "Sessions already joined keep the generation they run on";
  `composeFrom` = "a bind, not a mount"; `recompose` only before the
  agent "has produced nothing" (CALLER owns the check).
- **OQ 2 (website/docs.ts 340-460)** — gap read (330-524, file closed):
  `PairedPage` is a specialization of `MirroredPage`; per-page outline
  control rides the manifest; sidebar placement declared, ordered,
  FAIL-LOUD (`sectionSpec()` throws on undeclared placement); navigation
  targets DERIVED, never written down (`landingLink` throws on an empty
  collection); the website tree has a mechanical no-content-here gate
  (`website/AGENTS.md`, `pnpm docs:check`).

### New findings

- `system`/`user` is a per-ROOT trust classification governing authoring
  writability only; preset capability is entirely the body's composition
  (`packages/preset/agent-presets/src/{preset,index,authoring}.ts`).
- Composition mid-session-edit safety = standing mounts + file-stamp
  generations; children bind the parent's exact instance.
- Standard preset ships `fetch: false` web search and disabled
  codex/claude-code providers (exposure via preset copy).
- The `cordis` preset ships SKILL.md content — presets distribute
  knowledge with capability.
- `minimal` demonstrates the prompt-surface floor and realm shadowing.
- Plan-mode prompt pins the two unusual rules (agreement approves
  nothing; identical catalog across modes).
- docs.ts: composable projection kinds; manifest-declared outline/collapse;
  build-time placement failures (`website/docs.ts`).
- `CommandResult.sourceEventSeq` points at the authoritative domain
  event; `command/run` payloads structured ("never re-parses a line")
  (`packages/interaction/commands/src/types.ts`).
- Client/host faces project through a pure re-export ("Client code
  imports ONLY the client namespace") (`permission-presets/src/client.ts`).
- Settings commit contract is a runtime invariant (`settings/updated`
  only for registered namespaces, only on resolved-value change)
  (`packages/settings/settings/src/invariant.ts`).

### Transferable ideas added

(packet §5 carries 1-13; new 14-20) — **14** teaching error taxonomy at
delegation boundaries; **15** pin by construction (whole-value events +
folds, not snapshots); **16** write-path protection for redacted views
(path-ops); **17** reconcile-by-installed-state plugin management; **18**
bounded escalating shutdown; **19** preset = capability + curated skill
content; **20** derived navigation + fail-loud placement in publication
manifests. Update to packet idea 5: add revision-on-raw-change,
queue-front conflict checks, and INVARIANT-fatal fan-out.

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| `apps/cli/src/{plugin,process-shutdown}.ts` | partial | full reads |
| `packages/interaction/` src | READMEs only | all 5 subpackages (2 trivial files skimmed) |
| `packages/settings/settings/src/` | symbols only | index/redact/invariant full (types.ts unread) |
| `packages/settings/settings-file/src/` | README-level | unchanged |
| `apps/cli/config/agent-presets/` | skipped | all 4 (standard/minimal full; code head+tail; cordis rows + skills) |
| `packages/preset/agent-presets/src/` | unread | index.ts full |
| `website/docs.ts` | 340-460 gap | closed (330-524) |

### Contradictions and corrections from this pass

- **'system' trust classification ≠ preset** (cross-round ledger): the
  frozen §2.5 "shipped read-only `system` trust" could read as a preset
  named `system`; `system` is a root-trust classification
  (`PresetTrust = 'system' | 'user'`,
  `packages/preset/agent-presets/src/preset.ts:8`); the shipped presets
  are `standard`/`code`/`cordis`/`minimal`. Intended wording: "shipped
  presets under system-trust (read-only) roots". Frozen body unchanged.
- Minor tension worth preserving (not a contradiction): approval audit
  pairs REQUIRE an open turn while command lifecycle events are
  deliberately turn-less — different durability roles the frozen
  "log-only lifecycle" one-liner blurs.
- No other contradictions detected.
