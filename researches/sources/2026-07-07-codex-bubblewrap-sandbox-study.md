# Sources: OpenAI Codex Bubblewrap (`bwrap`) Linux Sandbox — Reference Study for `exec-sandbox`

**Date:** 2026-07-07
**Topic:** How `openai/codex` (the open-source Codex CLI agent, Rust workspace)
implements its Linux sandbox on top of Bubblewrap (`bwrap`), and what that
implies for our planned `vh-agent-harness exec-sandbox` feature (currently
briefed as a Go-native Landlock+seccomp trampoline).
**Kind:** Source/option packet. NOT active repo guidance — a reference study
plus an options comparison and one opinionated recommendation, intended for a
`planner` revision of the `exec-sandbox` brief.
**Studied against (our side):** `vh-agent-harness` repo at git `f9fd1c9`
("chore(dogfood): bump adopted lineage ref to v0.4.0"). No `exec-sandbox`
design doc exists yet; only the L1 `exec-ro` heuristic gate is implemented
(`internal/execro/classifier.go`, `internal/cli/exec_ro.go`), allowlisted in
`opencode.jsonc` as `vh-agent-harness exec-ro *: allow`.

---

## Research question & scope

- **Question:** How does Codex build and enforce its Linux sandbox with
  Bubblewrap, and which of its techniques should we borrow for
  `exec-sandbox` — pure bwrap shelling, Go-native Landlock+seccomp, a hybrid,
  or selectable backends?
- **Scope:** Codex's Linux sandbox codepath only (macOS Seatbelt and Windows
  Restricted-Token paths covered for completeness, not for adoption). The
  comparison is against our current `exec-sandbox` brief (Go-native
  Landlock+seccomp trampoline) and our existing `exec-ro` L1 layer.
- **Time-sensitivity:** MIXED. Codex source is a moving target (branch
  `master`/`main`, no commit SHA pinned — see "Version anchor" below); the
  *mechanisms* (bwrap namespaces, seccomp, Seatbelt) are STABLE kernel/OS
  primitives. Re-verify exact argv/flag counts against a pinned SHA before
  any code lands.
- **Source policy:** PRIMARY = `openai/codex` source read directly via the
  repo tools (zread file reads + doc search). Codex's `docs/sandbox.md` and
  `docs/exec.md` are STUBS (they only link out to
  `developers.openai.com/codex/security` and `/noninteractive`), so the source
  is the real truth, not those docs. Secondary web context used only to
  confirm high-level design intent.

## Confidence legend

- **HIGH** — verified directly against Codex source (file + function + quoted
  Rust); the mechanism is unambiguous.
- **MED** — single-source claim, behavioral inference from source structure,
  or derived from a doc-search summary rather than a re-read line.
- **LOW** — anecdotal / secondary; directionally useful only.

## How citations are formatted

Codex is Rust and zread does not return stable line numbers; files are read as
single JSON-escaped lines. Citations are therefore **file + function name +
short quoted code**, marked `fn-level` rather than `file:line`. Where a web
source gave a line number it is included. A short re-verification pass against
a pinned SHA is recommended before any adoption (see "Version anchor").

---

## Codex sandbox module map (where to look)

- `codex-rs/linux-sandbox/` — THE primary Linux module. Produces a standalone
  `codex-linux-sandbox` executable and a lib exposing `run_main()`. Key files:
  `src/{bwrap.rs, landlock.rs, lib.rs, linux_run_main.rs, proxy_routing.rs,
  launcher.rs, bundled_bwrap.rs, bazel_bwrap.rs, exec_util.rs}` + `README.md`
  + `tests/`.
- `codex-rs/sandboxing/` — cross-platform abstraction (`SandboxType`,
  `PermissionProfile`, transforms). Files: `src/{manager.rs, bwrap.rs,
  landlock.rs, seatbelt.rs, seatbelt_base_policy.sbpl,
  seatbelt_network_policy.sbpl, restricted_read_only_platform_defaults.sbpl,
  windows.rs, policy_transforms.rs, denial.rs}`.
- `codex-rs/{process-hardening/, execpolicy/, exec/, network-proxy/,
  windows-sandbox-rs/}` — supporting layers.
- `docs/sandbox.md`, `docs/exec.md` — STUBS only.

---

## Findings

### F1 — Exact `bwrap` argv Codex constructs  (HIGH, `fn-level`)

> **(finding)**: Codex builds two distinct bwrap argvs and a skip path, source=..., confidence=high, type=fact

Source: `codex-rs/linux-sandbox/src/bwrap.rs`.

- **`create_bwrap_flags`** (restricted-FS path). After `argv[0]` it appends:
  `[--new-session, --die-with-parent, <filesystem_args...>, --unshare-user,
  --unshare-pid, (if network != FullAccess) --unshare-net, (if mount_proc)
  --proc /proc, (if cwd normalized) --chdir <cwd>, --, <command...>]`.
- **`create_bwrap_flags_full_filesystem`** (full-disk-write but still
  network-restricted path):
  `[--new-session, --die-with-parent, --bind / /, --unshare-user,
  --unshare-pid, (if network != FullAccess) --unshare-net, (if mount_proc)
  --proc /proc, --, <command...>]`. Note: full disk write uses a writable
  bind of the whole root (`--bind / /`), not `--ro-bind`.
