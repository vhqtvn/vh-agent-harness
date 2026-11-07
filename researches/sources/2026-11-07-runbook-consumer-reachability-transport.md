# Runbook consumer-reachability — transport study (task-2026-11-07t04-39-29)

Scope: how `docs/ai/preserved-managed-reconciliation.md` (content corrected in 5a9c7852) reaches CONSUMERS when the stale-preserve warning fires. Transport only.

Verdict: hybrid (b)+(a′) — reword live warning (safeguard-first, correct routing, pointer to docs key) + ship runbook as a `vh-agent-harness docs` key (templates/docs embed, zero repo render, no inventory row). Reject (a) full render (seeds into every install: apply.go:421,456) and (c) (README.agent.md does not render: no corpus copy, no writer code; SKILL.md:132 claim is false).

Channels that reach consumers: templates/core render (corpus.go:29,36-37); binary-carried templates/docs via `docs [key]` (corpus.go:135-155; docs.go:32-123); templates/migrations via `help migrate` (corpus.go:175-189); published release notes; live warning output (install.go:504). Channels that do NOT: dev docs/ai (8 of 11 files unshipped; corpus ships only f2-rendering-family.md, shell-execution.md, stream-stall-recovery.md); README.agent.md (git ls-files; core_manifest.go:161-224; grep internal/).

Warning defects: install.go:463 header invites bare accept-platform before safeguarding; :504 tail "port the customization into an overlay pack" refuted for same-path (shadow.go:24-44 ErrShadowBuiltin); no full-procedure pointer. Reword keeps substring pins green (accept_platform_test.go:468-481,512); update wording-contract comment install.go:495-503.

Inventory correction: ledger scope = overlay packs ∪ core capabilities only (bootstrap_inventory_test.go:17-71; full-harness-profile.yml:18-39). Corpus docs and DocsDir keys require no ledger row; (a)'s real cost is the seed-everywhere render, not inventory.

Dedup: no round-2 transport card. 06-25-57 (completed) distinguished content-vs-transport and ordered content first (satisfied: 5a9c785 landed 2026-11-07 before this closeout). 04-39-27 erratum injection reported; 06-55-40 erratum:77 fix draft.

Follow-ups: (1) fix-warning-safeguard-first; (2) ship-runbook-docs-key (+docs-overrides dogfood entry; coordinate 06-55-40); (3) fix-readme-agent-render-claim (SKILL.md:132; guide.go:168,271; root.go:95; seam.go:1088; execro/classifier.go:11).

decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#open_questions[fnv1a:2570797be9a0f67c] (curated)
decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#measured_outcome[fnv1a:34f46d1e8b814093] (curated)

---

## Appended at landing (2026-11-07) — retirement carrier anchors

Landed 1:1 from the closeout-report appendix (the read-only study session
could not write outside `tmp/`, so the packet was embedded verbatim in the
report for a write-permitted slice to lift). This file's name and location
match the card's stored decision-record carrier path exactly
(`researches/sources/2026-11-07-runbook-consumer-reachability-transport.md`
— an immutable carrier slot). The two decision-item handles above are part
of the verbatim packet body; the line below adds the card-id anchor required
for card retirement.

Task-Card: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md
