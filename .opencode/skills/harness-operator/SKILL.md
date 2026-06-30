---
name: harness-operator
description: "Operate the vh-agent-harness agent harness itself — install, self-update, run the upgrade loop, read release migration notes, and use guide/update/doctor. Load this skill when asked to install, update, or operate the harness, or to read what changed in a release."
compatibility: opencode
---

# Harness Operator

This skill is the **load trigger for operating the harness itself**. It is a
concise routing aid, not a duplicate of the long-form manual. `vh-agent-harness
guide` is the dynamic authority — it reflects the real command surface of the
binary you are running; this skill points at it.

For **extending** the harness (adding agents/commands/skills/overlays), use
`/harness` instead — that is a different concern. Route there, do not duplicate.

## When to load

Load this skill when the task is any of:

- installing or re-installing the harness into a repo (`vh-agent-harness install`),
- running a release upgrade (`self-update` → `update` loop),
- previewing or applying a corpus refresh (`update --dry-run` / `update`),
- reading what changed in a release and how to migrate (`help migrate`),
- health-checking the install (`doctor`), or
- deciding what to edit when a generated file is wrong.

## The core loop

```
guide  →  update  →  doctor
```

- `vh-agent-harness guide` — orient yourself; state-aware next steps for the
  current directory. Run this first when unsure.
- `vh-agent-harness update --dry-run` — ownership-safe preview; nothing is
  written. Always preview before applying.
- `vh-agent-harness update` — apply platform_managed + active overlay_extension.
- `vh-agent-harness doctor` — lint the result; resolve any findings before
  considering the install healthy.

## Golden rules

1. **Preview first.** Run `update --dry-run` before `update`. The dry run is
   side-effect-free (no write, no lineage touch).
2. **Never hand-edit platform_managed files.** They are overwritten on every
   `update`. A change you need there must come from the template source.
3. **Edit under `.vh-agent-harness/`, never `.opencode/`.** The `.opencode/`
   tree is GENERATED from `.vh-agent-harness/` plus the embedded corpus; any
   hand edit there vanishes on the next `update`.
4. **Respect ownership.** A plain render only overwrites `platform_managed`
   (and active `overlay_extension`). project_owned files are preserved when
   present and seeded at most once when absent.

## The release upgrade loop

After a `self-update`, run this exact sequence (it is also the canonical
sequence embedded in every migration note):

```bash
vh-agent-harness self-update            # pull the new binary
vh-agent-harness version                # confirm the new version
vh-agent-harness update --dry-run       # ownership-safe preview of the refresh
vh-agent-harness update                 # apply platform_managed + active overlay_extension
vh-agent-harness doctor                 # lint the result
```

## Reading migration notes

Every release ships a migration note so operators and agents know what changed
and how to migrate:

- `vh-agent-harness help migrate` — detect the locally adopted version and show
  the relevant note (or the latest available).
- `vh-agent-harness help migrate v0.1.9` — show a specific release's note
  verbatim (a bare `0.1.9` is normalized to `v0.1.9`).

Notes describe **consumer-visible** changes, the automated migration sequence,
watch-outs, verification commands, and rollback. Read the note for the version
you are upgrading to before running the loop above.

## Pointers

- `vh-agent-harness guide` — dynamic, state-aware operating manual (authority).
- `README.agent.md` — long-form operating reference rendered into the repo.
- `/harness` — recipe for **extending** the harness (agents/commands/skills/overlays).