- **`create_bwrap_command_args`** dispatcher: if full-disk-write AND no
  unreadable-globs AND `FullAccess` network → returns the **raw command with
  bwrap SKIPPED entirely**. Otherwise delegates to one of the two builders.

**What Codex does NOT use** (verified by a `--flag` census over `bwrap.rs`):
no `--unshare-all`, no `--unshare-ipc`, no `--unshare-uts`, no `--hostname`,
no `--setenv`, no `--dev-bind`. Isolation is **selective**: explicit
`--unshare-user` + `--unshare-pid` (+ conditional `--unshare-net`).
`--new-session` and `--die-with-parent` are **always present** (lifecycle:
new session, dies with parent).

Flags observed in `bwrap.rs` (approx occurrence counts): `--unshare-user`×3,
`--unshare-pid`×3, `--unshare-net`×3, `--new-session`×3, `--die-with-parent`×3,
`--proc`×4, `--dev`×7, `--tmpfs`×21, `--bind`×16, `--ro-bind`×15,
`--remount-ro`×13, `--ro-bind-data`×6, `--dir`×5, `--perms`×22, `--files`×3,
`--chdir`×3.

> **(finding)**: Codex deliberately does NOT use --unshare-all; it opts into namespaces one at a time (user+pid always, net conditionally), source=..., confidence=high, type=fact

### F2 — Filesystem isolation model  (HIGH, `fn-level`)

Source: `codex-rs/linux-sandbox/src/bwrap.rs`, fn `create_filesystem_args`.

- **Default = read-only root.** Full-disk-read mode → `[--ro-bind / /, --dev
  /dev]`: the entire `/` is bind-mounted read-only, and a minimal `/dev` is
  materialized by bwrap's setup-dev (null/zero/full/random/urandom/tty).
  `/dev` is mounted BEFORE writable roots so that explicit writable `/dev/*`
  binds remain visible.
- **Restricted-read mode** (no full disk read): `[--tmpfs /, --dev /dev]`,
  then only approved readable roots are layered with `--ro-bind <root> <root>`,
  optionally plus `LINUX_PLATFORM_DEFAULT_READ_ROOTS` (system libs, Nix store)
  when `include_platform_defaults()` is set.
- **Writable roots** are layered with `--bind <root> <root>`. **Protected
  subpaths** (`.git`, resolved `gitdir:` targets, `.agents`, `.codex`) are
  re-applied **read-only** via `--ro-bind` even inside a writable root — so a
  writable workspace cannot mutate its own repo history / agent state.
- **Overlap resolution:** split-policy entries are applied in path-specificity
  order (narrower writable children re-open broader read-only/denied parents;
  narrower denied subpaths still win over broader writable parents). This is
  the load-bearing ordering rule that makes "writable repo except `.git`"
  actually hold.
- **Unreadable globs** are expanded via a ripgrep-style pass
  (`rg --files --hidden --no-ignore --glob …`) with an internal-walker
  fallback, then **masked** in bwrap by mounting `/dev/null` over the
  symlink/missing-component paths. `glob_scan_max_depth` is configurable.
- **Writable-root bind targets must exist** (missing roots are skipped, for
  cross-platform config tolerance).

> **(finding)**: Codex's writable-scratch model is layered binds under a read-only root, with .git/.agents/.codex forcibly re-read-only'd; this is the directly borrowable pattern for our repo + ./tmp/ convention, source=..., confidence=high, type=fact

**Implication for our `./tmp/` convention:** Codex's "ro-bind `/`, bind
writable roots, re-ro-bind protected subpaths" maps cleanly to "ro-bind repo,
bind `./tmp/`, re-ro-bind `.git`." The protected-subpath re-ro-bind is the
technique that prevents an agent in a writable workspace from rewriting its
own guardrails.

### F3 — Network handling  (HIGH, `fn-level`) — LOAD-BEARING for us

Sources: `codex-rs/linux-sandbox/src/bwrap.rs` (`BwrapNetworkMode`,
`should_unshare_network`); `codex-rs/linux-sandbox/src/landlock.rs`
(`install_network_seccomp_filter_on_current_thread`);
`codex-rs/linux-sandbox/src/proxy_routing.rs`;
`codex-rs/linux-sandbox/src/linux_run_main.rs`.

- **Tri-state at the bwrap builder:** `BwrapNetworkMode::{ FullAccess
  (default), Isolated, ProxyOnly }`. `should_unshare_network =
  !matches!(self, FullAccess)` → at the **bwrap level the decision is
  binary**: any non-`FullAccess` mode adds `--unshare-net` (full network
  namespace kill). `FullAccess` omits it (network allowed).
