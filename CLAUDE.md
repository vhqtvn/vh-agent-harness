# Agent Harness



## Architecture



## How to work here

- Read `AGENTS.md` for the full harness rules (term-contract, shell hygiene,
  command hygiene, delegation, commit gate, memory model).
- Run all commands through `vh-agent-harness exec`. Do not rely on host-installed tooling.
- The coordinator is read-only; delegate all coding/research/git to specialists.
- Git mutations route through the `committer` subagent (gated-commit protocol), available only where `core/gated-commit` is selected. Pass only this session's explicit file list; a concurrently-dirty tree is normal and unrelated dirty files are excluded by the private-index gate. To revert a stray file, use `commit-gate.sh revert <paths>`. On profiles without the capability, preserve the work, report the missing route, and request separately-authorized activation or operator handling (see `.opencode/docs/git-execution-routing.md`).
- Keep scratch under `./tmp/` (repo-relative). Never absolute home-dir paths.
