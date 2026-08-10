---
type: decision
date: 2026-08-10
scope: composition pipeline — byte-stable banding/ordering guarantees
status: candidate-recorded (operator cross-check P2)
source-basis: origin-hash work (16c69f8f, aa6641c)
---

# Byte-Stable Banding Order — Decision Memo

## Decision statement

**QUESTION:** Should the harness guarantee a byte-stable banding/ordering (e.g., file-hash banding, manifest ordering) for cache predictability and deterministic diffs across renders?

This is a recommendation the operator will decide on; the memo body does not speak as live repo policy.

## Decision context

The origin-hash work (commits `16c69f8f` and `aa6641c`) introduced hash-band file tracking to the harness. The relevant surfaces for this logic are `internal/drift/drift.go` and `internal/originhash/`, which dictate how file states are computed, compared, and reported. 

A lack of strict sorting or banding order during generation can lead to changing bytes in manifests even when there are zero semantic changes. This causes spurious cache misses and noisy, non-deterministic `git diff` outputs.

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided evidence flags "byte-stable banding order as a composition rule" as a P2 candidate. All three consumers currently render the output of the harness, making them vulnerable to any non-determinism in the generated composition output. The operator specifically requested evaluating if this needs immediate enforcement.

## This-harness posture (read by this researcher)

The harness currently tracks drift via `internal/originhash/` and `internal/drift/`. The posture today is that file contents and paths define the hash band, but the harness does not strictly guarantee lexical or byte-stable output ordering across all manifests and file concatenations. If a map is iterated without sorting, or concurrent generation outputs non-deterministically, the resulting bytes shift.

## Options considered

- **OPT-A — Adopt byte-stable banding immediately:** Enforce strict deterministic sorting and banding (file-hash banding, manifest ordering) across all generated files and manifests. This involves auditing the composition pipeline for non-deterministic map iterations or parallel output accumulation.
- **OPT-B — Document current ordering and defer (RECOMMENDED):** Document the current ordering contract. Wait to adopt strict byte-stable banding until a non-stable ordering is observably causing real-world cache misses or blocking non-deterministic diffs in consumer repos.

## Recommendation

**OPT-B.** Borrow/adopt byte-stable banding IF and ONLY IF a non-stable ordering is observably causing cache misses or non-deterministic diffs; otherwise, document the current ordering contract and defer.

## Findings

- **(finding)**: source=internal/originhash/, confidence=high, type=fact — The harness currently implements hash-band file tracking to assess drift.
- **(inference)**: source=operator-cross-check, confidence=medium, type=inference — The lack of severe cache miss reports suggests the current rendering order is largely deterministic by default, or the existing non-determinism has not yet surfaced as a critical blocker.

## Contradictions

- None detected in the covered scope.

## Risks / open-questions

- Do any of the three consumers currently experience spurious diffs on `vh-agent-harness update` that we haven't formally linked to non-deterministic banding? 
- Will deferred enforcement lead to a buildup of hidden non-deterministic code that becomes expensive to unravel later?