- **Managed-proxy mode (`ProxyOnly`):** ALSO `--unshare-net`, then a
  **TCP→UDS→TCP bridge** (`proxy_routing.rs`). Host side `spawn_host_bridge`
  connects to the real loopback proxy endpoint and serves a Unix domain
  socket; inside the netns, `spawn_local_bridge` listens on loopback and
  forwards to that UDS; proxy env vars (`HTTP_PROXY`/`HTTPS_PROXY`/etc., ~14
  keys) are rewritten to point at the local bridge port. Net effect: only the
  configured proxy endpoints are reachable. This is how Codex does
  "network-but-only-to-allowed-hosts" without allowing raw egress.
- **Seccomp is a SECOND network gate**, applied in-process after bwrap sets up
  the namespace (see F4/F9). Two modes:
  - **Restricted:** denies `connect`/`accept`/`accept4`/`bind`/`listen`/
    `getpeername`/`getsockname`/`shutdown`/`sendto`/`sendmmsg`/`recvmmsg`/
    `getsockopt`/`setsockopt`; allows only `AF_UNIX` `socket`/`socketpair`.
  - **ProxyRouted:** allows `AF_INET`/`AF_INET6` `socket` (to reach the local
    bridge), denies other `socket()` families; allows `AF_UNIX`
    `socketpair` only.
  - **Both modes ALWAYS deny:** `ptrace`, `process_vm_readv`/`writev`,
    `io_uring_setup`/`enter`/`register` (anti-escape / anti-inspect).
- **The "ask" tri-state does NOT live in the sandbox.** At the sandbox layer
  `NetworkSandboxPolicy` is observed as `{Enabled (full kill), Restricted}`
  — binary. "Ask" lives in Codex's **Guardian/approval + execpolicy**
  pipeline (operator approves a command; the approval carries the network
  policy the sandbox then enforces). So for Codex, **network "allow" is an
  approval decision, not a sandbox flag.**

> **(finding)**: Codex's network model is binary at the sandbox (kill via --unshare-net, or allow); selective egress is achieved by a userspace TCP→UDS→TCP proxy bridge, NOT by per-host firewall rules. The "ask" lives in the approval layer, not the sandbox., source=..., confidence=high, type=fact
> **(finding)**: For our network tri-state (deny/allow/ask), the borrowable Codex mechanic is: sandbox enforces deny (--unshare-net) or allow (no flag); "ask" must be a separate approval/decision layer. Selective egress (npm to a registry only) would require a proxy-bridge like Codex's, which is a large addition., source=..., confidence=med, type=inference

### F4 — bwrap + seccomp composition (Landlock is UNUSED)  (HIGH, `fn-level`)

Sources: `codex-rs/linux-sandbox/src/lib.rs`, `.../landlock.rs`.

- Codex **composes three mechanisms**: (a) **bwrap** for **filesystem**
  namespace isolation; (b) **seccomp in-process** for network/syscall
  restriction; (c) `PR_SET_NO_NEW_PRIVS`.
- **Landlock filesystem code EXISTS but is EXPLICITLY UNUSED.** The
  `landlock.rs` fn `install_filesystem_landlock_rules_on_current_thread`
  carries the comment: *"currently unused because filesystem sandboxing is
  performed via bubblewrap. It is kept for reference and potential fallback
  use."* It is reachable only behind the legacy feature flag
  `features.use_legacy_landlock=true`.
- The `landlock.rs` module doc states: *"Filesystem restrictions are enforced
  by bubblewrap in linux_run_main."* Only `no_new_privs` + the network
  seccomp filter are applied in-process.
- **Threat model (synthesized from the denied-syscall set + README):** confine
  the filesystem via namespace mounts; kill or proxy-bridge network via netns
  + seccomp; block `ptrace`/`process_vm`/`io_uring` to prevent escape and
  cross-process inspection; `no_new_privs` to prevent setuid escalation.

> **(finding)**: Codex chose bwrap (namespaces) for FS and seccomp for syscalls, and RETIRED Landlock FS to a legacy/fallback role. This is strong precedent that namespaces+seccomp are sufficient and Landlock-FS was not worth maintaining alongside bwrap., source=..., confidence=high, type=fact

### F5 — Privilege model  (HIGH, `fn-level`)

Sources: `codex-rs/linux-sandbox/src/{launcher.rs, bundled_bwrap.rs, bwrap.rs}`,
comment in `landlock.rs`.

- `--unshare-user` is requested **explicitly** rather than relying on bwrap's
  auto-enable — because bwrap skips auto user-namespace when the caller runs
  as uid 0. The explicit flag lets Codex run sandboxed even as **root inside a
  container** without needing ambient `CAP_SYS_ADMIN`.
- `PR_SET_NO_NEW_PRIVS` is **deliberately avoided** unless seccomp (or legacy
  Landlock) is actually being applied, because many bwrap deployments rely on
  a **setuid** bwrap binary and `NO_NEW_PRIVS` would interfere with that.
- **No sudo-prompt or setcap code** was found in these modules. The reliance
  is on unprivileged user namespaces (bwrap being setuid, or the kernel
  allowing unprivileged userns). There is no auto-`sudo` escalation path.
- **WSL1 is rejected** (`is_wsl1()` → error
  `Wsl1UnsupportedForBubblewrap`) because WSL1 cannot create user
  namespaces. WSL2 takes the normal path.

