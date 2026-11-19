# Retirement carrier — defer-release-g0-build-gofmt-arms

Durable decision-record carrier for task card
`defer-release-g0-build-gofmt-arms` (closeout 2026-11-19, session
`g0-capture-arms-exec`). This committed document is the artifact carrier the
v3 decision-record slots point at; each labeled item below reproduces the
card's decision-class content verbatim so it survives card retirement.

Landing: commit `017330b9ec9932d70154871618710a625d0d162d` on `main` carries
the `Task-Card: defer-release-g0-build-gofmt-arms` trailer.

## Items

### decision-item: defer-release-g0-build-gofmt-arms#owner_notes[fnv1a:5efee65285313edd] (verbatim)

source:review-defer · trigger:path_touched(scripts/release-tag.sh) · studied:2026-11-19 · From the e60f38a commit-review (leaf d finding F2 in the first review; gate-run D-F3): the go-build and gofmt arms of g0_fail_reason remain without end-to-end pins after defer-release-g0-capture-remaining-arms landed the historical/G0c/capture-write arms. The residual is honestly disclosed in the wrapper's contract comment. Deduped against .local/coordinator/tasks/ (no existing card covers these arms; defer-release-gate-decode-arm is a different surface).

### decision-item: defer-release-g0-build-gofmt-arms#measured_outcome[fnv1a:29e1f3677da56032] (verbatim)

Both remaining unpinned g0_fail_reason call sites (classic-flow go-build and gofmt arms) are now end-to-end pinned through the real wrapper: 8/8 arms pass with 0 skipped (previously all arms silently skipped in sandboxed exec environments due to the bash -c probe quirk, now fixed in this file). Full seam exit 0 at 265 tests (263 baseline + 2 new arms), 12 pre-existing out-of-scope skips disclosed and filed. Contract comment's NOT-pinned list shrunk to the historical G0c TARGET run alone (wording only, zero executable wrapper changes). Landed as 017330b with Task-Card trailer.

## Context (not part of the item texts)

The two follow-up candidates filed at closeout: draft card
`defer-release-s2-gate-bash-probe-script-mode` (the same `bash -c` grandchild
probe quirk silently skips all 12 arms of tests/scripts/release-tag-s2-gate.test.js
in sandboxed exec environments) and draft card
`defer-release-g0c-target-run-arm` (the historical G0c doctor TARGET run — the
sole gate-output surface the wrapper's contract comment still lists as NOT
end-to-end pinned).
