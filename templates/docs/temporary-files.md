# Temporary Files & `tmp/` Usage Guide

Agents and contributors often need to download, generate, or transform large **intermediate artifacts** (e.g., sample data, build outputs, cache files). These files are *not* source and must **never** be committed to the repository.
To keep the working tree clean and CI fast, follow the conventions below.

## 1. Use `./tmp/` for all transient assets

* **Location** – A top-level directory named `./tmp/` at the repository root holds all temporary assets (create it on demand):
  ```text
  <repo-root>/
  ├─ ...
  ├─ tmp/            <- put all temporary assets here
  └─ ...
  ```
* **Scope** – Store **only** data that is safe to delete at any time, such as:
  - Raw downloads fetched during tests or research runs
  - Uncompressed archives / build artifacts
  - Intermediate outputs produced by scripts or notebooks
  - Short-lived cache files

For OpenCode sessionized work, prefer a per-session subtree:

```text
tmp/agent-runs/<session-alias>/
```

This keeps bulky outputs grouped by task while the corresponding durable summaries live under `.opencode/state/sessions/<session-alias>/memory/`.

For long-running detached jobs (e.g. via the `bgshell-job` skill), expect job
state under a per-job subtree:

```text
tmp/agent-runs/<session-alias>/jobs/<job-name>/
```

Do not clean those job directories until the job is intentionally stopped or the durable outputs have been preserved elsewhere.

## 2. Never commit `./tmp/` contents

The `./tmp/` directory is covered by `.gitignore`.
**Do not** force-add or commit files from `./tmp/` unless the contribution explicitly documents why the file is *not* temporary (very rare) and gets explicit maintainer approval.

## 3. Clean up after yourself

Temporary files must be removed as soon as they are no longer needed.

### For agents
1. **Delete on success** – At the end of every autonomous run, remove any files you created under `./tmp/` *unless* a later step or human review still needs them.
2. **Handle failures** – If a run aborts early, attempt best-effort cleanup in a `finally` block or equivalent.
3. **Idempotency** – Write scripts so that reruns start from a clean slate (e.g., `rm -rf ./tmp/whatever || true`).
4. **Size awareness** – Warn the user if the cumulative size of `./tmp/` grows large and propose pruning.
5. **Session manifests** – When using the OpenCode session-memory workflow, record important temp outputs in `tmp/agent-runs/<session-alias>/manifest.json` so `/job-cleanup` can remove the disposable ones intentionally.

### For humans
* Run `git clean -fdX tmp/` to wipe ignored files.

## 4. Do not rely on system-level temp paths

Do **not** rely on container or host temp directories such as `/tmp` for agent or project workflows. Cleanup ownership for system-level temp files is not reliable enough, which makes them a poor fit for repeatable repository workflows.

Use `./tmp/` instead, even for short-lived intermediate artifacts, so temporary files stay visible, scoped to the repository, and easy to clean intentionally.

## 5. Example (agent pseudocode)

```python
from pathlib import Path
import shutil

work_dir = Path("tmp/agent-runs/my-run")
work_dir.mkdir(parents=True, exist_ok=True)
try:
    download_path = work_dir / "download.bin"
    output = work_dir / "processed.out"
    download(url, download_path)
    process(download_path, output)
    # ... later, after preserving durable artifacts ...
finally:
    shutil.rmtree(work_dir, ignore_errors=True)
```

## 6. CI behaviour

CI jobs run in fresh containers; `tmp/` starts empty and is **not cached** between jobs. Tests that rely on files under `tmp/` must create them at runtime.

---

By rigorously isolating and cleaning temporary assets inside `./tmp/`, we keep the repository lean, avoid accidental commits of large binaries, and ensure reproducible, deterministic builds.
