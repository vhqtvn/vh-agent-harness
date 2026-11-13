# Completed task-card retirement carriers — 2026-11-14

> **Deletion-gate carrier checkpoint.** The completed-card retirement
> gate (`/task-delete`) refuses to destroy the last local copy of
> decision-class content that exists only in gitignored task cards
> (`.local/coordinator/tasks/`). Per the gate's promote-or-preserve rule
> (`docs/coordination/RECORD_LIFECYCLE.md` → "Refusals at retirement"),
> this checkpoint promotes that content into committed, branch-reachable
> form so the eleven held cards transcribed below can be retired without
> force.
>
> Each per-card section carries:
>
> 1. the **card-id anchor** — the full task id as a literal line, so the
>    card stays discoverable from git after retirement
>    (`git log --branches -S'<card-id>'` → `git show <sha>:<path>`);
> 2. the **original carriers** declared by the slots (pinned commits /
>    artifact paths), kept for provenance;
> 3. **every decision-record item**: the canonical
>    `decision-item: <card-id>#<item-id> (<disposition>)` label line
>    followed by the verbatim handle text the gate quotes (for curated
>    slots the label IS the handle), plus the item's source content from
>    the card fields.
>
> For the four `no_trailer_match` cards (original carrier commits are
> trailer-less by design), each section also carries a literal
> `Task-Card: <card-id>` trailer-form line, and the landing commit of
> this checkpoint carries the same four trailer lines in its message so
> the landing-proof gate resolves without force.
>
> All matched strings below are programmatic copies from the held cards
> as they stood on 2026-11-14 — fnv1a item ids and handles are
> content-derived and are NOT paraphrased. No new decisions are made
> here; this document only preserves existing ones.

## Class A — `handle_not_in_carrier` (7 cards, 24 items)

Decision-record slots whose declared pinned commit carriers do not
contain the item's representation (the handles below were never in those
commits' messages or trees). This checkpoint re-lands each item's
representation — anchor + label + verbatim handle together in one
committed blob — as the durable carrier.

### A1. Runbook correction: overlay packs cannot replace same-path builtins — fix reconciliation route in…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md

- card file: `task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md.json`
- title: Runbook correction: overlay packs cannot replace same-path builtins — fix reconciliation route in preserved-managed-reconciliation.md
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `5a9c78525e9f8952e01ac0b45efcf0b1708e90f6`
- decision_record.version: 3

decision_record note (verbatim):

    three owner-notes items (provenance / dedup-ordering / trigger); no measured_outcome field was minted; outcome narrative carried in this report body

#### A1 item 1/3 — `owner_notes[fnv1a:7cee1e2fb73aa0f8]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:7cee1e2fb73aa0f8] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:7cee1e2fb73aa0f8] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task), 2026-11-07; the note itself cites reframe-debate session ses_e4b7468dffeEOdAHYYpwc2Pyx and runbook commit c968b21

curated content beside the label (slot note, verbatim):

    provenance of the correction: reframe-debate session ses_e4b7468dffeEOdAHYYpwc2Pyx (2026-11-07) refuted the runbook's same-path overlay-replacement claim against landed runbook commit c968b21; the correction landed as commit 5a9c78525e9f8952e01ac0b45efcf0b1708e90f6

#### A1 item 2/3 — `owner_notes[fnv1a:82705032f7c16b11]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:82705032f7c16b11] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:82705032f7c16b11] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task), 2026-11-07; ordering inherited from the card's dedup against sibling card task-2026-11-07t04-39-29

curated content beside the label (slot note, verbatim):

    ordering constraint honored: the content fix (commit 5a9c785) landed BEFORE any consumer-reachability decision (sibling card task-2026-11-07t04-39-29) renders or inlines the runbook, so the corrected text propagates

#### A1 item 3/3 — `owner_notes[fnv1a:2f17d389562e1cb6]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:2f17d389562e1cb6] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-25-57-runbook-correction-overlay-packs-cannot-replace-same-path-builtins-fix-reconciliation-route-in-preserved-managed-reconciliation.md#owner_notes[fnv1a:2f17d389562e1cb6] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task), 2026-11-07; attestation against the then-landed shipped doc

curated content beside the label (slot note, verbatim):

    trigger attestation: the defect was live in a landed shipped doc (docs/ai/preserved-managed-reconciliation.md at commit c968b21); fixed at HEAD by commit 5a9c785

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: reframe-debate finding (session ses_e4b7468dffeEOdAHYYpwc2Pyx, 2026-11-07) against the landed runbook (commit c968b21).
    Dedup: distinct from task-2026-11-07t04-39-29-runbook-consumer-reachability-… (that card decides transport/rendering; this fixes content). Ordering: land this fix BEFORE any consumer-reachability decision renders/inlines the runbook, so corrected text propagates.
    Trigger: fired — defect is in a landed shipped doc now.