> **(finding)**: Codex assumes unprivileged-userns availability (setuid bwrap or kernel userns); it does NOT prompt for sudo and does NOT install capabilities. Distros that fully disable unprivileged userns will fail Codex's sandbox too., source=..., confidence=high, type=fact

### F6 — Feature detection & "graceful skip"  (MED, `fn-level` + README)

Sources: `codex-rs/linux-sandbox/src/{launcher.rs, bundled_bwrap.rs}`,
`linux-sandbox/README.md`.

- `preferred_bwrap_launcher()` (OnceLock-cached) selection order:
  1. `find_system_bwrap_in_path()` (from `codex-rs/sandboxing`) — first bwrap
     on PATH **located outside the CWD** (security: avoid a repo-planted
     binary).
  2. If none/unsuitable → `bundled_bwrap::launcher()` (the bwrap shipped at
     `codex-resources/bwrap`).
  3. Else → `Unavailable` → **panic** ("bubblewrap is unavailable").
- **Capability probe** `system_bwrap_capabilities`: runs `<bwrap> --help`,
  scans stdout/stderr for `--argv0` (added in bwrap v0.9.0) and `--perms`.
  Older bwrap without `--argv0` takes a compatibility path; **missing
  `--perms` rejects the system bwrap** (forces the bundled one).
- **README-stated behavior:** bwrap missing → fall back to the bundled bwrap
  + emit a startup WARNING; userns cannot be created → startup warning; WSL1
  → reject sandboxed commands *before* invoking bwrap.

> **(finding)**: Codex's behavior on missing bwrap is WARN+FALLBACK (to a bundled bwrap), NOT graceful-skip-to-unsandboxed. When the sandbox is genuinely required and unavailable, it refuses/panics rather than running unsandboxed. This is a POLICY DIFFERENCE from our brief's "graceful skip if unsupported" — Codex prefers refusing over unsandboxed execution., source=..., confidence=med, type=inference

### F7 — Bundled bwrap shipping  (HIGH, `fn-level`) — KEY for our "single static binary" tension

Source: `codex-rs/linux-sandbox/src/bundled_bwrap.rs`.

- **Codex does NOT embed bwrap inside its own binary.** It ships a
  **separate bwrap binary** at `codex-resources/bwrap` (npm layout:
  `vendor/<triple>/codex-resources/bwrap`), discovered via
  `InstallContext::bundled_resource("bwrap")` or legacy candidates placed
  next to the executable.
