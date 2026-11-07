# Reconciling a preserved `platform_managed` file (stale-preserve runbook)

> Audience: operators and agents of repos managed by `vh-agent-harness`.
> Use this when `update` printed a `[consumer-edit, platform-also-changed]`
> stale-preserve warning, or `doctor` reported `preserved-stale-vs-corpus`.

## When this state occurs

`update` never clobbers a locally-edited `platform_managed` file: when the
live bytes differ from the recorded origin hash, the file is **preserved**
(`managed-diverged`, reason `consumer-edit`). Preservation is **per file**.
Most of the time that is the whole story — your edit stands, the platform
re-rendered the same base bytes, nothing is lost.

The state this runbook addresses is **both moved**: the platform's version of
that file ALSO changed since the base your edit diverged from (incoming
corpus hash ≠ recorded origin, live hash ≠ recorded origin). In that state
the preserved file **did not receive this update's platform changes**, while
its sibling runtime files and documentation DID advance. Dependent runtime
files or documentation updated by the release may no longer match it — for
example, a sibling script may import a symbol the preserved file predates,
or docs may describe behavior the preserved file does not implement.

Two surfaces name this state (both consequence-worded; neither asserts which
specific behavior is missing — the hashes cannot know that):

- `update` (live run): the preserved/stalled listing splits the
  consumer-edit set; stale paths are marked
  `[consumer-edit, platform-also-changed]` followed by a
  `stale-preserve warning:` line.
- `doctor` / preflight: `managed-drift` reports
  `preserved-stale-vs-corpus` at **WARN** (naming the paths). WARN never
  fails doctor and never blocks release gates — the edit is still a
  sanctioned preserved state, not drift.

An ordinary consumer edit (platform re-rendered the same base bytes) keeps
the standard preservation notice and `doctor`'s non-failing
`consumer-preserved edit(s)` INFO. Only the both-moved subset escalates.

## Step 1 — Safeguard the local edit FIRST (never skip)

`vh-agent-harness accept-platform <path>` writes the platform bytes over the
live file. It is destructive-by-intent and correct — but run BEFORE
safeguarding, it destroys the local edit it was supposed to reconcile.
**Never run a bare `accept-platform` on a stale-preserved path before the
patch is safeguarded.**

The DEFAULT safeguard is a byte-for-byte backup — it captures the live edit
regardless of git state:

```bash
cp <path> <path>.bak.$(date +%Y%m%d)
```

Keep the backup OUTSIDE the managed path (a `.bak` sibling of a managed
file is local-only clutter, not platform state — do not commit it unless
the repo's conventions say so).

If the managed path is git-TRACKED, patches can supplement the backup. Know
what each captures — these are git-relative diffs (worktree vs index/HEAD),
NOT a diff against the harness's recorded origin base:

```bash
git diff HEAD -- <path> > <path>.local-edit.patch # complete tracked-file diff (staged + unstaged)
git log -p -- <path>                              # edit history (re-apply reference)
```

WARNING: for an UNTRACKED managed file (e.g. a fresh install that was never
committed), `git diff` produces an EMPTY patch — the byte backup above is
the only real safeguard there.

## Step 2 — Inspect what the platform changed

See which paths the current corpus render disagrees with, and what update
would do:

```bash
vh-agent-harness diff               # per-path drift categories + paths (live vs current corpus)
vh-agent-harness update --dry-run   # per-file plan incl. the PRESERVE section
```

Both are path/plan views, not content deltas. For the actual byte delta of
your path, render a scratch install of the current corpus and diff your
safeguarded copy against its output:

```bash
vh-agent-harness install --target ./tmp/corpus-scratch --minimal   # current platform bytes
git diff --no-index <path>.bak.$(date +%Y%m%d) ./tmp/corpus-scratch/<path>
```

(`--minimal` yields the BASELINE corpus bytes. A path whose render depends
on your capability/overlay selection needs a scratch install carrying your
selection — copy your `.vh-agent-harness/vh-harness-profile.yml` into the
scratch target before installing.)

## Step 3 — Reconcile (choose one)

### Option A — adopt the platform version and re-apply your edit

Right choice when your edit is small/portable and the platform's new bytes
carry fixes or behavior you want:

```bash
vh-agent-harness accept-platform <path>   # writes platform bytes + advances the origin
# re-apply your local edit from the Step 1 patch/backup, then:
vh-agent-harness update                   # the path now classifies consumer-edit again (or converges)
vh-agent-harness doctor                   # expect the preserved-stale-vs-corpus WARN to clear
```

Note: after re-applying the edit the file is still a consumer edit (that is
fine and expected) — but it is now an edit **on top of the current platform
base**, so it is no longer stale-preserved and the warning/WARN clears. If
your re-applied edit happens to equal the platform bytes, the path simply
converges.

### Option B — promote the customization to a canonical home

Right choice when the edit should become canonical and survive every future
update without reconciliation. Which route fits depends on whether the
customization ADDS new paths or must REPLACE a file the platform already
renders.

**B1 — additive customization → overlay pack** (this is the promotion path
the drift remedy recommends): create the pack under
`.vh-agent-harness/overlays/<pack>/` (agents/, commands/, skills/,
`opencode-append.jsonc`, …), SELECT it under `overlays:` in
`.vh-agent-harness/vh-harness-profile.yml`, preview with
`vh-agent-harness update --dry-run`, then remove the hand edit and re-run
`update`. See `vh-agent-harness guide` and `/harness` for the overlay
anatomy.

An overlay pack can only ADD new paths (or merge config) — it cannot
replace a builtin at the same path. If a pack unit targets a path the
platform already renders (a core builtin, or an earlier active pack's
unit), the whole render fails closed with an `ErrShadowBuiltin` error
naming the colliding paths.

**B2 — same-path replacement → raise the file to `project_owned`** (the
route the `ErrShadowBuiltin` error itself points to): declare the override
in `.vh-agent-harness/harness-ownership.yml` and edit the live file
directly:

```yaml
overrides:
  .opencode/scripts/state-lib.js:
    class: project_owned
    reason: "local replacement; platform delta re-checked each release"
```

From then on the platform never touches that path on `update`: your edit
stands without preservation warnings, but the file no longer receives
platform changes either. Each future release, re-check the platform delta
yourself (Step 2's scratch-install diff, with your current live file as
the baseline) and re-apply the parts you want. `doctor` reports a
divergent ownership-raised path as non-failing preserved INFO.

For changes that should live in the platform for EVERY adopter, upstream
them into `templates/core/` (domain-free constraint) instead of keeping a
local edit.

## After reconciling

```bash
vh-agent-harness doctor    # preserved-stale-vs-corpus should be gone
```

If the WARN persists, the origin store still records a base the corpus has
moved past — re-check Step 2's diff for a second preserved path you have
not reconciled yet (the listing and the doctor detail name every path).

## Invariants worth knowing

- Preservation mechanics are unchanged by the warning: `update` never
  clobbers an AUTHORED (non-regenerated) preserved `platform_managed`
  file — warning or not. Exception by design: platform-REGENERATED paths
  (e.g. `allowed-commands.js`, `opencode.jsonc`) are overwritten on every
  apply; a consumer edit there is genuine drift, not preservation.
- The warning/WARN never fails a command and never blocks a release gate;
  it exists so the skew is visible at creation time (update) and at every
  health check (doctor).
- `origin-hashes.json` is binary-owned; never hand-edit it —
  `accept-platform` is the sanctioned re-baseline.