### A2. Erratum content fix: v0.29.0 erratum still carries refuted overlay same-path replacement guidance…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77

- card file: `task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77.json`
- title: Erratum content fix: v0.29.0 erratum still carries refuted overlay same-path replacement guidance (line ~77)
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `a362aa00a7f919e076bcc7be98ecb7d298bef2ae`
- decision_record.version: 3

decision_record note (verbatim):

    three owner-notes items (provenance / dedup / trigger); no measured_outcome minted — outcome narrative carried in the report body; carrier is the card's landing commit a362aa0 whose message carries the exact Task-Card trailer anchor

#### A2 item 1/3 — `owner_notes[fnv1a:d85fe4935ec4728c]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:d85fe4935ec4728c] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:d85fe4935ec4728c] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task, 2026-11-07); the note itself cites runbook-fix session ses_e4b497ad7ffeicF4yXGAsC5e0n and the commit 5a9c7852 closeout as the finding source

curated content beside the label (slot note, verbatim):

    provenance of the defect: found by the runbook-fix build session (ses_e4b497ad7ffeicF4yXGAsC5e0n) at commit 5a9c7852 closeout — erratum :77 still carried the refuted overlay-pack same-path replacement claim; fixed by this card's landing commit a362aa0

#### A2 item 2/3 — `owner_notes[fnv1a:38aee7b3614ad84e]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:38aee7b3614ad84e] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:38aee7b3614ad84e] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task, 2026-11-07); dedup boundary against sibling card task-2026-11-07t04-39-27 (erratum staging, reported)

curated content beside the label (slot note, verbatim):

    scope boundary honored: staging (04-39-27; staged card errata-v0290-preserved-file-skew.json) untouched by this slice — content defect only; both deadline-bound to the same ceremony, and the fix landed before it

#### A2 item 3/3 — `owner_notes[fnv1a:7df18115d852697a]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:7df18115d852697a] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t06-55-40-erratum-content-fix-v0.29.0-erratum-still-carries-refuted-overlay-same-path-replacement-guidance-line-77#owner_notes[fnv1a:7df18115d852697a] (curated)

- slot provenance (verbatim): recorded on the task card by coordinator intake (/write-task, 2026-11-07); trigger attestation against the committed corrections doc the staged transport points at

curated content beside the label (slot note, verbatim):

    trigger attestation: the defect was live in the committed corrections doc (docs/migration-errata/v0.29.0.md) that the staged transport card points at; fixed at HEAD by commit a362aa0 (staged_path unchanged, still resolving)

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: runbook-fix build session finding (ses_e4b497ad7ffeicF4yXGAsC5e0n, commit 5a9c7852 closeout) — erratum line :77 still carries the refuted overlay-pack same-path replacement claim.
    Dedup: erratum STAGING is done (04-39-27 reported; staged card errata-v0290-preserved-file-skew.json); THIS card is about the CONTENT defect only. Both are deadline-bound to the same ceremony.
    Trigger: fired — defect in a committed corrections doc that staged transport now points at.

### A3. Fix stale-preserve update warning: safeguard-first ordering, correct overlay/project_owned routin…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer

- card file: `task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer.json`
- title: Fix stale-preserve update warning: safeguard-first ordering, correct overlay/project_owned routing, docs pointer
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `89ea161d49001478f0ad993d92756aff684a223c`
- decision_record.version: 3

#### A3 item 1/4 — `owner_notes[fnv1a:73be66305b89f4b9]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer#owner_notes[fnv1a:73be66305b89f4b9] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Provenance: consumer-reachability study findings (task-2026-11-07t04-39-29 closeout, 2026-11-07T07-08-02).

- slot provenance (verbatim): card owner note (verbatim single line); the commit body of 89ea161 cites the two study defects this reword fixes

#### A3 item 2/4 — `owner_notes[fnv1a:0efc2f5a48584b55]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer#owner_notes[fnv1a:0efc2f5a48584b55] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Trigger: fired — defect is live in shipped update output now.

- slot provenance (verbatim): card owner note (verbatim single line); the pre-commit strings (bare-accept-platform header, refuted overlay tail) are visible in the commit diff as removed lines

#### A3 item 3/4 — `owner_notes[fnv1a:c68aca7752766c52]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer#owner_notes[fnv1a:c68aca7752766c52] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Dedup: distinct from 06-55-40 (erratum text) and ship-runbook-docs-key (transport); the three together retire the refuted-overlay-claim surface cluster.

- slot provenance (verbatim): card owner note (verbatim single line)