- The bundled bwrap is **SHA256-verified at exec time**
  (`CODEX_BWRAP_SHA256` build-time env → `verify_digest`) and is exec'd via
  `/proc/self/fd/<fd>` (so the on-disk path can't be swapped after open).
- This is exactly the point of friction for us: Codex accepts an **external
  binary dependency** and ships it alongside; we have a "single static Go
  binary, no host deps" identity. (See Comparison C1 and the policy call.)

> **(finding)**: Codex treats bwrap as an external, separately-shipped, hash-pinned binary — not as an embedded library. Adopting Codex's model verbatim would break our "single static Go binary, no host deps" identity unless bwrap is treated as an OPTIONAL, feature-detected runtime (system bwrap on PATH, or operator-installed)., source=..., confidence=high, type=fact

### F8 — macOS path (Seatbelt) & Windows  (MED, sbpl file + search_doc)

Sources: `codex-rs/sandboxing/src/seatbelt.rs`,
`seatbelt_base_policy.sbpl`, `seatbelt_network_policy.sbpl`, `manager.rs`.

- macOS uses **Apple `sandbox-exec` (Seatbelt)** with composed SBPL profiles.
  The path is **hardcoded to `/usr/bin/sandbox-exec`** (NOT a PATH search —
  security against binary injection), per a `search_doc` note.
  `SandboxType::MacosSeatbelt`; `create_seatbelt_command_args` prepends
  `/usr/bin/sandbox-exec` + composed SBPL + the original command.
- Base policy `seatbelt_base_policy.sbpl` is **closed-by-default**
  `(deny default)`, then allows process-exec/fork, same-sandbox signals,
  specific sysctls, mach-lookups, PTYs, ipc-posix-sem/shm. **Mirrors the
  Linux semantics** (ro-by-default FS, writable roots layered, network via
  policy). `seatbelt_network_policy.sbpl` controls network.
- Windows: `SandboxType::WindowsRestrictedToken` (codex-windows-sandbox:
  Restricted Token + ACL + WFP firewall). The Linux tag is `LinuxSeccomp`
  (despite being bwrap+seccomp).

> **(finding)**: Codex is a multi-backend sandbox (bwrap on Linux, Seatbelt on macOS, Restricted-Token on Windows) behind one cross-platform PermissionProfile abstraction. The pattern (one policy model, multiple enforcement backends) is directly relevant if we ever go cross-platform — but we are Linux-first, so this is completeness., source=..., confidence=med, type=fact

### F9 — Orchestrator: two-stage re-exec  (HIGH, `fn-level`)

Source: `codex-rs/linux-sandbox/src/linux_run_main.rs`, fn `run_main`.

- Codex uses a **two-stage re-exec** because seccomp must be applied
  in-process *after* bwrap has constructed the namespace:
  - **Outer stage:** bwrap constructs the FS view, then re-execs
    `codex-linux-sandbox` **inside** the namespace
    (`build_inner_seccomp_command`).
  - **Inner stage** (`apply_seccomp_then_exec`): activates proxy routes if
    needed, calls `apply_permission_profile_to_current_thread`
    (`no_new_privs` + seccomp, `apply_landlock_fs=false`), then execs the
    user command.
- The **full-disk-write + no-proxy + FullAccess-network** path skips bwrap
  entirely, applies seccomp in-process directly, and execs.

> **(finding)**: The reason Codex needs a re-exec (and a dedicated codex-linux-sandbox helper binary) is that bwrap sets up namespaces via exec while seccomp is applied in-process to the running thread. A Go-native trampoline that applies Landlock+seccomp in-process BEFORE exec would NOT need this re-exec — an architectural simplicity win for the Go-native approach., source=..., confidence=high, type=inference

### F10 — UX / profiles / config  (MED, manager.rs + README + search_doc)

- **Sandbox on/off tri-state:** `SandboxablePreference::{Auto, Require,
  Forbid}` — operator choice for whether to sandbox at all (NOT network).
- **Profile model:** `PermissionProfile` → `(FileSystemSandboxPolicy,
  NetworkSandboxPolicy)`. The legacy `SandboxPolicy` enum (`WorkspaceWrite`
  etc.) is still supported; `compatibility_workspace_write_policy` maps it to
  `writable_roots` + `network_access`.
- **Config keys:** `-c use_legacy_landlock=true` /
  `features.use_legacy_landlock`; `permissions.workspace.filesystem` with
  `:workspace_roots`, `glob_scan_max_depth`; per-root deny e.g.
  `[permissions.workspace.filesystem.":workspace_roots"] "**/*.env" = "none"`.
  `--no-proc` is available for restrictive containers that deny `--proc
  /proc`.
- **Network "allow" is an approval/execpolicy decision**, not a sandbox flag.

---

## Contradictions & stale signals

<!-- Codex's own docs vs source -->
- **Docs vs source:** `docs/sandbox.md` and `docs/exec.md` are stubs that link
  out; they under-document the actual mechanism. Anyone reading only the docs
  would miss the bwrap+seccomp composition, the proxy bridge, and the
  Landlock-is-unused fact. → Treat the SOURCE as canonical, which this packet
  does.
- **Landlock presence vs Landlock use:** `landlock.rs` contains a full FS
  Landlock implementation, which could mislead a reader into thinking Codex
  uses Landlock for FS. It does NOT — it's explicitly unused, kept "for
  reference and potential fallback." (HIGH, direct source comment.)
- **`LinuxSeccomp` tag vs reality:** the cross-platform `SandboxType` variant
  for Linux is named `LinuxSeccomp`, but Linux enforcement is bwrap
  (namespaces) + seccomp, with Landlock FS off. Naming is stale relative to
  the implementation.
- **"Graceful skip" semantics vs Codex's actual behavior:** our brief says
  `exec-sandbox` should "graceful skip if unsupported"; Codex instead
  WARN+FALLBACK to a bundled bwrap, and refuses/panics when a required
  sandbox is truly unavailable. This is a **policy divergence to flag for the
  operator** (see Recommendation policy-call #1).

None of these block adoption; they are framing caveats.

---

## Comparison: Codex Bubblewrap model vs Go-native Landlock+seccomp trampoline

Our constraints (from the brief + AGENTS.md identity):
- (C-LIGHT) lightweight + unprivileged + kernel-enforcing.
- (C-STATIC) cgo-free **single static Go binary** (`cmd/vh-agent-harness/`).
- (C-SKIP) graceful skip if unsupported.
- (C-LAYER) composes with our existing `exec-ro` L1 heuristic layer.
- (C-NET) network tri-state (deny / allow / **ask**).
- (C-MAINT) low maintenance surface (host deps vs `go.mod` libs).

### C-STATIC — single static Go binary, no host deps
- **Bubblewrap (Codex):** WEAK. bwrap is an external binary. Codex ships a
  separate pinned bwrap next to its binary; if we mirror that, we either
  (a) vendor a bwrap binary into our release (breaks "single static binary,
  no host deps"), or (b) require a system bwrap on PATH (host dep).
  Codex's `/proc/self/fd` + SHA256 pin shows this is taken seriously but it
  is still an external dependency.
- **Go-native Landlock+seccomp:** STRONG. Pure Go (e.g.
  `landlock`/`seccomp-go`-style libs), compiles into the one static binary,
  zero host deps. Matches our identity directly.
- **Verdict:** Go-native wins decisively on our defining constraint.

### C-LIGHT — lightweight + unprivileged + kernel-enforcing
- **Bubblewrap:** STRONG. Kernel-enforcing namespaces, unprivileged via
  userns/setuid-bwrap. Well-trodden.
- **Go-native:** STRONG (Landlock + seccomp are kernel-enforcing and
  unprivileged; `no_new_privs` is trivial). Slightly more code to write but
  no heavier at runtime.
- **Verdict:** tie. Both satisfy this fully.

### C-SKIP — graceful skip if unsupported
- **Bubblewrap:** MIXED. Codex WARN+FALLBACKs to a bundled bwrap and REFUSES
  when truly unavailable — i.e. it does NOT skip to unsandboxed. To match
  our "graceful skip" we'd have to add our own skip policy on top (our
  `exec-ro` already provides a fallback gate).
- **Go-native:** STRONG. We control detection (Landlock supported? seccomp
  available?) and can degrade cleanly to `exec-ro`/unsandboxed with a warn.
  Landlock has a clean "unsupported → no-op with ENOSYS/EPERM" story.
- **Verdict:** Go-native fits our skip semantics more naturally; Codex's
  model would need a policy override to skip instead of refuse.

### C-LAYER — composes with existing `exec-ro`
- **Bubblewrap:** STRONG but at a different layer. `exec-ro` is a
  read-only *intent* classifier; bwrap is a *mount-namespace* enforcer. They
  compose well (exec-ro decides "this should be read-only", bwrap enforces
  it at the FS level). Codex's protected-subpath re-ro-bind (F2) is exactly
  the enforcement exec-ro currently only heuristically claims.
- **Go-native:** STRONG. Landlock FS restrictions compose identically with
  exec-ro's intent, and Landlock's per-path access modes (ro/rw/noaccess)
  map 1:1 onto exec-ro's classification output.
- **Verdict:** tie; both compose. Go-native's per-path modes align slightly
  better with exec-ro's existing classifier output.

### C-NET — network tri-state (deny / allow / ask)
- **Bubblewrap:** PARTIAL. At bwrap level it's **binary**
  (`--unshare-net` kill vs allow). Selective egress requires Codex's
  TCP→UDS→TCP proxy bridge (F3) — a substantial addition. The **"ask" is NOT
  a sandbox feature** in Codex; it lives in the approval layer.
- **Go-native:** PARTIAL (same shape). deny = seccomp-block socket/connect;
  allow = no filter; selective egress (npm to one registry) is NOT natively
  expressible in seccomp either (seccomp is syscall-level, not host-level) —
  it would also need a proxy bridge or an eBPF/landlock-net layer. "ask" must
  be an approval layer in BOTH approaches.
- **Verdict:** tie at the *fundamental* level — neither bwrap nor
  Landlock+seccomp gives host-level egress control natively. The realistic
  tri-state for us is **deny / allow**, with **"ask" implemented as an
  approval/decision layer** (mirroring Codex), and selective egress deferred
  (would need a proxy bridge in either design). This revises the brief: do
  not promise host-scoped "allow only registry X" from the sandbox alone.

### C-MAINT — maintenance surface
- **Bubblewrap:** HEAVIER. External dep, version probing (`--argv0`,
  `--perms`), bundled-binary shipping + SHA256 pin, WSL1 special-casing,
  userns-availability handling, two-stage re-exec helper binary.
- **Go-native:** LIGHTER. A few `go.mod` libs (or hand-rolled raw syscalls),
  build into the binary, no runtime probing of an external tool's flags.
- **Verdict:** Go-native is materially less to maintain.

### Comparison summary

| Constraint            | Bubblewrap (Codex) | Go-native Landlock+seccomp |
|-----------------------|--------------------|----------------------------|
| C-STATIC single binary| WEAK (external bin)| **STRONG**                 |
| C-LIGHT unpriv+kernel | STRONG             | STRONG                     |
| C-SKIP graceful skip  | MIXED (refuses)    | **STRONG**                 |
| C-LAYER w/ exec-ro    | STRONG              | **STRONG** (per-path modes)|
| C-NET tri-state       | PARTIAL (binary)   | PARTIAL (binary; ask=layer)|
| C-MAINT surface       | HEAVIER            | **LIGHTER**                |

---

## Solution-scout packet (for the planner / optional debate)

### problem_frame
- **objective:** pick the enforcement mechanism for `vh-agent-harness
  exec-sandbox` (the L2 OS-level sandbox that backs up the L1 `exec-ro`
  heuristic gate).
- **constraints:** C-STATIC (single static Go binary, cgo-free), C-LIGHT
  (unprivileged + kernel-enforcing), C-SKIP (graceful skip if unsupported),
  C-LAYER (compose with `exec-ro`), C-NET (network deny/allow/ask), C-MAINT
  (low surface).
- **success_criteria:** a sandbox that (1) ships inside the existing binary,
  (2) enforces read-only intent at the kernel level with `.git`/agent-state
  protected, (3) degrades to `exec-ro`/warn when unsupported, (4) supports
  network deny and allow with "ask" routed through our existing approval
  surface, (5) needs no host package install.

### criteria (with importance)
- `single-binary-no-host-deps` — **critical** (this is our product identity).
- `kernel-enforcing-unprivileged` — **critical**.
- `graceful-skip-to-exec-ro` — **important**.
- `network-deny-allow` — **important** (host-scoped egress is **nice_to_have**
  and likely deferred — see C-NET).
- `low-maintenance-surface` — **important**.
- `cross-platform-future` — **nice_to_have** (Linux-first; macOS is
  second-class per the operator).

### options

- **O1 — Go-native Landlock+seccomp trampoline (keep current brief).**
  - mechanism: in-process Landlock FS restrictions + seccomp network/syscall
    filter + `no_new_privs`, applied before `exec`. Per-path Landlock modes
    map to exec-ro's classifier output; protected subpaths (`.git`, agent
    state) get no-access.
  - adaptation_for_repo: implement behind `exec-sandbox` as a Go pkg under
    `internal/`; feature-detect Landlock/seccomp at startup; on unsupported
    → warn + fall back to `exec-ro` (graceful skip).
  - evidence_ids: E1 (F4 Codex retired Landlock-FS to fallback but the
    *primitive* is sound), E2 (F9 no re-exec needed for in-process apply),
    E3 (F2 protected-subpath concept), E4 (F3 network is binary in any
    syscall-layer approach).
  - assumptions: Landlock v2 + seccomp available on our target kernels
    (modern Linux — verify min kernel).
  - risks: Landlock has gaps vs full namespace isolation (no mount-namespace
    hiding; Landlock is *additive access control* on top of the real FS, not
    a fresh mount view); seccomp network is syscall-level not host-level.
  - cheapest_validation_step: spike — apply Landlock ro/rw/noaccess on
    `./tmp/` + repo + `.git` and confirm a sandboxed `touch`/network call is
    denied; confirm clean skip on a Landlock-less kernel.

- **O2 — Shell out to bwrap mirroring Codex.**
  - mechanism: construct the bwrap argv (F1): `--new-session
    --die-with-parent --ro-bind / / --dev /dev --bind <repo> <repo>
    --ro-bind <repo/.git> <repo/.git> --bind <repo/tmp> <repo/tmp>
    --unshare-user --unshare-pid [--unshare-net] --proc /proc -- <cmd>`.
  - adaptation_for_repo: detect system bwrap on PATH (outside CWD); on
    missing → graceful skip to exec-ro (DIVERGES from Codex, which refuses).
  - evidence_ids: E1 (F1 argv), E2 (F2 layered binds + protected subpaths),
    E3 (F3 binary network).
  - assumptions: a usable bwrap is present on operator systems OR we accept
    the host dep.
  - risks: **breaks C-STATIC** (external binary); version/flag probing
    (`--argv0`, `--perms`) adds maintenance; distros that disable
    unprivileged userns fail; WSL1 fails.
  - cheapest_validation_step: prototype the argv builder in Go and run a
    sandboxed `ls`/`touch` against `./tmp/` to confirm the bind layering.

- **O3 — Hybrid: Go-native default, optional bwrap backend.**
  - mechanism: Landlock+seccomp is the default (always available, in-binary);
    if a system bwrap is detected AND the operator opts in (profile), use the
    stronger namespace isolation for high-distrust commands.
  - adaptation_for_repo: `exec-sandbox` exposes a backend selector
    (`auto|landlock|bwrap`); `auto` = Landlock unless bwrap present +
    profile demands it.
  - evidence_ids: E1–E4 (all).
  - assumptions: the added abstraction is worth it for a small high-distrust
    slice.
  - risks: two codepaths to maintain (heaviest C-MAINT); complexity creep.
  - cheapest_validation_step: confirm the selector + Landlock path first;
    bwrap path can be a later slice.

### cross_option_notes
- The dominant tradeoff is **C-STATIC vs isolation strength**: bwrap gives a
  true fresh mount namespace (stronger FS isolation — hidden paths, not just
  access-denied), but at the cost of an external binary and our product
  identity. Landlock is *additive* access control on the real FS view (a
  denied path is still *visible*, just inaccessible) — weaker than a
  namespace hide, but in-binary and sufficient for the exec-ro enforcement
  goal.
- Major evidence gap: we have NOT benchmarked whether Landlock's
  "access-denied but visible" model is acceptable for our threat model (an
  agent can still `ls` a denied tree). If path *visibility* matters, bwrap's
  namespace hide is the only option that satisfies it.
- **O1 should probably lead** as the default; O3 is the high-upside path if
  a high-distrust slice later demands namespace-grade hiding.

### debate_handoff_question
> Given that Landlock is additive access-control (denied paths stay visible)
> while bwrap hides them via a fresh mount namespace, is path-visibility a
> real threat for `exec-sandbox`'s use cases — and if so, is the external
> bwrap dependency justified for that slice, or should we accept
> "inaccessible but visible" as the L2 contract and keep the binary pure-Go?

---

## Recommendation (single pick)

**Pick O1: keep the Go-native Landlock+seccomp trampoline as the `exec-sandbox`
default.** It is the only option that satisfies our defining constraint
(C-STATIC single static Go binary, no host deps), it composes naturally with
`exec-ro`'s per-path classifier output, it gives us a clean graceful-skip to
`exec-ro`, and it is materially lighter to maintain than a bwrap shell-out
with version probing and bundled-binary shipping. Codex's own choice to
**retire Landlock-FS to a fallback in favor of bwrap** does NOT contradict
this — Codex is a cross-platform product that already accepts an external
bwrap dep and needs the *namespace-hide* property for its broader threat
model; we are a Linux-first single-binary tool whose L2 goal is to enforce
`exec-ro`'s read-only intent at the kernel level, which Landlock does
sufficiently. Borrow Codex's *techniques*, not its *architecture*: (a) the
protected-subpath re-readonly concept (F2) → implement as Landlock
no-access on `.git`/agent-state inside a writable root; (b) the binary
network model (F3) → seccomp deny/allow with "ask" routed to our approval
layer; (c) `--die-with-parent`/`--new-session` lifecycle (F1) → equivalent
process-group + `PR_SET_PDEATHSIG` handling.

**Decisive tradeoff:** bwrap gives a stronger FS isolation (true namespace
hide vs Landlock's additive access-denied-but-visible) in exchange for an
external binary that breaks our single-static-binary identity. For our
scoped L2 enforcement goal, that trade is not worth it by default — but it
becomes worth it for an optional high-distrust slice (→ O3) IF path
visibility is later judged a real threat.

### Operator policy calls required (NOT decided here)
1. **Graceful-skip vs refuse:** Codex refuses when the sandbox is required but
   unavailable; our brief says graceful-skip. Confirm we want skip-to-`exec-ro`
   (warn) rather than Codex-style refuse. *(affects C-SKIP)*
2. **External bwrap dependency:** if O3 (optional bwrap backend) is ever
   adopted, accepting a system-or-bundled bwrap breaks the strict "single
   static Go binary, no host deps" identity unless bwrap is explicitly
   treated as an OPTIONAL, feature-detected runtime (system bwrap on PATH,
   never auto-installed). The operator must sign off on that identity
   carve-out before O3 lands. *(affects C-STATIC)*
3. **Host-scoped egress:** neither approach gives "allow only registry X"
   natively (C-NET). Confirm the realistic network contract is **deny/allow**
   with **ask-as-approval-layer**, and that selective egress (npm to one
   registry) is deferred — it would need a proxy bridge (Codex's TCP→UDS→TCP)
   or an eBPF/landlock-net layer in either design. *(affects C-NET)*

---

## Version anchor & verification status

- **Source studied:** `openai/codex` branch `master`/`main`, read via the
  zread repo tools. **Commit SHA NOT pinned** — no SHA was captured during
  the read pass. The mechanisms (bwrap namespaces, seccomp, Seatbelt) are
  STABLE, but exact flag counts/argv order can drift across Codex releases.
- **Before any code lands,** re-run the cited file/function reads against a
  PINNED commit SHA and record it here. The functions to re-verify:
  `bwrap.rs::{create_bwrap_flags, create_bwrap_flags_full_filesystem,
  create_bwrap_command_args, create_filesystem_args, BwrapNetworkMode,
  should_unshare_network}`, `landlock.rs::
  {install_network_seccomp_filter_on_current_thread,
  install_filesystem_landlock_rules_on_current_thread (the "unused" comment)}`,
  `linux_run_main.rs::run_main`, `launcher.rs::preferred_bwrap_launcher`,
  `bundled_bwrap.rs::launcher`, `proxy_routing.rs::{spawn_host_bridge,
  spawn_local_bridge}`.
- **Unverified items (mark explicitly):** exact line numbers (zread returns
  single-line JSON; citations are `fn-level`); the macOS Seatbelt SBPL details
  are MED confidence (sbpl file read + search_doc summary, not a full
  seatbelt.rs re-read); the "ask lives in the approval layer" claim (F3) is
  inferred from `manager.rs` + `search_doc`, not a single quoted line.

## Confidence summary
- HIGH: F1 (argv), F2 (FS model), F3 (network binary+proxy+seccomp), F4
  (Landlock unused), F5 (privilege), F7 (bundled bwrap), F9 (re-exec).
- MED: F6 (feature detection nuance), F8 (macOS/Windows), F10 (UX/config),
  the "graceful skip is refuse in Codex" inference.
- LOW: none material.

---

## Promotion targets (if this becomes active guidance)

This packet is a SOURCE study + option comparison, NOT active guidance. If the
coordinator adopts the recommendation, the **promotion targets** are:
- `docs/ai/` (or equivalent) — a durable `exec-sandbox` design note capturing
  the chosen mechanism (O1 default), the protected-subpath rule, the network
  deny/allow + ask-as-approval-layer contract, and the graceful-skip policy.
- The `exec-sandbox` brief in the planner (revise the network tri-state to
  deny/allow + approval-layer ask; note selective egress is deferred).
- `README.agent.md` — only if/when `exec-sandbox` actually ships and the
  command surface changes (do NOT update for a design decision alone).

These updates belong in a **follow-up slice**, not in this source packet.

## Recommended next specialist / command
- **Next:** `planner` (or `/solution-brief`) to revise the `exec-sandbox`
  brief around O1, incorporating the three operator policy calls above. If
  the path-visibility question (debate_handoff_question) is contentious,
  route through `debate` first.
- **Artifact type produced here:** `sources` (this file). A separate
  `researches/decisions/` memo is NOT warranted unless the operator asks for
  a formal option-recommendation record; this packet already carries the
  recommendation inline at the operator's request.
