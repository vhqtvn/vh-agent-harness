// Package redlines implements the machine-local "private redlines" capability:
// never-committed, cross-project sensitivity knowledge that applies to every
// repository on a machine with no per-repo setup.
//
// Two subject kinds are supported:
//
//   - scrub-project: a project whose material must be scrubbed before anything
//     derived from it lands in git (matched by labels/aliases).
//   - forbidden-relation: two term-sets whose co-occurrence in a committed
//     artifact would leak a private relation, including an AMBIENT case where a
//     repo's identity implies one side so the other side's terms are banned
//     outright within that repo.
//
// # Security invariants (non-negotiable)
//
// The registry holds private terms. Nothing the package emits — no error, no
// diagnostic, no log line — may contain a real term, a label, a term-set, or a
// subject's `why` text. The ONLY token safe to echo anywhere is a subject's
// OPAQUE id, which is structurally constrained to the form `subj-<chars>`.
// Every error string in this package is opaque by construction: it references
// paths, opaque ids, and generic reason codes, never sensitive content.
//
// All test fixtures use OBVIOUSLY synthetic terms (e.g. `synthetic-alpha`,
// `subj-test-scrub`). No real registry entry is authored by this package or its
// tests; the operator authors the real registry manually under their XDG dir.
//
// # Architecture: always-compiled, machine-registry-activated, otherwise inert
//
// The capability ships compiled into the binary. When no user-level registry
// exists, or when the registry binds no subject to the current repo, every
// entry point is a complete NO-OP: it returns success, produces byte-empty
// output, and creates no files (the "zero-footprint" property). A present-but-
// invalid or unreadable registry FAILS CLOSED with an opaque error.
//
// The commit gate (templates/core/.opencode/scripts/commit-gate.sh) is the sole
// authoritative enforcement surface; a scanner running there MUST scan the
// EXACT object the gate authorizes. The exact-object locator contract is
// settled in target.go (Slice 0): an immutable git tree hash.
//
// # Layering (historical slices, all shipped)
//
// This file documents the package. The capability is implemented across these
// files, all of which ship together in the current binary:
//
//   - target.go: exact staged-object locator contract — ScanTarget +
//     NewScanTarget. Pure, no I/O, no matching, no gate wiring.
//   - registry.go, binding.go, security.go: XDG discovery, schema
//     validation, repo binding, secure-file posture. Read-only; inert when
//     absent/non-binding.
//   - scanner.go: the pure lexical matching engine (scrub labels, forbidden-
//     relation co-occurrence, ambient degeneration, binary/oversized skip,
//     dedup, deterministic sort).
//   - The `redlines scan` command (internal/cli/redlines_scan.go) and the
//     commit-gate integration (commit-gate.sh invokes `redlines scan` on the
//     exact acquired tree) are the enforcement surfaces. `redlines guidance`
//     (internal/cli/redlines.go) is the local agent-context channel. Doctor
//     surfaces a registry file-permission WARN. All ship in the same binary.
package redlines
