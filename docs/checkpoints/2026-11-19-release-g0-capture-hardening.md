# Checkpoint — v0.30.0 G0 first-invocation incident + FAIL-preserving capture hardening (2026-11-19)

Dated incident record and hardening note. Before this checkpoint the incident
lived only in the release session and the tmp capture file; this record makes
it durable per the incident-documentation rule.

## Incident (v0.30.0 release ceremony, first wrapper invocation)

- During the v0.30.0 ceremony, `scripts/release-tag.sh`'s FIRST invocation
  failed G0 (`ok:false`). The refusal JSON's `error` field inlined the ENTIRE
  `go test ./...` output unbounded (`"<reason> (output: $G0_OUTPUT)"`).
  Repeated warning spam during the test run filled the caller's head-kept
  output budget, and the `--- FAIL:` identity at the output TAIL was
  truncated away — the red was undiagnosable from the refusal.
- **Non-reproducing:** three independent re-runs at the identical tree were
  green. The root cause of the red itself is NOT identifiable now (fog — do
  not chase). Evidence retained: `tmp/release-g0-capture-v0.30.0.txt` (local
  scratch under gitignored `tmp/`, not in git; this checkpoint is the durable
  record) is the GREEN retry capture (ok:true, WRAPPER_EXIT=0, tag v0.30.0
  pushed); the lost-capture red output no longer exists.
- v0.30.0 is tagged (peels to M = 09a8120) and pushed; local `main` is ahead
  of origin by exactly the three ceremony commits (N=fc9ffbb, R=26a3f32,
  M=09a8120).

## Defect class and hardening

The capture mechanism found in the wrapper: **inline command substitution**
(`G0_OUTPUT=$(go test ./... 2>&1)` in both classic and historical G0 flows)
whose full value was inlined unbounded into the refusal JSON — the wrapper
retained the complete textual output (modulo command-substitution
trailing-newline normalization), but the refusal line exceeds any downstream
consumer's head-kept display budget, so tail FAIL lines get truncated away.

Hardening (same change as this checkpoint; wrapper + test + README):

1. **Durable full capture** — on a red, the failing gate command's complete
   unfiltered TEXTUAL output (everything the wrapper itself saw; bash command
   substitution retains it all, modulo trailing-newline normalization) is
   written to
   `<repo-root>/tmp/release-gate-capture-<version>-<label>-<utcstamp>-<pid>.txt`
   (label: `go-test`|`go-vet`|`go-build`|`gofmt`|`g0c-doctor`). Green runs
   deliberately write NOTHING: G0b's `git status --short` runs AFTER G0 in
   the classic flow, and an always-on capture would false-refuse dirty in any
   repo that does not gitignore `tmp/`.
2. **FAIL identity independent of display budget** — the refusal extracts the
   failure-summary lines (`^--- FAIL:` / `^FAIL` for go commands; tier-FAIL
   lines for the G0c doctor control run) and places them BEFORE any excerpt.
3. **Bounded display** — the inline excerpt is first-40 + last-40 lines with
   an elision marker; the unbounded whole-output inline is gone.
4. **Path disclosure** — the refusal names the capture file.

**Invariant (pinned end-to-end):** a gate red preceded by voluminous warnings
still yields the failing test's name in the refusal. Proven by
`tests/scripts/release-tag-g0-capture.test.js` — "crux: spam-preceded G0
go-test red still names the failing test + capture path in the refusal"
(4000 warning lines before a `--- FAIL:`; asserts FAIL identity + capture
path in the refusal, complete textual capture on disk, and middle-line
elision from the bounded excerpt).

Files changed: `scripts/release-tag.sh` (helper `g0_fail_reason` + G0 arms in
both flows + G0c control run), `tests/scripts/release-tag-g0-capture.test.js`
(new seam, follows `release-tag-s2-gate.test.js` conventions),
`README.agent.md` (release-ceremony "Recovery: ceremony refusals" section —
FAIL-preserving refusal shape documented) — all three landed as commit 1
`fae7238cd45ca3bff785156dc09a8e2a023da8e0`; this checkpoint document is the
separate commit 2 per the reviewer-confirmed split.

## Findings

- **G0 refusal output was unbounded inline:** source=scripts/release-tag.sh
  pre-change (`(output: $G0_OUTPUT)`), confidence=high, type=fact.
- **`go test` runs a default high-confidence vet subset:** source=observed in
  test development (`printf` vet error surfaced as a go-test build failure),
  confidence=high, type=fact — the vet-arm fixture uses `copylocks` (outside
  that subset) to reach the wrapper's dedicated `go vet` step.
- **Doctor tier-FAIL lines are mid-line tokens, not line prefixes:**
  source=internal/cli/doctor.go (name token + tier token), confidence=high,
  type=fact — hence the parameterized extraction pattern for the G0c arm.

## Contradictions

None detected.

## Verification

Assessed tree for the CHANGED state: commit
`fae7238cd45ca3bff785156dc09a8e2a023da8e0` (commit 1 — wrapper + test +
README, landed 2026-11-19 via gated-commit after commit-review; committed
tree `5fc16f8ca239f78da1e57687f24dcf437f009049`, byte-identical to the
reviewed tree). The gate receipts below were re-run at HEAD = `fae7238` with
the only working-tree delta being this checkpoint document (untracked at run
time, no test surface).

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| New seam green (3 arms: spam-red crux, vet-red, green-no-capture) | `node --test tests/scripts/release-tag-g0-capture.test.js` → 3 pass / 0 fail | yes |
| Existing wrapper seam unbroken | `node --test tests/scripts/release-tag-s2-gate.test.js` (part of full suite) | yes |
| Wrapper syntax valid | `bash -n scripts/release-tag.sh` → exit 0 | yes |
| Full JS suite | `node --test tests/scripts/` → 260 pass / 0 fail / 0 skipped (baseline 257 + 3 new) | yes |
| Full Go gate set | `go test ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` exit 0 with 0 lines | yes |

```behavioral-closure
verdict: proven
# tree binding: assessed state = commit 1 fae7238cd45ca3bff785156dc09a8e2a023da8e0 (wrapper + test + README); receipt re-run at that HEAD — see the Verification preamble
path: scripts/release-tag.sh G0 red path — a failing gate command (go test red with ~4000 warning lines preceding the FAIL identity) flows through g0_fail_reason into the refusal JSON: extracted FAIL summary + capture-file path + bounded excerpt
verifier: tests/scripts/release-tag-g0-capture.test.js (spawns the REAL wrapper end-to-end in a scratch ceremony repo whose prior gates pass)
command: node --test tests/scripts/release-tag-g0-capture.test.js
result: proven
```

Crux outcome observation (not mechanism): the parsed wrapper refusal itself
contains `--- FAIL: TestWarnThenFail` and the capture path, the capture file
on disk contains the complete textual output (spam head, elided middle, FAIL
tail), and a middle spam line is absent from the refusal while head/tail
survive — the tested classic-flow scenario preserved the FAIL identity
despite the warning volume that consumed the v0.30.0 first invocation's
refusal. The historical flow uses the same helper (G0 arms in both flows
route through `g0_fail_reason`), not
separately test-covered (noted, not chased — same helper, same composition;
a follow-up candidate with trigger `path_touched(scripts/release-tag.sh)`
was filed from the commit review's DEFER findings).