#### A3 item 4/4 — `measured_outcome[fnv1a:4e23769e82d42a5f]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-46-fix-stale-preserve-update-warning-safeguard-first-ordering-correct-overlay-project_owned-routing-docs-pointer#measured_outcome[fnv1a:4e23769e82d42a5f] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Warning reworded and green: commit 89ea161d49001478f0ad993d92756aff684a223c on main (gate-verified; reviewer approve 4/4 twice — pre-commit review and gate-anchored re-review). Safeguard-first ordering pinned by test on BOTH the warning line and the listing header; refuted overlay same-path route absent from shipped code; routing matches runbook B1/B2 (additions-only overlay pack; project_owned + harness-ownership.yml + re-apply burden for same-path); docs pointer valid at this HEAD (af97e05). Targeted preserved-listing test PASS; go test ./... EXIT 0 (33 ok).

- slot provenance (verbatim): measured at closeout (2026-11-11) from tree-bound receipts; commit body of 89ea161 carries the defect list and receipts summary

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: consumer-reachability study findings (task-2026-11-07t04-39-29 closeout, 2026-11-07T07-08-02).
    Trigger: fired — defect is live in shipped update output now.
    Dedup: distinct from 06-55-40 (erratum text) and ship-runbook-docs-key (transport); the three together retire the refuted-overlay-claim surface cluster.

measured_outcome (verbatim):

    Warning reworded and green: commit 89ea161d49001478f0ad993d92756aff684a223c on main (gate-verified; reviewer approve 4/4 twice — pre-commit review and gate-anchored re-review). Safeguard-first ordering pinned by test on BOTH the warning line and the listing header; refuted overlay same-path route absent from shipped code; routing matches runbook B1/B2 (additions-only overlay pack; project_owned + harness-ownership.yml + re-apply burden for same-path); docs pointer valid at this HEAD (af97e05). Targeted preserved-listing test PASS; go test ./... EXIT 0 (33 ok).

### A4. Ship runbook as `vh-agent-harness docs` key (templates/docs/ embed, zero repo render) + dogfood o…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry

- card file: `task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry.json`
- title: Ship runbook as `vh-agent-harness docs` key (templates/docs/ embed, zero repo render) + dogfood overrides entry
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `af97e058540c29dcacb466d9d0455f20d825c5f4`
- decision_record.version: 3

#### A4 item 1/4 — `owner_notes[fnv1a:520e9d3b4c1f8cb6]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry#owner_notes[fnv1a:520e9d3b4c1f8cb6] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Provenance: consumer-reachability study verdict hybrid (b)+(a′) (task-2026-11-07t04-39-29 closeout).

- slot provenance (verbatim): card owner note (verbatim single line); the referenced study closeout report is local transport, the commit body of af97e05 carries the verdict summary

#### A4 item 2/4 — `owner_notes[fnv1a:19d89f4abb099057]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry#owner_notes[fnv1a:19d89f4abb099057] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Trigger: fired — adopters currently have no consumer-side access to the runbook.

- slot provenance (verbatim): card owner note (verbatim single line); pre-af97e05 adopter inaccessibility is evidenced by the study (option (c) rejected) and the commit's new-key receipts

#### A4 item 3/4 — `owner_notes[fnv1a:2eea86cc15d7af4f]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry#owner_notes[fnv1a:2eea86cc15d7af4f] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Dedup: no existing card ships a docs key for this content; round-2 transport dedup verified by study.

- slot provenance (verbatim): card owner note (verbatim single line)

#### A4 item 4/4 — `measured_outcome[fnv1a:5d9093df3b4e7808]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-26-54-ship-runbook-as-vh-agent-harness-docs-key-templates-docs-embed-zero-repo-render-dogfood-overrides-entry#measured_outcome[fnv1a:5d9093df3b4e7808] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Docs key shipped and green: commit af97e058540c29dcacb466d9d0455f20d825c5f4 on main (gate-verified, reviewer approve 4/4). Key served both via dogfood override and via the embed path against a fresh minimal install (190 lines each); zero consumer-repo render proven (find + update --dry-run receipts); go test ./... + gofmt + vet all green; wantDocKeys pin extended (7 keys).

- slot provenance (verbatim): measured at closeout (2026-11-11) from tree-bound receipts; the commit body of af97e05 carries the receipts summary and inventory policy sentence

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: consumer-reachability study verdict hybrid (b)+(a′) (task-2026-11-07t04-39-29 closeout).
    Trigger: fired — adopters currently have no consumer-side access to the runbook.
    Dedup: no existing card ships a docs key for this content; round-2 transport dedup verified by study.

