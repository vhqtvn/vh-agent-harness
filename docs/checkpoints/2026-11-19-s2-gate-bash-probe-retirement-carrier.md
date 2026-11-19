# Retirement carrier — defer-release-s2-gate-bash-probe-script-mode

Durable decision-record carrier (v3) for the local task card
`defer-release-s2-gate-bash-probe-script-mode` (closeout 2026-11-19, session
`g0-capture-arms-exec`). This committed document is the artifact carrier the
closeout's decision-record slots point at; each labeled item below reproduces
the card's decision-class content verbatim so it survives card retirement.

Landing: commit `615ce81d8efe9d9f77f7757ee81f69439687f047` on `main` carries
the `Task-Card: defer-release-s2-gate-bash-probe-script-mode` trailer.

## Items

### decision-item: defer-release-s2-gate-bash-probe-script-mode#owner_notes[fnv1a:a33b5a70ed16a201] (verbatim)

source:review-defer + builder full-seam run · trigger:path_touched(tests/scripts/release-tag-s2-gate.test.js) · studied:2026-11-19 · convergent review finding (tier1_a-F2/c-F1/d-F1) + the 017330b closeout's 12-skip disclosure.

### decision-item: defer-release-s2-gate-bash-probe-script-mode#owner_notes[fnv1a:80bdee3e28b432ab] (verbatim)

STUDY VERDICT 2026-11-19 (researcher, static at dbd8f38): all premises VERIFIED (probe spawnSync bash -c true at L37; 12 declarative per-test skip arms — 9×RUN at L206-343, 3×BASH at L363-433; 13th vm arm L458 has no skip; skip mechanism = registered-and-counted, receipt-consistent 265=253+12). Receipt archaeology: arms ran green historically (673c8c7 2026-08-09 introducing commit, 74a6fd8 2026-11-15 json_str arms) and plausibly in the e60f38a session's unsandboxed env; dormant specifically under sandboxed exec supervision AND in ALL of CI (go-test.yml runs only go test — no workflow runs node --test, so the file is CI-invisible). Risk register: assertion rot LOW (s2/json_str surfaces unchanged since 74a6fd8), fixture assumptions LOW (hermetic scratch), timing LOW/net-positive (removes ~40s probe hang); real-repo residue NONE statically (9 arms confine writes to scratch via G0_REPO_ROOT; 3 json_str arms exit at version validation L217-225 before any git access; vm arm in-process). Dedup holds. Disposition: EXECUTE-NOW, refined scope attached.

### decision-item: defer-release-s2-gate-bash-probe-script-mode#measured_outcome[fnv1a:6b94a407deffa6c8] (verbatim)

Experiment executed clean on the first shot — zero reds, zero triage, zero rot fixes. The probe flip (spawnSync("bash", ["/dev/null"]) mirroring g0-capture 017330b) un-dormanted all 12 previously-skipping arms under sandboxed exec; every arm passed green. Full seam 265/265 pass, 0 skipped — exactly as predicted. The ~40s probe hang is gone (file runtime now ~8.6s). Committed as 615ce81d8efe9d9f77f7757ee81f69439687f047 (single file +7/−1, Task-Card trailer verified via landing-proof grep). No wrapper changes; no new follow-ups surfaced (the study's risk register held: assertion rot LOW → none found, fixture assumptions LOW → none found, timing net-positive → confirmed).
