# Retirement carrier — defer-release-g0-capture-remaining-arms

Durable decision-record carrier for task card
`defer-release-g0-capture-remaining-arms` (closeout 2026-11-19, session
`g0-capture-arms-exec`). This committed document is the artifact carrier the
v3 decision-record slots point at; each labeled item below reproduces the
card's decision-class content verbatim so it survives card retirement.

Landing: commit `e60f38a7e572f062435768729a166933d76c49f9` on `main` carries
the `Task-Card: defer-release-g0-capture-remaining-arms` trailer.

## Items

### decision-item: defer-release-g0-capture-remaining-arms#owner_notes[fnv1a:081b9fc2ace61631] (verbatim)

source:review-defer · trigger:path_touched(scripts/release-tag.sh) · studied:2026-11-19 · Consolidated from the fae7238 commit review (findings a-F1/d-F1/b-F2/c-F4 deferred with the same trigger; one theme, deduped per intake bar). The classic-flow crux IS covered and proven (see docs/checkpoints/2026-11-19-release-g0-capture-hardening.md); these are the remaining surfaces of the same helper. An additional always_before predicate from the review: do not claim the G0c capture path behaviorally complete until its arm lands.

### decision-item: defer-release-g0-capture-remaining-arms#open_questions[fnv1a:a1bc75992bf0b860] (verbatim)

How to fixture G0c control red in a scratch repo without building the real harness binary (the staleness guard requires ./bin/vh-agent-harness with the inject-errata subcommand)? Options: shell stub that answers --help and exits nonzero with FAIL-tier doctor output.

Resolution (context, not part of the item text): verified the guard's real
requirements in scripts/release-tag.sh (executable ./bin/vh-agent-harness +
`release inject-errata --help` canary exit 0) and adopted the card's suggested
option — a committed executable bash stub whose `release` arm answers the
canary and whose `doctor` arm reds with tier-FAIL lines; see the arm-2 test
and the closeout report.

### decision-item: defer-release-g0-capture-remaining-arms#measured_outcome[fnv1a:82809896b5db513f] (verbatim)

All three remaining g0_fail_reason arms now pinned end-to-end through the real wrapper; 6/6 arms green (run 3x incl. at the committed tree), full tests/scripts/ seam 263/263 green, Go commit bar clean. Landed as e60f38a on main with the Task-Card trailer. Wrapper diff verified comments/wording-only by commit-reviewer (4/4 leaves approve). One residual coverage gap (go-build/gofmt arms) captured as draft card defer-release-g0-build-gofmt-arms.