measured_outcome (verbatim):

    Docs key shipped and green: commit af97e058540c29dcacb466d9d0455f20d825c5f4 on main (gate-verified, reviewer approve 4/4). Key served both via dogfood override and via the embed path against a fresh minimal install (190 lines each); zero consumer-repo render proven (find + update --dry-run receipts); go test ./... + gofmt + vet all green; wantDocKeys pin extended (7 keys).

### A5. Fix README.agent.md render-claim honesty: harness-operator SKILL.md:132 + dangling consumer point…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go

- card file: `task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go.json`
- title: Fix README.agent.md render-claim honesty: harness-operator SKILL.md:132 + dangling consumer pointers (guide.go/root.go/seam.go/execro-classifier.go)
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `9b74cdf88558fb8be3ccd9fcd050fd301c325055`; commit `134798a72dd25dbd02c5b6b6e12147be3aaf869d`
- decision_record.version: 3

#### A5 item 1/4 — `owner_notes[fnv1a:5f7c0f69b7c09520]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go#owner_notes[fnv1a:5f7c0f69b7c09520] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Provenance: consumer-reachability study KEY FACT + corollary defects (task-2026-11-07t04-39-29 closeout): README.agent.md does NOT render into consumers (no templates/**/README.agent.md; classifyCorePath no case, core_manifest.go:161-224; only templates/core renders, corpus.go:29,36-37).

- slot provenance (verbatim): card owner note (verbatim single line); the commit body of 9b74cdf restates the ground fact with the same file:line anchors

#### A5 item 2/4 — `owner_notes[fnv1a:358fd51f25650d92]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go#owner_notes[fnv1a:358fd51f25650d92] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Trigger: fired — shipped skill makes a false claim; consumer-facing pointers dangle.

- slot provenance (verbatim): card owner note (verbatim single line); the removed claim text is visible in the 9b74cdf diff as a deleted line

#### A5 item 3/4 — `owner_notes[fnv1a:25a8555fae6574ec]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go#owner_notes[fnv1a:25a8555fae6574ec] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Dedup: no existing card covers render-claim honesty; cluster distinct from warning/erratum cards.

- slot provenance (verbatim): card owner note (verbatim single line)

#### A5 item 4/4 — `measured_outcome[fnv1a:cc2415dc47d6e70d]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t07-27-04-fix-readme.agent.md-render-claim-honesty-harness-operator-skill.md-132-dangling-consumer-pointers-guide.go-root.go-seam.go-execro-classifier.go#measured_outcome[fnv1a:cc2415dc47d6e70d] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Render-claim honesty fixed and green: commits 9b74cdf (content, 6 files, gate-verified approve 4/4, Task-Card trailer carried) + 134798a (chore(render) ledger sync mirroring 0b56ab6) on main. No shipped surface claims README.agent.md renders into consumer repos (grep receipt: only the corrective negative claim remains); all consumer-facing pointers verified against render tiers (docs/ai/shell-execution.md tracked+platform_managed; overlay docs; runbook docs key af97e05). Scope extended per the card's grep-survey clause: templates/core/.vh-agent-harness/AGENTS.core.md:129 (renders into every consumer repo) repointed. make update 2-overwrite/179-in-sync; make doctor HEALTHY 0 problems; gofmt/vet/go test ./... all green at final HEAD 134798a (fresh -count=1 cli run).

- slot provenance (verbatim): measured at closeout (2026-11-11) from tree-bound receipts at final HEAD 134798a; the content commit 9b74cdf carries the claim-removal and pointer-retarget facts, the chore(render) commit 134798a carries the rendered bytes + ledger

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: consumer-reachability study KEY FACT + corollary defects (task-2026-11-07t04-39-29 closeout): README.agent.md does NOT render into consumers (no templates/**/README.agent.md; classifyCorePath no case, core_manifest.go:161-224; only templates/core renders, corpus.go:29,36-37).
    Trigger: fired — shipped skill makes a false claim; consumer-facing pointers dangle.
    Dedup: no existing card covers render-claim honesty; cluster distinct from warning/erratum cards.

measured_outcome (verbatim):

    Render-claim honesty fixed and green: commits 9b74cdf (content, 6 files, gate-verified approve 4/4, Task-Card trailer carried) + 134798a (chore(render) ledger sync mirroring 0b56ab6) on main. No shipped surface claims README.agent.md renders into consumer repos (grep receipt: only the corrective negative claim remains); all consumer-facing pointers verified against render tiers (docs/ai/shell-execution.md tracked+platform_managed; overlay docs; runbook docs key af97e05). Scope extended per the card's grep-survey clause: templates/core/.vh-agent-harness/AGENTS.core.md:129 (renders into every consumer repo) repointed. make update 2-overwrite/179-in-sync; make doctor HEALTHY 0 problems; gofmt/vet/go test ./... all green at final HEAD 134798a (fresh -count=1 cli run).

