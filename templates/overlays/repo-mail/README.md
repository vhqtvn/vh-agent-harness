# repo-mail overlay pack

The repo-mail inter-repo communication protocol overlay. See the record-of-
decision: `researches/decisions/2026-07-26-repo-mail-protocol.md`.

## Design context (project delivery narrative — relocated from the gate header)

The core egress gate (`templates/core/.opencode/repo-configs/repo-mail-egress-gate.js`)
is shipped **domain-free**: it carries no project delivery narrative or internal
canon citations, because `corpus.go` embeds it into every distributed binary.
That provenance lives HERE (the project-owned overlay surface) instead:

- **This is Slice 2 of the repo-mail protocol — the Caveat-1 extension.** The
  core gate is the domain-free matcher extension Caveat 1 of the decision memo
  calls for. Per the memo (verbatim):

  > "The present generic matcher is command-oriented and is not already an
  > arbitrary-message anonymization validator... Slice 2 will likely need a
  > domain-free matcher extension to validate arbitrary message scalars, not
  > just command strings."

- The gate's four anonymization invariants, the REJECT-not-transform spine, and
  the send-authorization (`scrub.result`) verify model correspond to the memo's
  **Contract C** (client egress gate, fail-closed) and **Contract A** (canonical
  envelope field schema). The identifier allow-list encodes Contract A's closed
  field set. **Decision 2** (Non-Actuation by Construction) is honored: the gate
  is a pure validator with no actuation vocabulary. The channel-scoped
  directional-authorization front (the O1 audit's ADAPTABLE item) is a later
  slice, not this one.

See the decision memo for the full contract text, the failure-mode table, and
the caveats.

## What this pack provides (Slice 2)

**`plugins/repo-mail-egress-wiring.js`** — the integration that wires the
generic domain-free egress gate (shipped in `templates/core`) to the shared
`scrubCredentials` helper (shipped in the `auto-classifier-pilot` pack).

This pack contains ONLY the integration wiring. The gate CONTRACT + GENERIC
deny-rules live in `templates/core/.opencode/repo-configs/repo-mail-egress-gate.js`
(domain-free, no vendor/repo/endpoint identity — that is the publishable
contract). This overlay binds that contract to the real scrub helper.

## The load-bearing invariant — REJECT-not-transform

A message carrying a repo identifier / credential / endpoint is **REFUSED**,
never scrubbed-and-sent. `scrubCredentials` is used as a **detector**: if
applying it would change a scalar, that scalar carries credential-shaped
content → the send is REJECTED. The canonical bytes returned on a pass are the
ORIGINAL unmodified serialization; on a reject, nothing is sent.

## Gate enforce-order (per Contract C / memo egress spine D1)

```
canonicalize → forbidden-patterns (generic + private-ext)
             → scrubCredentials any-mutation check (detector only)
             → identifier allow-list (channel-id/class/key-id opaque format)
             → pass / REJECT (fail-closed on any uncertainty)
```

## Selecting this pack

Add `repo-mail` to `.vh-agent-harness/vh-harness-profile.yml`:

```yaml
overlays:
  - auto-classifier-pilot   # provides scrubCredentials
  - repo-mail               # provides the egress-gate wiring (this pack)
```

Then run `make update` to render the wiring unit into `.opencode/plugins/`.

## Self-tests

Both units carry a dual-purpose `node:test` self-test (run directly):

```
node templates/core/.opencode/repo-configs/repo-mail-egress-gate.js   # gate (stub scrub)
node templates/overlays/repo-mail/plugins/repo-mail-egress-wiring.js  # wiring (real scrub)
```

The gate self-test uses a stub `scrubCredentials` for isolation; the wiring
self-test imports the REAL `auto-gate-scrub.js` helper and proves the
composition end-to-end in both directions (dirty → rejected; clean → passes).

## Private deny-rules

A project's private deny-rules (its own repo name, feature names, specific
endpoints) are loaded by `loadPrivateDenyRules(modulePath)` from a gitignored
JS module that exports `REPO_MAIL_DENY_RULES`. They are composed ON TOP of the
generic core rules. The generic layer is best-effort (catches identity-leak
SHAPES: URLs, git remotes, emails, home-dir paths); the private layer catches
project-specific names the generic layer cannot know.

## Scope (this slice)

This slice is ONLY the client-side fail-closed egress gate over canonical
bytes. Channel-scoped directional authorization, key exchange, carrier-side
auth, and the terminal operator projection are LATER slices (Slice 5/6), not
here.
