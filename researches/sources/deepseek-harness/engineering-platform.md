Durable home: researches/sources/deepseek-harness/engineering-platform.md (landed 2026-08-17).

# DeepSeek Harness (dsh) — Engineering Platform: Source Packet

## 1. Provenance

- **Source repo**: deepseek-ai/deepseek-harness, local checkout at `refs/deepseek-harness/`
- **Commit**: `47f943859bef60e4160492346772ded9b24f765a` (2026-08-13)
- **Slice**: engineering-platform — `scripts/` gate system, `.github/` CI + issue-management policy-as-code, `.gitlab-ci.yml`, lefthook, knip/jscpd/oxlint configs, vitest config family, `pytest.ini`, `python/`, `packages/test-support`, `packages/typert`, `docs/development.md`, `docs/testing.md`, `docs/i18n/`, CONTRIBUTING, BENCHMARK, release system, vendor governance.
- **Audience**: maintainers of vh-agent-harness (Go repo-resident agent harness with doctor health checks, gated commits, ownership classification, AGENTS.md discipline, generated+gated catalogs) mining dsh's engineering platform.
- **Subject in one line**: "DeepSeek Harness" (dsh) is a Cordis-based everything-is-a-plugin agent harness (pnpm monorepo); this packet covers the *engineering platform* around it — how the repo verifies itself.
- **Method**: static reading only; dsh code never executed by the research pass. Confidence reflects read depth per item: **full** (entire file read), **structure** (header + structural scan of a large file), **cross-ref** (existence + role known from other files' references only; body unread). Anti-guess rule: nothing below describes unread file *contents* beyond what a read file asserts about them.
- **Evidence provenance**: gathered by a prior researcher pass (~70% slice coverage) that hit its step budget before writing; this pass transcribed the packet first (write-fence honored), then performed top-up reads (see §8 Coverage for the final state).

---

## 2. Gate system map

### 2.1 Orchestrator: `scripts/run-gates.ts` (890 lines — read in full)

Single orchestrator for every verification lane. A gate is a node in a **dependency DAG** (`Gate.needs`); the graph is validated before anything runs (`validateGateGraph` / `findDependencyCycle`: duplicate ids, unknown dependencies, cycles). A gate whose dependency failed is **skipped**, and the skip cascades; the process exits 1 if any non-`allowFailure` gate failed *or was skipped*. `allowFailure: true` is inherited by aggregate modes (e.g. Windows observational results carried into `ci-windows-complete`).

**14 modes** (`Mode` type, lines 15–29):

| Mode | Role |
|---|---|
| `ci-primary` | Full Linux primary aggregate (see 2.3) |
| `ci-linux-primary` | Linux-only subset of primary |
| `ci-static` | Static analysis lane |
| `ci-lint-contracts-ready` | Lint/contract readiness lane |
| `ci-coverage` | Coverage lane (worker-budget split, see 2.2) |
| `ci-snapshot` | Keyless snapshot replay lane |
| `ci-artifacts` | Build-artifact verification lane |
| `ci-consumers` | Consumer-install lane (one worker per gate) |
| `ci-windows-blocking` | Wine Windows gates that block |
| `ci-windows-complete` | All Windows gates (observational allowed to fail) |
| `ci-windows-observational` | Windows observational-only lane |
| `node-compat` | Node version compatibility matrix lane |
| `check-all` | Local "run everything" aggregate |
| `doc-sync` | Documentation sync aggregate |

### 2.2 Mechanics (cross-cutting)

- **Shell-free pnpm invocation** (`pnpmInvocation`): invokes pnpm via `npm_execpath` + `process.execPath` instead of shelling out — Windows-safe, no shell-quoting surface. *(source: scripts/run-gates.ts, full read — confidence high, type fact)*
- **Concurrency**: per-mode defaults (`defaultConcurrency`); local doc modes capped at **4 workers** explicitly to avoid memory blowup; `DSH_GATE_CONCURRENCY` env override; `ci-consumers` runs **one worker per gate**.
- **Coverage worker-budget split**: `DSH_COVERAGE_MAX_WORKERS` is split ~2/3 to instrumented suites and ~1/3 to exempt-heavy suites (`coverageWorkerArgs`). Backed by `scripts/coverage-exempt.ts` (+ spec): suites may be exempted from instrumentation only if their **measured files are fully covered elsewhere**; `DSH_COVERAGE_EXEMPT_HEAVY=1` selects the heavy lane. The exemption is *budgeted*, not forgiven.

### 2.3 Named aggregates and gate groups (as composed in run-gates.ts)

- **`ciPrimaryGates`** — the spine: static + typert-contracts → typecheck/lint → coverage → node-compat smokes → snapshot → doc-sync leaves → build → `publint` / node-next / built-invariants / built-bin-smoke.
- **`docSyncLeafGates`** — ~27 documentation gates, including all `verify-*-catalog` gates, agent-note gates, translation gates, `doc-budgets`, and the docs-site build.
- **`builtBinSmokeGate`** — 12 e2e entry files proving the **built** `lib/` entrypoints work under plain Node (real-entry-path verification, not source-plane).
- **`nodeCompatSmokeGates`** — source-worker, zstd, source-launch, jsdom, CLI lazy-search smokes across Node versions.
- **Windows split**: `ci-windows-blocking` vs `ci-windows-observational` vs `ci-windows-complete`; observational gates carry `allowFailure: true` into the complete aggregate.

---

## 3. verify-*/gen-* catalog (same-script duality: every `gen-*` doubles as `verify-*` via a `--check` mode)

Design invariant: **generators are the verification** — a catalog is verified by re-running its generator in check mode and diffing. No separately-maintained expectations.

| Script | Mechanism | Protects what | Artifact guarded |
|---|---|---|---|
| `gen-cordis-catalog.ts` (51KB, structure) | Per-subsystem Cordis API regions injected between GENERATED markers, **byte-identically into both language sides**; file:line staleness semantics | Cordis API doc/catalog drift; EN/ZH divergence | Cordis catalog docs |
| `gen-config-catalog.ts` (41KB, structure) | Derives catalog from package entry points + Schemastery schemas | Config doc drift vs real schema | Config catalog |
| `gen-tool-catalog.ts` (40KB, structure) | Collects tool schemas by **booting each tool plugin** | Tool docs vs actual tool surface | Tool catalog |
| `gen-persistence-catalog.ts` (structure) | Merges `SessionEventMap` + generates `known-event-types.ts` | Event-type list drift | Persistence/event catalog |
| `gen-client-catalog.ts` (27KB, structure) | **Lexical slot-walk, no type-checker**; feeds `cordis_inspect` | Client API surface drift | Client catalog |
| `gen-scoped-events.ts` (structure) | TS-program-derived resolver map; 0 matches needs explicit `@dshScopeScan unsupported`; multi-match fails | Scoped-event resolver rot | Scoped event map |
| `gen-module-graph.ts` + `gen-doc-graphs.ts` (structure) | peerDependencies → mermaid diagrams + tables | Architecture diagram drift | Module/doc graphs |
| `gen-third-party-notices.ts` (36KB, structure) | Workspace manifests + vendor manifest + pyproject + patches | Third-party notice drift | `THIRD_PARTY_NOTICES.md` (pre-commit auto-regenerates) |
| `gen-translation-brief.ts` (structure) | Narrowest-granularity update briefing (code-fence splice → units → sections → whole doc) + `--apply` | Translator effort/accuracy | i18n briefs |
| `gen-cordis-api.ts` (structure) | Compat entry | API surface compat | Cordis API entry |
| `gen-cordis-inspect-catalog.ts` (structure) | Companion to client catalog | Inspect-tool drift | cordis_inspect catalog |
| `verify-agent-note-format.ts` / `-classification.ts` (full) | Line/section grammar per lifecycle; class/lifecycle set membership | Agent-note discipline | `.agents/notes/` |
| `verify-translation-pairing.ts` (full) | 3-file pairs + structural-signature diff (§4.3) | EN/ZH doc equivalence | `*.zh.md` + `*.i18n.yaml` |
| `verify-doc-budgets.ts` + `doc-budgets.manifest.json` (full) | Word ceilings per doc (AGENTS.md 1900, architecture 2400, testing 1150, …); relocate → condense → **raise ratchet** | Doc bloat | Key docs |
| `verify-type-equiv.ts` (full) | Manifest-driven verbatim type-paste equivalence via TS parser; public-api projection | Docs lying about types | Type-bearing docs |
| `verify-md-wrap` / `verify-md-links` / `verify-doc-refs` / `verify-package-paths` / `verify-public-repository-links` / `verify-mermaid` / `verify-export-jsdoc` (existence + role; docSyncLeafGates members) | Markdown hygiene family | Link rot, wrap style, ref validity, path validity, mermaid syntax, exported jsdoc presence | Docs tree |
| `verify-doc-site-fragments` (partial — purpose known, body not fully read) | Headers gathered from specs | Docs-site fragment consistency | Site fragments |
| `verify-vendored-links.ts` (full) | Lockfile must resolve vendored names to `link:` — never registry | Registry squatting via vendored deps | pnpm lockfile |
| `verify-runtime-closure.ts` (full) | exe deploy manifest must supply every workspace peer | Runtime missing deps | Deploy manifest |
| `verify-packed-install.ts` (release, header) | Throwaway consumer outside repo; vendored family tarballs passed `--from` | Broken release artifacts | npm tarballs |

*(Confidence: high for full-read rows; medium for structure rows — sizes and mechanisms from header/structure reads of large files. Type: fact for mechanics, inference where intent is inferred from naming + doc context.)*

---

## 4. Per-area deep-dives

### 4.1 Test infrastructure

**Vitest config family** (all read in full):

- `vitest.config.ts` — **dual forks-pool projects** (thread-safe vs process-bound suites separated); **per-file 100/100/100/100 thresholds** (lines/branches/functions/statements); custom `coverage-uncovered-locations.cjs` reporter emitting exact `path:line:col`; Windows-conditional excludes; pwsh-probe-conditional exemptions (exemption active **exactly when** the suites skip — presence of pwsh gates the exemption, not the platform constant); coverage-exempt env integration; a large **explicit GUI-debt exclusion list** each entry carrying `TODO(gui)` comments (exclusions are named debts, not amnesties); tsconfig-paths facade resolves **source-plane**, never built `lib/`.
- `vitest.shared.ts` — `--no-webstorage` flag; standard-decorator pre-transform plugin injecting v8-ignore for `__esDecorate` (decorator-transform coverage holes handled at the transform seam).
- `vitest.snapshot.config.ts` — **keyless replay by default** (no per-assertion keys); `DSH_SNAPSHOT=record|refresh`; replay parallel with `DSH_SNAPSHOT_MAX_WORKCURRENCY`; record/refresh **serial** (parallel recording corrupts goldens).
- `vitest.e2e.config.ts` — real-API e2e, **self-skipping** when credentials absent; `retry: 2`; `DSH_E2E_MAX_WORKERS`.
- `vitest.web.config.ts` — serial browser lane. `vitest.web.perf.config.ts` — `*.perf.ts`, 600s timeouts, console intercept off. `vitest.web-stress.config.ts` — `*.stress.ts` opt-in.

**`scripts/test-invariants.ts`** (full): monkey-patches `RegistryService.prototype.plugin` so **every ordinary test root automatically gets the invariant service** plus its owning package's companion; lazy `import.meta.glob` of ~168 companions; a topology test mounts all of them; a manual-topology exception list for tests that must construct registries by hand. Philosophy: invariants are not opt-in — the default test path cannot forget them.

**`packages/test-support/`** (6 subpackages; roles from package docs/headers):

- `llm-mock-server` — scriptable OpenAI-compatible **fault server**: ~25 wire behaviors (`connection_reset`, `partial_disconnect`, `stall`, `malformed_json`, `rate_limit`, seeded weighted-random mode); FIFO script consumption; `mock:llm` CLI emits JSONL telemetry.
- `llm-replay` — fixture = **the persisted session JSONL itself** (no separate fixture format); assistant/chunk reconstruction per `(turn, step)`; override sidecars patch **by index**; `{{fromRequest:<regex>}}` live-request placeholders; per-session first-call-order keying to support subagents.
- `acp-snapshot`, `agent-loop-testkit`, `client-runtime`, `loader-smoke` — (roles known; bodies not deep-read this slice).

### 4.2 typert (`packages/typert/`, 4 subpackages: registry / loader / generator / protocol)

Generator = TS analyzer → **compiler-independent FaceModel/TypeGraph** → Zod schema + TYPERT-contribution emitters; `check` and `write` modes (`write` inserts checker-derived annotations); runs as a **tsdown plugin in the Host face only**; the repository-specific Cordis projection policy is **passed in from scripts** — no hidden taxonomy inside the package. Fixtures include full type-model and remote-model trees. Package is **coverage-exempt by design**: correctness pinned by fixture suites + a byte-for-byte catalog reproduction test instead of instrumentation. Contract surfaces appear in the primary aggregate as `typert-contracts` early in the spine (before typecheck).

### 4.3 i18n / translation pairing

- `verify-translation-pairing.ts` (full): **3-file pairs** — `foo.md` + `foo.zh.md` + `foo.i18n.yaml` (the yaml holds git blob hashes of the last-confirmed-consistent state). Three modes: corpus-wide, `--cached` (operates on the **git index plane** via `readGitIndexBlob` — pre-commit never touches the worktree), and named-pairs. `--write` records confirmations and stores snapshots **in the git ODB** under content-addressed refs `refs/dsh/translation-pairing/snapshots/*` (GC-proof recovery pointers; `translation-pairing-git.ts`). Checks: generated regions must be **byte-identical** on both sides; **structural signature diff** (headings, tables, lists, code fences, link targets) — prose may diverge, structure may not; language-switcher rules with a generated-source exemption; exclusions are an **exclusions-only manifest** (nothing implicitly exempt).
- `merge-translation-pairing.ts` + `merge-translation-pairing-driver.sh`: registered as git merge driver `merge.dsh-translation-pairing` (`.gitattributes`), **fail-closed** — a merge it cannot confidently resolve marks conflict rather than guessing.
- `gen-translation-brief.ts`: computes the **narrowest granularity** of needed translation update (code-fence splice → units → sections → whole doc) and can `--apply` it.
- `docs/i18n/README.md` (full): languages are **equal-authority** (neither side is the source of truth); pairs merge whole; and an explicit honesty limit: **"green gate ≠ sound confirmation"** — the structural check proves structure matches, not that the translation is good.

### 4.4 Release system (`scripts/release/*.ts`, headers)

- `families.ts` — three families: **dsh / vendor / native**, each with its own version baseline, tags, and publish set.
- `bump.ts` — version is **committed in-repo** and readable without CI.
- `pack.ts` — **credential-free pack from exactly one commit** = the release boundary; order file.
- `publish.ts` — **REGISTRY-DECIDED PUBLICATION**: manifest missing → publish; identical integrity → skip (idempotent re-run); differs → **fail** (content changed without a bump). The registry is the arbiter of what has been published, not local state.
- `verify.ts` — tag + publishability gates, Actions-only.
- `verify-packed-install.ts` — throwaway consumer **outside the repo**; vendored-family tarballs passed `--from` so **cross-family bumps verify pre-publish**.
- `tarball.ts`, `process.ts` (three failure behaviors).
- Bodies **not read**: `publish-npm-baseline.ts` (38KB), `build-exe-for-python-sdk.ts` (21KB), `smoke-python-runtime.py` (39KB); `build-python-release.py` header only.

### 4.5 Agent-note system

- `scripts/agent-note-tree.ts` (full): closed **lifecycle** set (proposed / implemented / rejected + archived) and closed **class** set (feature / bug-fix / simplification / architecture / process / testing); filenames `yyyy-mm-dd-topic.md`; `INDEX.md` forbidden; legacy homes (`docs/rfc`) forbidden.
- `verify-agent-note-format.ts` (full): line 1 `# Agent Note: <title>`; line 3 lifecycle `Status` grammar; **required sections per lifecycle** (proposed: Proposal / Acceptance criteria / Risks; implemented: Decision / Consequences; rejected: Proposal); proposal-era headings banned in implemented notes; mandatory `## Alternatives considered` (or a pre-2026-07-05 grandfather comment); retired debt markers banned.
- `verify-agent-note-classification.ts` — structure gate for class/lifecycle membership.
- `archived-agent-notes.ts` (full) + spec: notes archived as **triplets** (`.md` / `.zh.md` / `.i18n.yaml`); **SHA-256 frozen-content manifest** (append-only, extension-validated); git blob-hash sidecars; archive headers with `Archived: <date>` ≥ filename date; EN↔ZH archive-date equality enforced.
- `.agents/notes/README.md` budget: 125 lines. Root `AGENTS.md` rule: **non-trivial changes MUST include an Agent Note in the same PR**.

### 4.6 Issue-management policy-as-code (`.github/issue-management/policy.mjs` + `config.json`, full)

- Validation errors are **Chinese-language** (community norm encoding).
- Issue body ≤ **50 visible units** outside collapsed `<details>` (Han chars + Latin tokens).
- `Owner: @login` line required **only when ≥2 assignees**.
- Title must contain Chinese; no type/priority/status prefixes.
- `Type` ∈ 5 native GitHub types; Project statuses Inbox→Backlog→Ready→In progress→In review→Done/No action, with **Done↔completed / No action↔not_planned closure coupling** (project status and close state are one transition, not two).
- PR policy: exactly one `kind/*`, ≥1 `area/*`, ≤1 `priority`, must reference ≥1 same-repo issue; priority propagates from resolved issues.
- Legacy label aliases reserved; lifecycle automation via **ProjectsV2 GraphQL** (status transitions derived from `review_requested` / `changes_requested` events; `lifecycleActor` loop-guard so the bot never reacts to itself); audit comment upsert keyed by marker `<!-- dsh-issue-policy -->`.
- **The policy is tested** by `policy.test.mjs` (`test:issue-management`), itself a CI gate inside `ciSharedStaticGates` — policy-as-code with its own green light.

### 4.7 Python lane

- `pytest.ini`: root collection restricted to `python/sdk/tests` (keeps pytest out of the TS monorepo's fixtures).
- CI: `python-sdk` job (3.10, keyless pytest via **uv**), `python-runtime` job (release-shaped node24-linux-x64 exe).
- `.gitlab-ci.yml` (full): Python **tag-gated** wheel pipeline — GLIBC ≤2.28 `readelf` assertion; manylinux_2_28 container smoke; macOS deployment-target check; tag↔package.json version match.
- Publication: **trusted publishing** via `pypa/gh-action-pypi-publish`; validate vs publish **environments** separated; **no checkout in publish jobs**; enforced by `ci-workflow.spec.ts`.

### 4.8 Vendoring governance

- `rescape-vendor.ts` (header + structure): rewrites vendored Cordis to `@deepseek-ai/*` scope (**prevents registry squatting** — a hostile publish of the upstream name can't be linked in); idempotent delimited-token rule; `EXACT_EDITS` with **hit counts** (upstream change makes the rescope fail loud instead of silently skipping); postconditions re-checked in `--check` mode; `--reverse` for re-vendoring from a fresh upstream drop.
- `check-vendor-manifest.sh`: a staged `vendor/src` change **must co-stage** a `vendor/README.md` manifest edit (pairing rule at the git stage boundary).
- `verify-vendored-links.ts`: lockfile resolves vendored names to `link:` — never registry. `verify-runtime-closure.ts`: exe deploy manifest supplies every workspace peer.

### 4.9 CI topology (`.github/workflows/ci.yml` 937 lines, full + `scripts/ci-workflow.spec.ts`, full)

**PR lanes**: `node-24/static`; `node-24-coverage`; `node-24-consumers` (16-core hosted); `node-compat` matrix (22.19, 26); `python-sdk`; `python-runtime` (release-shaped exe); `windows` (real win-x64 Node **under Wine on hosted Linux — BLOCKING**); `windows-native` (real Windows, non-blocking). **Master-only standby drills**: `serial-linux-selfhosted`, `serial-windows`, `wine-apt-cache` cache seeder. Two `workflow_dispatch` **runner-benchmark** suites (4→96 core matrices). Aggregate `all-checks-passed` = **single stable required check**; its `if: always()` is load-bearing (the aggregate must evaluate even when a lane fails).

**CI-as-code-under-test**: `ci-workflow.spec.ts` **parses the workflow YAML and asserts its own CI's topology** — pnpm action-setup destination isolation; Wine job topology; **failover switches** (`DSH_CI_FAILOVER_LINUX/_WINDOWS` vars → self-hosted pools; dependabot excluded); push-run exemption from cancel-in-progress + an **exact allowlist of push-reachable jobs** (so standby drills always reach a verdict — rationale documented inline); Vitest forks pool ×2; release-shaped python target required; E2B manual-only; python release separation (see 4.7); GitLab macOS check reuse; issue-lifecycle event shapes; lefthook exclusions for archived notes.

### 4.10 Windows/Wine gates (`scripts/wine-windows-gates.sh`, full)

- **Snapshot-tree isolation**: never mutates the working tree — tar of tracked + untracked-unignored files minus `.claude/.codex`.
- Wine-specific pnpm overrides appended to the snapshot's `pnpm-workspace.yaml` (hoisted linker, win32-x64 arch).
- **Checksum-verified** Windows Node zip with offline cache fallback + mirror resume.
- pnpm hoisted-linker **rename-race retry** (signature-gated on pnpm#12880).
- **stdio-through-files** (Wine EBADF workaround).
- Unix symlink for vue.
- Two blocking surfaces run **concurrently** (Host+Client tsc/tsdown build; vitepress site); **all statuses captured so neither failure hides the other**.

### 4.11 Hooks (lefthook.yml full; install-lefthook.mjs 845 lines — header + lock/ownership core read this pass)

- **pre-commit**: staged translation-pairing check (**index-plane**, archived excluded); archived-agent-notes verify; staged **oxlint** with `.oxlintrc.staged.json` (+`stage_fixed`); **third-party-notices auto-regeneration** (glob covers all generator inputs; `git add THIRD_PARTY_NOTICES.md`) — *regenerate instead of reject*; whitespace (`git diff --cached --check`); vendor manifest guard.
- **pre-merge-commit**: pairing + archived notes. **pre-push**: typecheck.
- **Principle**: hooks are fast local checkpoints only; heavy verification lives in the gate system. Root AGENTS.md codifies the local/CI split: "Never default to the full suite or repeat a passing check for commit or push. CI owns exhaustive coverage and the platform matrix" (with a `dsh-pre-push-checks` skill; "report only commands run"), and `check:windows-wine` is local "ONLY when diagnosing a known Windows failure… CI owns this signal".
- **Installer contract** (`install-lefthook.mjs`, lines 1–150 + lock/ownership sections):
  - Constants: `MINIMUM_GIT = 2.26.0`; worktree-local hooks dir `dsh-hooks`; ownership marker `.dsh-lefthook-owned` (version 1 + owner string); `DSH_LEFTHOOK_ALLOW_HOOKS_PATH_OVERRIDE` env escape hatch; guard pattern `^extensions\.` — refuses when enabling a git extension would activate *user-owned* settings (config provenance is traced via `--show-origin`/`--show-scope`, distinguishing direct-file vs included-file entries before touching anything).
  - **Install lock** (`dsh-lefthook-install.lock`, common dir): exclusive create (`wx`, 0o600) with record `<pid> <uuid>`; release re-verifies dev+inode+record bytes before unlink (refuses if ownership changed); contention path validates record/stat (TOCTOU re-stat after read), polls 50 ms up to 30 s (1 s init timeout); a lock whose owner PID is dead is **stale → manual recovery error, never auto-deleted**.
  - **Hooks-dir ownership**: marker written exclusive (`wx`, 0o600) binding the hooks path; installer *refuses to overwrite an unowned hooks directory* and refuses an invalid marker.
  - Registers `merge.dsh-translation-pairing.driver` (name + `scripts/merge-translation-pairing-driver.sh %O %A %B %P`), gated by a `tsx/esm --probe` of the driver script (**fail-closed**: probe must succeed before the driver is trusted).
- `scripts/AGENTS.md` codifies the orchestrator style repo-wide: "Gate scripts invoke pnpm shell-free, normalize repository-relative glob paths to `/` at ingestion, and keep platform adaptation in the gate that needs it instead of a shared platform layer."

### 4.12 Meta-configs

- `.oxlintrc.json` — type-aware oxlint + tsgolint; correctness rules off + explicit rule list; vendor/native excludes documented inline.
- `knip.json` — 787 lines, **sampled this pass** (head + key-pattern census): overwhelmingly *per-workspace entry/project declarations* — test fixtures (`.spec.ts`, `.e2e.ts`, `.snapshot.ts`, example agents/drivers) declared as **entries** so knip understands the test-only surface is live, not suppressed; `ignoreWorkspaces: vendor/*, python/sdk-runtime`; `ignoreBinaries` = platform/sandbox tools (bwrap, icacls, musl-gcc, sandbox-exec, tar, taskkill, where.exe, python3); two `ignoreDependencies`; **no export-level ignores found** (no `ignoreExportsInFiles`-style keys) — dead-export hygiene is enforced, not waived. Runs inside `pnpm run hygiene` (= knip + publint + workspace constraints + NodeNext consumer check, per root AGENTS.md).
- `.jscpd.json` — `minTokens 60` (mild), tests ignored, `exitCode 1` (copy-paste detection is a hard gate but a soft threshold).
- `.editorconfig` + `.gitattributes` — LF canonical; `merge=dsh-translation-pairing` driver attribute on paired docs.
- `tsdown.config.ts` — build faces selected via `--env.DSH_BUILD_FACE`; `typertPlugin` host-only.
- `pnpm-workspace.yaml` — `catalog:`, `linkWorkspacePackages`, vendor overrides, **`allowBuilds` DENY-BY-DEFAULT install-script policy** with reviewed exceptions, `minimumReleaseAgeExclude`, `patchedDependencies`.
- `check-workspace-constraints.ts` (header) — workspace globs, vendored set, publication source allowlist, project-reference face violations.

### 4.13 Docs (all listed read in full unless noted)

- `docs/development.md` — two-aggregate tsconfig architecture; **source-plane vs artifact-plane** distinction; TODO marker semantics (FIXME/TODO/XXX); ts type-equiv mechanics.
- `docs/testing.md` — **5 test tiers**; per-file 100% philosophy — "an uncovered line is often dead code"; `with-key` policy — "inference is cheap here" (favor type-level proofs over runtime fixtures); **verify-the-world-not-self-report**; real-entry-path testing including **built lib/ + bin.js smokes**; snapshot-required rule.
- `docs/AGENTS.md` — tier taxonomy, **one-home-per-fact**, slop checklist, word budgets + **5% headroom rule**.
- `docs/i18n/README.md` — see 4.3. Root `AGENTS.md` — agent-note mandate, engineering norms, and notable codified policies confirmed this pass: `pnpm run hygiene` aggregate; CI-owns-exhaustive-coverage local-check policy; sandbox-failure escalation guidance ("retry unchanged with the narrowest host escalation… never bypass genuine test failures"); **`CLAUDE.md` symlinks `AGENTS.md`** at root, `packages/`, `examples/` (one rule file, many agent CLIs); doc-budget escape valve ("raise a `verify-doc-budgets` ceiling when the required content genuinely needs more space"); PR-history discipline (split independent changes; `--force-with-lease` only); label taxonomy (one `kind/*`, material `area/*`, native Issue Types).
- `CONTRIBUTING.md` — read in full this pass (23 lines, itself an i18n pair with `CONTRIBUTING.zh.md`): **external PRs not accepted at this stage**; contribution routes are GitHub Discussions (upvote-weighted triage for a very small team), ecosystem plugins (`dsh-plugin` GitHub topic), blog posts, and community Q&A. Explicit stance: official packages are *not* inherently more important than community ones — "this repository [is] an idea, an official showcase, and a source of inspiration, but not a mandate." Also notable: the engineering-platform surface (gates, doc-sync, i18n pairing, release) exists and stays fully verified **despite zero external code contributors** — the audience for the platform is the internal team + agents, not a PR firehose.

---

## 5. Findings

- **(finding)**: dsh runs all verification through ONE orchestrator (`run-gates.ts`) with a validated dependency DAG, cascade-skip semantics, and per-mode concurrency budgets. source=scripts/run-gates.ts (full), confidence=high, type=fact
- **(finding)**: every generator doubles as its own verifier (`--check`); catalogs are never verified against separately-maintained expectations. source=scripts/gen-*.ts (structure reads) + docs/development.md, confidence=high, type=fact
- **(finding)**: CI workflow YAML is itself under test — `ci-workflow.spec.ts` parses `ci.yml` and asserts topology, failover vars, push-reachability allowlists, and publication hygiene. source=scripts/ci-workflow.spec.ts (full), confidence=high, type=fact
- **(finding)**: Windows verification is blocking on hosted Linux via Wine (snapshot-isolated, checksummed Node, rename-race retries, stdio-through-files), with real-Windows as non-blocking observational. source=scripts/wine-windows-gates.sh (full) + ci.yml (full), confidence=high, type=fact
- **(finding)**: issue/PR management is executable policy (ProjectsV2 GraphQL lifecycle automation) with its own test suite running as a CI gate. source=.github/issue-management/* (full), confidence=high, type=fact
- **(finding)**: coverage philosophy is per-file 100/100/100/100 with *budgeted, named* exemptions (exempt suites' measured files must be fully covered elsewhere; GUI exclusions carry TODO(gui) debt markers). source=vitest.config.ts + scripts/coverage-exempt.ts + docs/testing.md (full), confidence=high, type=fact
- **(finding)**: ordinary tests cannot forget cross-cutting invariants — RegistryService is monkey-patched to auto-attach the invariant service + companions to every test root. source=scripts/test-invariants.ts (full), confidence=high, type=fact
- **(finding)**: EN/ZH docs are equal-authority pairs verified by structural-signature diff + git-blob-hash state files, with a fail-closed merge driver and ODB-stored recovery snapshots; the repo explicitly admits the gate's limit ("green gate ≠ sound confirmation"). source=verify-translation-pairing.ts + docs/i18n/README.md (full), confidence=high, type=fact
- **(finding)**: the release boundary is "credential-free pack from one commit"; publication is registry-decided and idempotent (missing→publish, identical→skip, differs→fail). source=scripts/release/pack.ts + publish.ts (headers), confidence=medium (headers), type=fact
- **(finding)**: agent notes are a closed-vocabulary lifecycle system (proposed/implemented/rejected, archived as i18n triplets under a SHA-256 frozen manifest), mandated by AGENTS.md for non-trivial changes. source=scripts/agent-note-tree.ts + archived-agent-notes.ts + AGENTS.md (full), confidence=high, type=fact
- **(finding)**: vendoring is governed end-to-end: scope-rescue with EXACT_EDITS hit counts, stage-time manifest pairing, lockfile link-resolution proof, and runtime-closure proof for the exe. source=rescue-vendor.ts (structure) + check-vendor-manifest.sh + verify-vendored-links.ts + verify-runtime-closure.ts, confidence=high, type=fact
- **(finding)**: dependency install scripts are deny-by-default (`allowBuilds`) with reviewed exceptions in pnpm-workspace.yaml. source=pnpm-workspace.yaml, confidence=high, type=fact
- **(finding)**: pre-commit regenerates THIRD_PARTY_NOTICES.md instead of rejecting stale copies — the hook fixes what it can prove. source=lefthook.yml (full), confidence=high, type=fact
- **(finding)**: the lefthook installer treats hook installation as a safety problem: exclusive-create pid+uuid install lock with dev/ino+record-verified release, stale locks require manual recovery (never auto-deleted), unowned hooks directories are never overwritten, git-config provenance (`--show-origin`) is traced before modifying anything, and the fail-closed merge driver must pass a tsx probe before registration. source=scripts/install-lefthook.mjs (lines 1–150 + lock/ownership sections), confidence=high, type=fact
- **(finding)**: knip.json contains no export-level suppression — test fixtures are declared as entries (teaching knip the surface), not ignored; dead-export hygiene is a hard gate (`pnpm run hygiene`). source=knip.json (sampled) + root AGENTS.md, confidence=medium (sampled, pattern-census based), type=fact
- **(finding)**: dsh does not accept external PRs at this stage (CONTRIBUTING.md); contribution is routed to Discussions/ecosystem plugins. The full verification platform therefore serves an internal team + agent operators — it is *agent-operability and small-team discipline*, not open-PR firehose defense. source=CONTRIBUTING.md (full), confidence=high, type=fact
- **(finding)**: `CLAUDE.md` symlinks `AGENTS.md` at three levels — one rule file serving multiple agent CLIs (the same problem vh solves with generated AGENTS.md + overlays). source=refs AGENTS.md ("Editing these instructions"), confidence=high, type=fact
- **(inference)**: local verification is deliberately minimal and evidence-matched ("match evidence to the surface… never default to the full suite"), with CI as the sole owner of exhaustive coverage — the inverse of a "run everything locally" culture, and directly relevant to vh's exec-verb ladder philosophy. source=root AGENTS.md + dsh-pre-push-checks skill ref, confidence=high, type=inference (intent stated in doc)
- **(finding)**: llm test support uses the persisted session JSONL itself as the replay fixture, plus a scriptable fault-injection OpenAI-compatible server (~25 wire behaviors). source=packages/test-support/llm-replay + llm-mock-server (headers/docs), confidence=medium, type=fact
- **(inference)**: the platform's unifying principle is *"verify the artifact, not the intention"* — built-lib smokes, registry-decided publish, generator-check duality, index-plane hooks, and topology-asserting CI all re-derive truth from a ground plane instead of trusting self-report. source=synthesis across full reads, confidence=medium, type=inference

---

## 6. Transferable ideas → vh-agent-harness

(a) **gen-*/verify-* self-verifying catalogs.** vh already generates catalogs (permissions, agents, skills) — dsh's pattern of *one script, two modes* (write vs `--check`) with byte-diff verification would let `vh-agent-harness doctor` verify rendered `.opencode/` against corpus without a second expectations file. Map: ownership classification + doctor `managed-drift` check.

(b) **CI-workflow-under-test.** `ci-workflow.spec.ts` asserting the workflow YAML's topology is directly transplantable: vh's own release/doctor CI could parse its workflow + `opencode.jsonc` and fail when permission surfaces or job topology drift. Map: generated+gated catalogs discipline.

(c) **Agent-note triplet + frozen-archive manifest ≈ vh session memory/checkpoint discipline.** dsh archives decision notes as content-frozen, hash-manifested, i18n-paired artifacts with per-lifecycle required sections. vh's `docs/checkpoints/` + session memory could adopt: closed lifecycle vocab, per-lifecycle section grammar, append-only SHA-256 archive manifest, and "non-trivial change must carry a note in the same PR" enforcement. Map: AGENTS.md discipline + checkpoint rules.

(d) **Registry-decided idempotent publish.** vh's release gate could adopt missing→publish / identical-integrity→skip / differs→fail so re-running a release is safe and un-bumped content changes fail loudly. Map: release gates (G0* checks).

(e) **Single-aggregate required check** (`all-checks-passed`, `if: always()` load-bearing): one stable required status across a wide fan of lanes — branch protection never goes stale when lanes are added/renamed. Map: vh CI topology.

(f) **allowBuilds deny-by-default.** pnpm's install-script policy is the npm-world analogue of vh's exec-verb ladder; the *reviewed-exceptions list in one visible file* is the transplantable part for any future JS tooling surfaces. Map: permission policy shape.

(g) **Doc budgets ratchet** (word ceilings per canonical doc; relocate → condense → raise, in that order; +5% headroom rule). Directly applicable to vh's AGENTS.md conciseness rules and `docs/ai/` growth. Map: AGENTS.md "authoritative and concise" + docs placement rules.

(h) **Coverage worker-budget splitting** (2/3 instrumented + 1/3 exempt-heavy from one max-workers budget) and per-mode concurrency caps: relevant to vh's `go test` fan-out and doctor parallelism. Map: Makefile/test runner.

(i) **Policy-as-code issue management.**vh's task-card lifecycle (`/write-task` → `/task-review`) already leans this way; dsh shows the fully-executable version — status↔closure coupling, loop-guarded automation actor, audit-comment upsert marker, and the policy's own test gate. Map: `.local/coordinator/` task registry semantics.

(j) **Pre-commit regeneration instead of rejection** (third-party notices): where vh's commit gate can *derive* the correct artifact (formatting, manifest ordering), regenerate-and-stage beats reject-and-correct — without weakening gates for underivable properties. Map: commit-gate.sh behavior.

*(Bonus items surfaced by top-up reads:)*
(k) **One rule file, many agent CLIs** — `CLAUDE.md` symlinks `AGENTS.md` instead of forking content per tool; vh already generates AGENTS.md but the symlink pattern applies wherever per-CLI rule files are demanded (e.g. `CLAUDE.md`, `GEMINI.md`). Map: AGENTS.md discipline.
(l) **Lock safety contract for shared repo state** — pid+uuid exclusive-create lock, dev/ino+record-verified release, stale→manual-recovery-never-auto-delete, unowned-target refusal. Directly applicable to vh's `.git/commit-gate.lock/` semantics. Map: gated-commit skill / commit-gate.sh.

*(These are synthesis recommendations, type=inference, grounded in the findings above; adoption decisions belong to a downstream debate/planner pass, not this packet.)*

---

## 7. Contradictions

- **None detected** within the gathered evidence: no read file contradicted another read file, and no dsh doc contradicted observed script behavior.
- **Prior-evidence correction** (not a dsh contradiction): the handoff described `install-lefthook.mjs` as "~120/31k lines"; the file is actually **845 lines** (~31KB) — a units mixup in the prior pass, corrected here.
- Noted **designed tensions** (explicit in dsh's own docs, not contradictions): per-file 100% coverage vs a large named exemption apparatus (resolved by "exemptions are budgeted debts with TODO markers"); structural-signature translation gate vs its admitted limit ("green gate ≠ sound confirmation"); fast local hooks vs heavy CI (resolved by "hooks are checkpoints only"); word-budget ceilings vs a documented raise-valve ("raise a `verify-doc-budgets` ceiling when the required content genuinely needs more space").

---

## 8. Coverage

| Area | Read state | Coverage |
|---|---|---|
| scripts/run-gates.ts gate system | full | ~95% |
| .github/workflows/ci.yml + ci-workflow.spec.ts | full | ~95% |
| wine-windows-gates.sh | full | ~95% |
| issue-management policy + tests | full | ~95% |
| lefthook.yml + installer | yml full; installer head + lock/ownership core (~300/845 lines) | ~65% |
| vitest config family + test-invariants.ts | full | ~95% |
| agent-note system (4 scripts + README + AGENTS rule) | full | ~95% |
| translation pairing (4 scripts + i18n README) | full | ~90% |
| doc gates | doc-typecheck/budgets/type-equiv full; md-* family existence+role; doc-site-fragments partial | ~70% |
| generators (12 gen-*) | headers/structure only | ~40% |
| packages/test-support | 2 of 6 subpackages (headers/docs) | ~35% |
| packages/typert | structure + role | ~45% |
| scripts/release/* | headers; 3 large bodies unread | ~45% |
| vendoring | rescue-vendor structure; 3 checks full | ~70% |
| meta-configs | oxlint/jscpd/editorconfig/gitattributes/pytest.ini/tsdown/pnpm-workspace read; knip.json sampled (head + pattern census) | ~75% |
| .gitlab-ci.yml | full | ~95% |
| docs (development/testing/AGENTS/i18n) | full | ~90% |
| root AGENTS.md | key sections re-confirmed this pass | ~80% |
| python/ sdk tests | pytest.ini + CI jobs only | ~25% |
| CONTRIBUTING.md | full (this pass) | ~95% |
| BENCHMARK | not read | 0% |

---

## 9. Open questions

1. ~~**knip.json rule inventory**~~ — **substantially answered this pass**: no export-level ignores; fixture-as-entry declarations dominate; remaining detail = per-workspace entry rationale for the ~150 declarations (low value).
2. ~~**install-lefthook.mjs full contract**~~ — **substantially answered this pass** (lock, ownership marker, extension guard, escape hatch, probe); remaining = uninstall/update paths in the unread tail (~545 lines).
3. **gen-translation-brief granularity rules**: exact decision tree from diff shape → splice/units/sections/whole-doc.
4. **Python smoke scenario coverage**: what `smoke-python-runtime.py` (39KB) actually exercises against the release-shaped exe.
5. **Unread release bodies**: `publish-npm-baseline.ts` (38KB), `build-exe-for-python-sdk.ts` (21KB) — how the npm baseline and the python exe build relate to the families/pack/publish spine.
6. **verify-doc-site-fragments** full mechanics (headers-from-specs gathering).
7. **BENCHMARK doc** content: what runner-benchmark suites measure and how results gate (or inform) infra choices.

---

## 10. Next-step routing

- Durable artifact: this packet → promote to `researches/sources/deepseek-harness/engineering-platform.md` (operator/commit slice; this session is write-fenced to `tmp/**`).
- If transferable ideas (a)–(j) are to be pursued: route the shortlist to a `debate` pass with this packet as evidence base, then `planner` for adoption slices targeting doctor/commit-gate/docs-sync surfaces.

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the engineering-platform gap-fill addendum (all 7 OQ answers, 8 gen-*
body mechanisms, test-support reads, all 5 trailers banked). Method
unchanged: static reading only, `refs/` never modified.

### Open-question resolutions

- **OQ 1 (knip.json)** — full read (787 ln): 8 `ignoreBinaries`
  platform tools; ~60 per-workspace stanzas whose dominant pattern
  declares TEST TREES AS ENTRY POINTS (reachability starts at tests);
  only `examples` carries ~35 named fixture entries; per-workspace
  `ignoreDependencies` narrow and role-explained; ZERO export-level
  ignores anywhere; root workspace declares `scripts/**` as entries.
- **OQ 2 (install-lefthook tail)** — tail read in full (300-845); NO
  uninstall path (install-only; no-op on CI / missing bin / non-git).
  New mechanics: lock PUBLICATION check (re-stat the published inode;
  release re-verifies, ENOENT = ownership-changed); bounded
  incomplete-record tolerance then manual recovery; dead-owner locks
  never auto-delete; hardlink-aware (`nlink !== 1` refused);
  `core.hooksPath` negotiation (command/worktree-scoped refused);
  merge-driver install write-verify-rollback; postconditions re-derived
  from effective state; `GIT_CONFIG_*` stripped from child env.
- **OQ 3 (gen-translation-brief)** — full reads
  (`scripts/translation-brief.ts` 512 ln + `gen-translation-brief.ts`
  302 ln). Decision tree narrowest-first: both-sides-drift → `document`
  ("no side is a trustworthy mapping anchor"); mechanical only when all
  changes are inside code fences AND masked prose byte-identical;
  units = positional alignment on language-neutral kinds (heading DEPTH
  only); sections = heading-delimited spans; fallback → `document` with
  reason. Extras: first-occurrence tracking; `--apply` re-validates
  against the structure signature before writing; last-confirmed text
  from git blob hashes in `.i18n.yaml`.
- **OQ 4 (python smoke)** — full read of
  `scripts/smoke-python-runtime.py` (970 ln): five keyless scenarios
  against a mock SSE model — sdk-default (zstd magic `28b52ffd`),
  sdk-custom (exe + hand-written cordis.yml; `run_code` 6*7=42;
  workflow), sdk-minimal (persistent-bash PTY state; advertised tool set
  asserted EXACTLY {bash, str_replace_editor}), sdk-snapshot
  (`cordis_define` → dynamic tool → subagent → workflow →
  `cordis_undefine`; 4 golden files under volatile normalization;
  `--update-snapshots` only "after reviewing the behavior"), direct
  (raw JSON-RPC stdio). Cross-cutting: the mock asserts on the
  MODEL-REQUEST plane — required absences included.
- **OQ 5 (release bodies)** — both fully read; a SEPARATE lane from the
  `scripts/release/` families spine. `scripts/publish-npm-baseline.ts`
  (1083 ln): commit-addressed baseline (version via `git show
  commit:package.json`); packing inside a DETACHED WORKTREE in mkdtemp
  ("the caller's checkout is never touched"); tarball validation (no
  `workspace:` protocol, exact-pinned internals); throwaway-consumer
  smoke under a SCRUBBED env incl. a PTY probe booting `dsh web --port
  0`; idempotent publish (missing→publish / identical→skip /
  differs→fail). `scripts/build-exe-for-python-sdk.ts` (527 ln):
  pkg `--sea` executables + the Python node carrier; symlink-free staged
  closure; whole-tree assets (runtime bare-imports defeat static
  analysis); node-pty built on the target arch (cross-compile refused);
  `CI: 'true'` children ("must not mutate or validate a developer's Git
  hooks").
- **OQ 6 (verify-doc-site-fragments)** — full read (157 ln): NOT
  header-gathering — artifact-plane verification of the BUILT VitePress
  HTML via JSDOM ("Markdown and VitePress use different heading-slug
  algorithms, so source-link validation alone cannot prove that a
  published fragment exists"); runs as part of `docs:build`.
- **OQ 7 (BENCHMARK.md)** — full read (3 lines): NOT about the CI runner
  suites — a user-facing pointer to the Python SDK guide; the packet's
  expectation has no referent in this file.

### New findings

- knip's test-surface policy is "declare tests as entries"; no
  export-level suppression exists (`knip.json` full).
- The translation brief chooses granularity by ALIGNMENT SOUNDNESS, not
  diff size (kind-sequence 1:1 across all three states or escalate
  coarser).
- Code-fence-only translation changes are fully mechanical; `--apply`
  output passes the verifier before landing.
- Doc fragments verified on the built-artifact plane — "verify the
  artifact, not the source".
- **gen-cordis-catalog**: three-way fail-closed partition (projection vs
  curated maps vs independent AST scan); rendered-but-undeclared = "the
  scan itself regressed — fix the scan, not the maps"; regions spliced
  byte-identically into BOTH language sides; `.i18n.yaml` re-recorded
  only on region-confined writes (`scripts/gen-cordis-catalog.ts`).
- **gen-config-catalog**: AST classification mirroring the Loader;
  verbatim pastes with identity-checked names; three-valued path lookup
  cross-checks the runtime schema ("the paste cannot hide a
  loader-accepted field"); JSDoc enforced on nested properties.
- **gen-tool-catalog**: the one runtime-harvest generator (fresh Context
  per package, disposed in `finally`); `assertToolsHarvested` treats
  zero tools as a BROKEN boot; all-rejecting `CatalogAttachmentStore`
  seam marker; rejecting-start subagent provider = full capability set;
  per-child tools via child scope.
- **gen-persistence-catalog**: DUAL artifact — docs catalog AND the
  runtime module `known-event-types.ts` the session-read unknown-type
  refusal checks against; exactly one owning `SessionEventMap`, no
  `extends`, the JSDoc IS the catalog entry; `SurfaceEventType` closed
  union declared once.
- **gen-client-catalog**: lexical scan (deliberately NO type-checker);
  duplicate slots fail; JSDoc from the REGISTRANT's side; per-slot
  120-line report budget ("a report a model cannot finish reading is a
  defect rather than a detail"); `replaceRisk: shadows-shipped-ui`.
- **gen-scoped-events**: TypeChecker-backed; resolver derived from
  exactly-one identical-type payload (zero → explicit `@dshScopeScan
  unsupported`; multiple → fail; unnecessary tag → fail).
- **gen-doc-graphs**: labels its own epistemic mode per page
  (`generated` / `hybrid generated` / `curated`) with bidirectional
  completeness guards; event names only from finite literal sets; a
  zero-dispatcher event fails as "dead vocabulary... teach
  scripts/gen-doc-graphs.ts that form".
- **agent-loop-testkit**: mounts five prerequisites and "deliberately
  does not mount AgentLoop or register an adapter" (tests own the
  topology); its explained-empty companion proves the `./invariant` rule
  reaches test-support packages.
- **loader-smoke**: `resolveExampleMode` fails loud on env typos;
  dual-mode src/lib resolver shared by every subprocess harness;
  isolated-cwd runs embedding both streams in failures;
  `expectedExitCode` designed-failure pinning ("including succeeding —
  still fails the smoke"); `runFixtureTurn` = inbox-receipt-to-idle
  driver (exactly one top-level agent; durable spliced event = turn
  start; completion = whole-agent idle).
- **acp-snapshot**: four-layer keyless architecture; deterministic
  InputStep script against the REAL agent bin via the cordis Loader
  ("see docs/postmortem/0001"); durable-state wait vocabulary (child
  progress read from the child's persisted JSONL); exactly one scenario
  per header-composition class; `recorded` vs `authored` (never
  re-recorded); pure normalizers preserving seqs.
- **client-runtime (test-support)**: "real where the contract lives,
  doubled only at service faces" — production Context/SlotRegistry/
  renderer mounted; test doubles implement the production FACES so "a
  production face change breaks this double at compile time"; DOM
  snapshot hygiene (CSS-module + svg fingerprints).
- **gen-third-party-notices**: license policy as a GENERATION GATE —
  fail-closed SPDX evaluation; non-permissive RUNTIME deps hard-fail
  ("a distribution decision, not a rendering detail"); metadata from the
  INSTALLED store; pre-commit regeneration + spec-asserted committed
  bytes.

### Transferable ideas added

(new/updated relative to packet (a)-(l)) — **(m)** alignment-soundness
granularity ladder (never truncate or guess; escalate coarser); **(n)**
assert on the model-request plane in mocks (advertised tools incl.
required absences); **(o)** detached-worktree packing (identity fixed
before expensive work; throwaway tree only); **(a-update)** gen/verify
duality now uniform across the 12-script fleet — "a generator's output
is a candidate until the paired verifier passes" (9 of 12 read in
depth); **(l-update)** full lock-safety contract (inode re-stat, bounded
tolerance, never auto-delete, write-verify-rollback).

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| generator fleet (12 gen-*.ts) | ~55% | ~95% (9/12 in depth + verify-doc-site-fragments) |
| test-support | ~35% | ~70% (testkit + loader-smoke full; others at header/export depth) |
| knip.json | partial | 100% (787 ln) |
| install-lefthook.mjs | head 300 | 100% (845 ln) |
| doc gates | partial | ~90% |
| release bodies (baseline + exe) | 0 | ~85% (both full; families spine packet-level) |
| BENCHMARK.md | 0 | 100% |
| python smoke | 0 | ~90% (golden snapshots unread) |

### Contradictions and corrections from this pass

- None detected. All prior-session claims re-checked verified verbatim
  (testkit mount contract + explained-empty companion; loader-smoke
  resolver/isolation/pinning/inbox-receipt driving).
- One nuance, not a contradiction: BENCHMARK.md does not cover the CI
  runner-benchmark suites (OQ 7) — an answered expectation-mismatch, not
  a repo inconsistency.