### A6. eslink: dynamic-import escaped-literal specifier skips the not-decodable guard (discriminator gap)

**card-id anchor** (verbatim, single line):

    defer-eslink-dynimport-escaped-literal

- card file: `defer-eslink-dynimport-escaped-literal.json`
- title: eslink: dynamic-import escaped-literal specifier skips the not-decodable guard (discriminator gap)
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `f9352e43f03959f3e6bf5ae8cc91ea1ce47fe3ca`
- decision_record.version: 3

#### A6 item 1/3 — `owner_notes[fnv1a:b493636684498839]`

decision-item label line (canonical shape):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:b493636684498839] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:b493636684498839] (curated)

- slot provenance (verbatim): cascade-3 commit-review leaf tier1_c F1 at d5882ded (severity major, DEFER/non-gating) — the finding this card executed; its substance is carried by commit f9352e43's message

#### A6 item 2/3 — `owner_notes[fnv1a:1fa0ae48fd63006b]`

decision-item label line (canonical shape):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:1fa0ae48fd63006b] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:1fa0ae48fd63006b] (curated)

- slot provenance (verbatim): path_touched(internal/eslink/parser.go) trigger fired at card ready-time; parser.go was touched and landed in commit f9352e43

#### A6 item 3/3 — `owner_notes[fnv1a:5bbcae5a605f994b]`

decision-item label line (canonical shape):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:5bbcae5a605f994b] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: defer-eslink-dynimport-escaped-literal#owner_notes[fnv1a:5bbcae5a605f994b] (curated)

- slot provenance (verbatim): studied:2026-11-09 study stamp from cascade-3 triage; execution landed 2026-11-08/09 in commit f9352e43

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    source:review-defer (cascade-3 commit-review, leaf tier1_c F1, commit d5882ded; re-flagged as the substantive new finding — severity major, disposition DEFER/non-gating)
    trigger:path_touched(internal/eslink/parser.go)
    studied:2026-11-09

### A7. eslink fixture-hardening batch: composed-chain/optional-chain/U+2028 fixtures, golden source-span…

**card-id anchor** (verbatim, single line):

    task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes

- card file: `task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes.json`
- title: eslink fixture-hardening batch: composed-chain/optional-chain/U+2028 fixtures, golden source-spans, assertDelta multiset, isSeqRoot + coverReason micro-fixes
- refusal class at last retirement attempt: `handle_not_in_carrier`
- original carriers declared by the slots: commit `c3f302ca4861b5fd96491a3983c1f3964450b99c`
- decision_record.version: 3

#### A7 item 1/3 — `owner_notes[fnv1a:81c6cc45397d0942]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:81c6cc45397d0942] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:81c6cc45397d0942] (curated)

- slot provenance (verbatim): S1' cascade-3 + closeout curation queue provenance (commit d5882ded, coordinator triage m0125, operator 'all' m0126) — the batch this card executed; substance carried by commit c3f302ca's message

#### A7 item 2/3 — `owner_notes[fnv1a:22401da0af1fab15]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:22401da0af1fab15] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:22401da0af1fab15] (curated)

- slot provenance (verbatim): trigger fired (eslink at HEAD; hardening before S2-S4 wiring) — satisfied: hardening landed at c3f302ca

#### A7 item 3/3 — `owner_notes[fnv1a:630840e4ad328757]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:630840e4ad328757] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-08t18-32-17-eslink-fixture-hardening-batch-composed-chain-optional-chain-u-2028-fixtures-golden-source-spans-assertdelta-multiset-isseqroot-coverreason-micro-fixes#owner_notes[fnv1a:630840e4ad328757] (curated)

- slot provenance (verbatim): dedup note (t1a-F3 excluded as the separate binary-in card, executed first as 60d28c8; remaining findings collapsed here per drain-first)

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: S1′ cascade-3 + closeout curation queue (commit d5882ded, closeout 2026-11-08T19-xx; coordinator triage m0125, operator 'all' m0126).
    Trigger: fired — eslink landed at HEAD; hardening rides before any S2-S4 wiring.
    Dedup: t1a-F3 excluded (duplicate of ready card); all other uncarded findings collapsed here per drain-first.

## Class B — `no_trailer_match` (4 cards, 12 items)

Cards whose original carrier commits are trailer-less by design, so the
landing-proof gate found no reachable commit carrying the exact
`Task-Card: <card-id>` trailer line. Each section below carries the
trailer-form line, the card-id anchor, and the decision-record items
verbatim; the landing commit of this checkpoint carries the four exact
trailer lines in its message body.

### B1. Stage migration notice + erratum for state-lib.js hub-token normalization (4469f3c) ahead of next…

