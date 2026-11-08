# Checkpoint — O4 eslink F5 deferral ratified, asset preserved (2026-11-07)

Decision record for the disposition of the uncommitted S1 `internal/eslink/`
implementation and the O4 work blocked behind it, following the consolidated
F2 disposition brief (`tmp/agent-runs/eslink-o4-f2-disposition/brief.md`).

## Decision

**F5 DEFERRAL RATIFIED — operator approval m0101, 2026-11-07.**

- The O4 detection-frame build (stages S2–S4) is **HELD**. All stages deferred,
  none completed; exact S2–S4 scopes were never established and are not
  invented here.
- **No fifth review cascade by default.** The declining-discovery-rate premise
  that motivated "one more round" is refuted by evidence (below); sunk
  investment is not evidence of correctness. A new cascade requires fresh
  authorization with a structural explanation of the slash-goal failure and a
  discriminating context matrix covering both error directions.
- The ratified D1/D2/D3 objective is not rescinded or quietly downgraded; S1
  does not land through a residual-risk DEFER.

## G32 outcome (why BLOCKED stands)

- **BLOCKED — 4 BLOCKs / 3 defects.**
- One root family: **misclassified slash → phantom regex → wrong surface**,
  producing both false-ok and false-broken outcomes.
- **Rate trend refutes convergence:** ~2 findings/round earlier vs 4 BLOCKs in
  G32. The 37 total review rounds are nonconverging (external r28 approved the
  then-current state; G29–G32 blocked).
- G32's consolidated report lives in the build session record; the disposition
  rationale is the F2 brief (copied into the archive as `brief.md`).

## Asset preserved

The untracked S1 package (5 Go files, 10,280 lines / 417,297 bytes, ~60
regression functions; baseline pins 61 modules / 49 edges / 5 dynamic UNKNOWN /
0 broken / 0 unparseable) is archived non-executably, marked G32 BLOCKED:

- **Primary:** `.local/coordinator/archives/eslink-g32-blocked/eslink-g32-blocked-2026-11-07.tar.gz`
  (SHA-256 `f45d2fc104c04f29317e48ccc7988eaea5cd6a2186a9a4982c2115f6c629e750`;
  gitignored). This fixed path is the designated fallback for the concurrent
  read-only researcher session that may still be measuring against the removed
  files.
- **Secondary:** `.local/backups/eslink-g32-blocked/eslink-g32-blocked-2026-11-07.tar.gz`
  (same tar, byte-identical — same SHA-256; gitignored via `.gitignore`
  `.local/backups/`, added as its own chore commit this slice after the
  operator chose that disposition mid-slice).
- Tar contents: the 5 files at their original `internal/eslink/` relative
  paths, plus `README.md` (status/history/restoration instructions),
  `MANIFEST.md`, and a verbatim copy of the F2 brief. No executable file
  modes (verified). Context HEAD at archival: `96e7bbe`.

### Per-file SHA-256 manifest (G32 pins re-derived and MATCHED)

| File | Lines | Bytes | SHA-256 |
|---|---:|---:|---|
| corpus_test.go | 254 | 9,316 | `2d2eaa608d4a861645fc02503bcbdd4863646b20572ef284afd1682ef1038338` |
| eslink_test.go | 5,591 | 233,984 | `ea22d628bec56685ee55e49097fa6f9f5bb9dce92d6b07dbf86861d6afe201cd` |
| graph.go | 548 | 19,807 | `ceb565b8e1f4ad8ff14a4d9098d04225d9c17568b4f29f97144ee34f4b1327be` |
| lexer.go | 3,041 | 126,130 | `f5eff7f1083564c69f18d048d7f0bb0d187a4c20f957b6a14687c47450ae277d` |
| parser.go | 846 | 28,060 | `5204557259283ca59728436e1035fac19743790a2851ce0c520684f999388268` |

### Restore verification (receipt summary)

Primary tar extracted into an isolated `tmp/` dir; all 5 files compared
against the live tree before removal: **FULL MATCH — 5/5 byte-for-byte (cmp)
and SHA-256; 8/8 expected archive members; no executable modes** (script
`tmp/eslink-g32-archive/verify-restore.sh`, run 2026-11-08, exit 0). Removal
of `internal/eslink/` happened only after this receipt: the package was fully
untracked (`git ls-files internal/eslink` empty), so git state is unchanged —
`git status --porcelain` went from `?? internal/eslink/` + ` M .gitignore` to
` M .gitignore` only.

## Restart path (no implementation restart before this exists)

A measured **corpus-conformance closure-coverage packet** is the mandatory
first artifact, then **debate → planner → operator** — in that order. The
packet must report a three-level distinction, never a single blended number:

1. **recognized imports** — edges whose import side the restricted grammar
   actually parses;
2. **assessable edges** — edges where provider/transitive re-export closure
   is derivable;
3. **enforceable obligations** — edges with closed surfaces on both sides
   that could carry enforcement without false verdicts.

Coverage must be measured over **core + overlays + mixed preserved trees**
(rendered token variants and destination collisions included, older
adopter-preserved bytes honestly UNKNOWN — closure never inherits from
ancestry or a corpus-origin label), and must include an **independent audit
of the 49-edge reference ledger** (S1 cannot be its own correctness oracle).
Frozen grammar before counting; complete consumption or UNKNOWN (no
convenient-line resumption); import completeness separated from export
completeness; regressions retained, including removed-export, late-import,
decoy, unsupported-suffix and slash/update negative mutations. O5
(vendor/adapter: tree-sitter, wazero, goja) may be researched in parallel;
none is a demonstrated drop-in. Operator ratification is required before any
re-scope: coverage-loss acceptance bar, what counts as closed evidence,
WARN/UNKNOWN presentation, corpus authoring restrictions, dependency-policy
changes.

## Standing value retained (not at risk by this deferral)

- Landed WARN surfacing for stale preserved managed files — commit `74f4568`
  ("fix(cli): surface stale preserved managed files on update and doctor",
  verified reachable from HEAD).
- The erratum, the runbook, and the research memos landed earlier in the O4
  arc.

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| G32 pins match live files | `sha256sum internal/eslink/*.go` — all 5 prefixes identical to G32 report | yes |
| Primary archive sealed | tar SHA-256 `f45d2fc1…` at `.local/coordinator/archives/eslink-g32-blocked/` | yes |
| Secondary byte-identical | `sha256sum` both tars → identical digests | yes |
| Secondary path ignored | `git check-ignore -v` → `.gitignore:86:.local/backups/` | yes |
| Restore verified before removal | `bash tmp/eslink-g32-archive/verify-restore.sh` → FULL MATCH 5/5, exit 0 | yes |
| Nothing tracked under internal/eslink | `git ls-files internal/eslink` → empty | yes |
| Tree clean after removal | `git status --porcelain` → ` M .gitignore` only (state at time of removal, before this checkpoint file was authored) | yes |
| WARN commit reachable | `git merge-base --is-ancestor 74f4568 HEAD` → exit 0 | yes |

Note: `go test ./...` was not run — no tracked file changed in this slice
beyond `.gitignore` and this document; the removed package was never in git,
so the tested surface at HEAD is unchanged by construction.