**card-id anchor** (verbatim, single line):

    task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release

**Task-Card trailer-form line** (verbatim; the landing commit of
this checkpoint carries the same line in its message):

    Task-Card: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release

- card file: `task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release.json`
- title: Stage migration notice + erratum for state-lib.js hub-token normalization (4469f3c) ahead of next release
- refusal class at last retirement attempt: `no_trailer_match`
- original carriers declared by the slots: artifact `docs/checkpoints/2026-11-11-hub-token-normalization-erratum-staged.md`
- decision_record.version: 3

#### B1 item 1/4 — `owner_notes[fnv1a:d6f36b810698e246]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release#owner_notes[fnv1a:d6f36b810698e246] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Provenance: stage-4 decision record researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md item 3 (delivery gate: migration notice/erratum MANDATORY, 7-element content enumerated); hub churn landed 4469f3c + 0b56ab6.

- slot provenance (verbatim): Owner note recorded on the task card at creation (2026-11-11T18:10:20Z, coordinator session); carried verbatim into the carrier checkpoint by execution session erratum-stage-hub-token at closeout (2026-11-11).

#### B1 item 2/4 — `owner_notes[fnv1a:462ecde4b7a4df51]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release#owner_notes[fnv1a:462ecde4b7a4df51] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Trigger: fired — docs legs of the delivery gate complete (af97e05/89ea161/9b74cdf+134798a); this is the last gate element.

- slot provenance (verbatim): Owner note recorded on the task card at creation (2026-11-11T18:10:20Z, coordinator session); carried verbatim into the carrier checkpoint by execution session erratum-stage-hub-token at closeout (2026-11-11).

#### B1 item 3/4 — `owner_notes[fnv1a:8f9a4508b1dc4099]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release#owner_notes[fnv1a:8f9a4508b1dc4099] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Dedup: distinct from errata-v0290-preserved-file-skew (that corrects the v0.29.0 note's preservation promises; this notices the upcoming hub churn).

- slot provenance (verbatim): Owner note recorded on the task card at creation (2026-11-11T18:10:20Z, coordinator session); carried verbatim into the carrier checkpoint by execution session erratum-stage-hub-token at closeout (2026-11-11).

#### B1 item 4/4 — `measured_outcome[fnv1a:0fa186571c8f013e]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release#measured_outcome[fnv1a:0fa186571c8f013e] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    Committed docs/migration-errata/v0.29.0-hub-token-normalization.md at 05a16d7 (one commit, parent 134798a, zero tags, no push, tree clean) carrying all 7 mandated elements verbatim-grep-checkable (22/22 strings).

- slot provenance (verbatim): Measured outcome of this execution session (2026-11-11); full text carried verbatim into the carrier checkpoint.

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

owner_notes entries (verbatim):

    Provenance: stage-4 decision record researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md item 3 (delivery gate: migration notice/erratum MANDATORY, 7-element content enumerated); hub churn landed 4469f3c + 0b56ab6.
    Trigger: fired — docs legs of the delivery gate complete (af97e05/89ea161/9b74cdf+134798a); this is the last gate element.
    Dedup: distinct from errata-v0290-preserved-file-skew (that corrects the v0.29.0 note's preservation promises; this notices the upcoming hub churn).

measured_outcome (verbatim):

    Committed docs/migration-errata/v0.29.0-hub-token-normalization.md at 05a16d7 (one commit, parent 134798a, zero tags, no push, tree clean) carrying all 7 mandated elements verbatim-grep-checkable (22/22 strings). Staged card .local/coordinator/tasks/errata-v0290-hub-token-normalization.json (status=staged, staged_path → the committed notice, mirrors errata-v0290-preserved-file-skew.json exactly; v0290 card untouched — both still staged). Discovery proven without a ceremony: real-repo inject-errata --dry-run reaches the held-erratum/no-note error (exit 1, reachable only with non-empty staged set); fixture rehearsal with the make-built binary (0.29.0+dev) and byte-identical FINAL notice bytes listed BOTH staged cards as would-inject into an untagged v0.30.0 note, live-injected both + flipped both staged→completed, re-run idempotent no-op; fixture doctor #13 FAIL-names both staged cards pre-inject → PASS post-inject; real-repo doctor #13 SKIP (designed steady state, no untagged note). Gates: TestMigrationNotes ok, go test ./... 33/33 packages ok, make test-js 234/234 pass. Delivery gate of researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md item 3 now complete.

### B2. Release ceremony: inject docs/migration-errata/v0.29.0.md into next release note

**card-id anchor** (verbatim, single line):

    task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note

**Task-Card trailer-form line** (verbatim; the landing commit of
this checkpoint carries the same line in its message):

    Task-Card: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note

- card file: `task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note.json`
- title: Release ceremony: inject docs/migration-errata/v0.29.0.md into next release note
- refusal class at last retirement attempt: `no_trailer_match`
- original carriers declared by the slots: artifact `docs/checkpoints/2026-11-07-erratum-v0290-staged-transport.md`
- decision_record.version: 3

#### B2 item 1/2 — `open_questions[fnv1a:45688b74012cf57b]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note#open_questions[fnv1a:45688b74012cf57b] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note#open_questions[fnv1a:45688b74012cf57b] (curated)

- slot provenance (verbatim): Answer derived from code (internal/cli/release_inject.go stagedErrataCards over claims.Derive; internal/memory/claims claim.go loadDeferCards errata-*.json scan) and the committed ceremony doc (.opencode/agents/releaser.md inject step, unconditional after note authoring); verified by the dry-run receipts in this report.

#### B2 item 2/2 — `measured_outcome[fnv1a:b941983cf3b7dec1]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note#measured_outcome[fnv1a:b941983cf3b7dec1] (verbatim)

gate-quoted handle (verbatim, disposition `verbatim`):

    real-repo dry-run before/after contrast (no-op → held-erratum/no-note error, reachable only when the staged set is non-empty)

- slot provenance (verbatim): Line drawn verbatim from the measured_outcome; full outcome in this report and summarized in the checkpoint artifact.

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

open_questions entries (verbatim):

    Does the standing ceremony auto-discover this card at the N step, or must the releaser be handed it explicitly?

measured_outcome (verbatim):

    v0.29.0 erratum staged into the ceremony's own discovery transport and machine-discovered. Staged: .local/coordinator/tasks/errata-v0290-preserved-file-skew.json (status=staged, staged_path=docs/migration-errata/v0.29.0.md — the committed erratum file itself, the machinery-canonical shape per writeStagedErrataCard/releaser.md; committed file untouched). Proven: real-repo dry-run before/after contrast (no-op → held-erratum/no-note error, reachable only when the staged set is non-empty); fixture rehearsal (real binary, byte-identical erratum, untagged v0.30.0 note) listed "would inject section for v0.29.0 into v0.30.0.md; would flip card staged→completed", live flip + idempotent re-run; doctor #13 SKIPs in-repo (no imminent release — by design) and FAIL-names the staged card in the imminent-release fixture. Gates: TestMigrationNotes ok, go test ./... all ok, internal/cli -count=1 ok (95s). NO commit — both artifacts are gitignored transport; the machinery demands no committed location (mission STOP condition did not fire). Card's open question answered: the ceremony auto-discovers the card at the N-step (releaser.md inject step runs the subcommand unconditionally; no hand-off needed). Ceremony-time follow-up (not this slice): inject-errata flips the card staged→completed, then delete the erratum file + retire the card per the erratum's own blockquote. Full receipts in the closeout report under .local/coordinator/reports/task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note/.

### B3. Runbook consumer-reachability: adopters cannot see docs/ai/preserved-managed-reconciliation.md

**card-id anchor** (verbatim, single line):

    task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md

**Task-Card trailer-form line** (verbatim; the landing commit of
this checkpoint carries the same line in its message):

    Task-Card: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md

- card file: `task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md.json`
- title: Runbook consumer-reachability: adopters cannot see docs/ai/preserved-managed-reconciliation.md
- refusal class at last retirement attempt: `no_trailer_match`
- original carriers declared by the slots: artifact `researches/sources/2026-11-07-runbook-consumer-reachability-transport.md`
- decision_record.version: 3

#### B3 item 1/2 — `open_questions[fnv1a:2570797be9a0f67c]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#open_questions[fnv1a:2570797be9a0f67c] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#open_questions[fnv1a:2570797be9a0f67c] (curated)

- slot provenance (verbatim): answered by this study (2026-11-07): README.agent.md does not render; no round-2 transport card existed; inlining alone insufficient without a durable full-procedure source — curated answer to be committed in the named artifact (pending write-permitted slice)

#### B3 item 2/2 — `measured_outcome[fnv1a:34f46d1e8b814093]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#measured_outcome[fnv1a:34f46d1e8b814093] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t04-39-29-runbook-consumer-reachability-adopters-cannot-see-docs-ai-preserved-managed-reconciliation.md#measured_outcome[fnv1a:34f46d1e8b814093] (curated)

- slot provenance (verbatim): study verdict (2026-11-07): hybrid (b)+(a′) recommended, (a) and (c) rejected with file:line evidence — curated summary to be committed in the named artifact (pending write-permitted slice)

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

open_questions entries (verbatim):

    Does README.agent.md render into consumer repos? Does a round-2 transport card already cover this? Is inlining into the warning text sufficient without a rendered runbook?

measured_outcome (verbatim):

    Transport verdict delivered: README.agent.md does NOT render into consumer repos (no corpus copy, no writer code); recommended hybrid = reword the stale-preserve warning to safeguard-first with correct routing + ship the runbook as a vh-agent-harness docs key (binary-carried, zero repo render); full consumer-side render rejected as per-install tree cost; dev-repo-only reliance rejected as broken. Evidence: researches/sources/2026-11-07-runbook-consumer-reachability-transport.md

### B4. O4 cohort-consistency: frame adjudication research (detection-refusal vs qualification-admission;…

**card-id anchor** (verbatim, single line):

    task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check

**Task-Card trailer-form line** (verbatim; the landing commit of
this checkpoint carries the same line in its message):

    Task-Card: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check

- card file: `task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check.json`
- title: O4 cohort-consistency: frame adjudication research (detection-refusal vs qualification-admission; plan-seam, Go ES-module link check, semantic check, qualification policy)
- refusal class at last retirement attempt: `no_trailer_match`
- original carriers declared by the slots: artifact `researches/decisions/2026-11-07-cohort-consistency-preflight-feasibility.md`
- decision_record.version: 3

#### B4 item 1/4 — `measured_outcome[fnv1a:2c9cb433c9e1f7d4]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#measured_outcome[fnv1a:2c9cb433c9e1f7d4] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#measured_outcome[fnv1a:2c9cb433c9e1f7d4] (curated)

- slot provenance (verbatim): memo §Verification + §Frame Verdict — all four questions answered with evidence; closeout gate exercised live (refuse → activate → declare)

#### B4 item 2/4 — `open_questions[fnv1a:49c2c09ec52ea363]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:49c2c09ec52ea363] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:49c2c09ec52ea363] (curated)

- slot provenance (verbatim): memo §FRAME VERDICT — ship detection m0034 D1/D2/D3 + honesty rider; reject qualification m0038 lock-in-in-effect; operator ratifies

#### B4 item 3/4 — `open_questions[fnv1a:276a410b7606f844]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:276a410b7606f844] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:276a410b7606f844] (curated)

- slot provenance (verbatim): memo §FRAME VERDICT → flip-condition audit — detection retains: materially less friction yes; adequate coverage yes under comparative reading (assumption labeled, fallback outcome named)

#### B4 item 4/4 — `open_questions[fnv1a:80a105e7b578ae50]`

decision-item label line (canonical shape):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:80a105e7b578ae50] (curated)

gate-quoted handle (verbatim, disposition `curated`):

    decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:80a105e7b578ae50] (curated)

- slot provenance (verbatim): memo §FRAME VERDICT → flip-condition audit — qualification: independence passes, recovery passes, lock-in condition fires in effect via Q4(d)+(f), labeled inference

source decision-item content from the card fields (verbatim —
verbatim handles are single lines of these texts):

open_questions entries (verbatim):

    FRAME VERDICT (memo output, operator ratifies): which frame ships — detection (proven-incompatibility refusal; contingent policy = m0034 D1 hybrid + D2 pre-write refusal w/ --allow-broken-cohort + D3 separate doctor FAIL) vs qualification admission (guarantee-scoped qualification; contingent policy = m0038 D1' qualified/proven-incompatible/unattested tri-state + D2' refuse unqualified ACTIVATION w/ override-keeps-UNATTESTED + D3' assurance-FAIL naming missing-assurance)? Evidence decides via both frames' flip conditions.
    Detection-frame flip: retain detection if Q3 demonstrates adequate actual-failure coverage with materially less friction.
    Qualification-frame flips: reject qualification if it lacks independence from preserved JS, becomes blanket template lock-in, or cannot support safe refuse->override->reconcile->rerun recovery.

measured_outcome (verbatim):

    All four gating questions answered with verdicts + evidence. Frame verdict delivered as flip-condition adjudication table with full contingent policy package, architecture sketch, new-surface/inventory analysis, and contradiction audit vs origin-hash-update-sync.md (none detected, two pre-existing flags). Closeout gate itself exercised live: initial refusal (card ready-not-working, unbound session, missing decision-record), then activate, then retry with decision-record.

## Provenance of this transcription

- Source: the eleven held cards in `.local/coordinator/tasks/` (status
  `completed`), read 2026-11-14. Strings copied programmatically from the
  card JSON by the slice that authored this checkpoint.
- Gate contract: `docs/coordination/RECORD_LIFECYCLE.md` (retention-
  preservation gate v3; retrievability; refusals at retirement).
- Retirement re-runs are the coordinator's step — no cards were deleted
  by landing this checkpoint.

