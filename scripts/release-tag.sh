#!/usr/bin/env bash
# scripts/release-tag.sh — sanctioned release-tag wrapper for vh-agent-harness.
#
# This script is the ONLY sanctioned surface that runs `git tag` in this repo.
# The releaser agent (and any operator) invokes it to create an annotated git
# tag for a release. Agents must NEVER call `git tag` / `git push` directly —
# shell-guard's `git-mutation-bypass` rule denies those verbs to every agent;
# this wrapper is the sole sanctioned mutation path for cutting a release tag.
#
# It is PROJECT-LOCAL infra for this dogfood repo. It is NOT part of the
# domain-free embedded corpus (templates/core/) and is not shipped into other
# projects by the harness.
#
# Usage (local-only annotated tag, the default — does NOT push):
#   RELEASE_TAG_MESSAGE_FILE=tmp/release-tag-msg-v0.2.0.txt scripts/release-tag.sh v0.2.0
#
# Usage (opt-in push of the new tag to origin after creating it):
#   RELEASE_TAG_PUSH=1 RELEASE_TAG_MESSAGE_FILE=tmp/release-tag-msg-v0.2.0.txt scripts/release-tag.sh v0.2.0
#
# Usage (push-only — push an already-cut local tag through the sanctioned wrapper):
#   scripts/release-tag.sh v0.2.0 --push-only
#   Inverts the tag-existence check (the tag MUST already exist locally and
#   MUST be an annotated tag object), skips the DEFER gate, and skips the
#   `git tag -a` mutation. If the tag is missing, the wrapper refuses with
#   `"tag <v> does not exist; cannot push-only"` (prefix of the full stderr
#   line, which also names the remedy). If the tag exists but is a lightweight
#   tag, the wrapper refuses with `"... is not an annotated tag object;
#   push-only requires an annotated tag ..."`. Use this to push a tag that
#   was cut by a prior create-only invocation so a push-only slice never
#   needs raw `git push` (forbidden by the git-mutation-bypass rule). Cannot
#   be combined with --override-* flags.
#
# Usage (release with explicit operator override ceremony):
#   RELEASE_TAG_MESSAGE_FILE=tmp/release-tag-msg-v0.13.0.txt \
#   scripts/release-tag.sh v0.13.0 \
#       --override-release-version v0.13.0 \
#       --override-manifest-sha <blob-sha-of-.vh-agent-harness/release-defer-dispositions.json>
#
# Arguments / environment:
#   $1                          -> version, must match ^v[0-9]+\.[0-9]+\.[0-9]+$
#   --override-release-version  -> optional; explicit operator confirmation of
#                                  the release version bound to an override.
#                                  Must equal $1. Requires --override-manifest-sha.
#   --override-manifest-sha     -> optional; explicit operator confirmation of
#                                  the manifest blob SHA being overridden.
#                                  Must equal `git rev-parse HEAD:<path>` of
#                                  the committed manifest (NOT the worktree
#                                  file's hash-object). Requires
#                                  --override-release-version.
#   $RELEASE_TAG_MESSAGE_FILE   -> path to the annotated tag message file (must
#                                  exist and be non-empty); passed to `git tag -a -F`
#   $RELEASE_TAG_PUSH           -> optional; set to "1" to also `git push origin <version>`
#   --push-only                 -> optional POSITIONAL FLAG (after <version>); push an
#                                  already-cut local tag to origin WITHOUT re-running
#                                  the DEFER gate or the `git tag -a` mutation.
#                                  Requires the tag to exist locally. Cannot be
#                                  combined with --override-* (the DEFER gate is
#                                  skipped in push-only mode).
#
# Output: exactly ONE valid JSON object on stdout, nothing else.
#   success: {"ok":true,"tag":"vX.Y.Z","commit":"<HEAD-sha>","pushed":false,
#             "error":null,"disclosures":[...],"accepted_overrides":[...]}
#   refusal: {"ok":false,"tag":"vX.Y.Z|null","commit":"<sha>|null","pushed":false,
#             "error":"<reason>","disclosures":[...],"accepted_overrides":[...]}
# `disclosures` and `accepted_overrides` are ALWAYS present (both `null` in
# --push-only mode, which skips the DEFER gate; otherwise they carry the
# evaluator's disclosures and any operator-approved overrides).
# On any validation failure the script prints the refusal JSON and exits non-zero.
# Refuses (non-zero) if the tag already exists (create flow only; push-only
# INVERTS this check), so re-running after a failure is safe.
#
# Disclosures and accepted overrides are also printed to stderr BEFORE the
# `git tag -a` mutation so the operator can observe exactly what will be
# released. The same data is embedded in the final JSON for machine consumers.
#
# Two distinct failure classes (the wrapper surfaces both explicitly):
#   (a) "release-relevant finding requires disposition" — classification=blocker.
#       Remedy: resolve the finding OR supply the override ceremony
#       (--override-release-version + --override-manifest-sha).
#   (b) "manifest missing/malformed/stale" — classification=evaluator-error.
#       Remedy: repair the committed manifest (override is inapplicable).
set -euo pipefail

# --- helpers (no stdout except the final emit) ---

json_str() {
  # Print a JSON string literal for $1, escaping \, ", and ALL C0 control chars
  # (U+0000-U+001F) per RFC 8259 §7, or the bare word null if empty. Named
  # control chars (\b \t \n \f \r) use the JSON shorthand; every other C0 byte
  # uses \u00XX. (U+0000 itself cannot occur in a bash variable.) This matters
  # because raw git stderr or an operator-supplied env value can flow into the
  # error field, and a literal control byte inside a JSON string is invalid.
  local v="${1-}"
  if [ -z "$v" ]; then
    printf 'null'
    return
  fi
  # 1) backslash, double-quote, and named control chars via parameter expansion
  v="${v//\\/\\\\}"
  v="${v//\"/\\\"}"
  v="${v//$'\b'/\\b}"
  v="${v//$'\t'/\\t}"
  v="${v//$'\n'/\\n}"
  v="${v//$'\f'/\\f}"
  v="${v//$'\r'/\\r}"
  # 2) catch-all: any remaining C0 control char (0x01-0x1F) -> \u00XX
  local i=0 n=${#v} c code out=""
  while [ "$i" -lt "$n" ]; do
    c="${v:i:1}"
    code=$(printf '%d' "'$c" 2>/dev/null || true)
    if [ -n "$code" ] && [ "$code" -ge 0 ] 2>/dev/null && [ "$code" -lt 32 ]; then
      out+=$(printf '\\u%04x' "$code")
    else
      out+="$c"
    fi
    i=$((i+1))
  done
  printf '"%s"' "$out"
}

emit() {
  # ok tag commit pushed error disclosures accepted_overrides
  # (booleans as bare true/false; nulls via json_str; disclosures/overrides are
  # pre-rendered JSON array literals, or the literal "null")
  printf '{"ok":%s,"tag":%s,"commit":%s,"pushed":%s,"error":%s,"disclosures":%s,"accepted_overrides":%s}\n' \
    "$1" "$(json_str "$2")" "$(json_str "$3")" "$4" "$(json_str "$5")" "${6:-null}" "${7:-null}"
}

VERSION="${1-}"
MSG_FILE="${RELEASE_TAG_MESSAGE_FILE-}"
OVERRIDE_RELEASE_VERSION=""
OVERRIDE_MANIFEST_SHA=""
MANIFEST_REL=".vh-agent-harness/release-defer-dispositions.json"

# --- pre-scan for --push-only ---
#
# --push-only is the explicit operator intent for pushing an already-cut
# local annotated tag through the sanctioned wrapper, removing the need
# for agents to fall back to raw `git push` (which is forbidden by the
# git-mutation-bypass rule). When set, the wrapper:
#   - skips RELEASE_TAG_MESSAGE_FILE validation (it is not creating a tag)
#   - INVERTS the tag-existence check: the tag MUST already exist locally
#     (cut by a prior create-only invocation); refuse if missing
#   - skips the override ceremony, the DEFER gate, and the `git tag -a`
#     mutation, going straight to `git push origin <version>`
#   - emits the same JSON contract with disclosures=null and
#     accepted_overrides=null
# The DEFER gate is skipped because the tag already passed it at creation
# time — push-only trusts the existing annotated tag object. Explicit flag
# (not implicit RELEASE_TAG_PUSH + existing-tag) because accidental pushes
# are a real risk and explicit intent is safer than magic.
PUSH_ONLY=0
for _a in "$@"; do
  if [ "$_a" = "--push-only" ]; then
    PUSH_ONLY=1
    break
  fi
done
unset _a

# --- version + message-file validation ---

if [ -z "$VERSION" ]; then
  emit false "" "" false "missing version argument (usage: scripts/release-tag.sh <vX.Y.Z>)" null null
  exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  emit false "$VERSION" "" false "version must match vX.Y.Z (e.g. v0.2.0)" null null
  exit 1
fi

# MSG_FILE is required only for the create flow (it backs `git tag -a -F`).
# Push-only does not create a tag, so the message file is irrelevant.
if [ "$PUSH_ONLY" != "1" ]; then
  if [ -z "$MSG_FILE" ]; then
    emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE is not set" null null
    exit 1
  fi

  if [ ! -s "$MSG_FILE" ]; then
    emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE ('$MSG_FILE') is missing or empty" null null
    exit 1
  fi
fi

# --- tag existence ---
#
# Default (create) flow: refuse if the tag already exists (idempotent-safe
# re-runs). Push-only flow: refuse if the tag does NOT exist locally —
# push-only is not a creation path, and the tag must have been cut by a
# prior create-only invocation that already passed the DEFER gate. Capture
# the tag's target commit SHA here for the push-only JSON output (the
# create flow uses HEAD below).
#
# Push-only additionally refuses if the existing tag is NOT an annotated
# tag object: `git rev-parse --verify refs/tags/<v>^{commit}` resolves for
# BOTH annotated and lightweight tags, so the existence check alone would
# let a lightweight tag (`git tag <v>` with no `-a`) reach `git push` and
# defeat the annotated-tag invariant the wrapper exists to enforce. The
# `git cat-file -t` assertion below closes that gap.
TAG_TARGET_SHA=""
if [ "$PUSH_ONLY" = "1" ]; then
  if ! TAG_TARGET_SHA="$(git rev-parse -q --verify "refs/tags/${VERSION}^{commit}" 2>/dev/null)"; then
    emit false "$VERSION" "" false "tag ${VERSION} does not exist; cannot push-only (cut it first without --push-only)" null null
    exit 1
  fi
  # The tag exists, but push-only trusts the existing ANNOTATED tag object
  # (created by the full ceremony, which already passed the DEFER gate).
  # `git cat-file -t` distinguishes tag objects ("tag") from lightweight
  # refs that point straight at a commit ("commit"). `|| true` defends the
  # command substitution under `set -e` (the ref exists by the check above,
  # so this branch is unreachable in practice, but the guard is cheap).
  TAG_TYPE="$(git cat-file -t "refs/tags/${VERSION}" 2>/dev/null || true)"
  if [ "$TAG_TYPE" != "tag" ]; then
    emit false "$VERSION" "" false \
      "refs/tags/${VERSION} is not an annotated tag object; push-only requires an annotated tag created by the full ceremony (got lightweight tag)" \
      null null
    exit 1
  fi
else
  # refuse if the tag already exists (idempotent-safe re-runs)
  if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null 2>&1; then
    emit false "$VERSION" "" false "tag ${VERSION} already exists" null null
    exit 1
  fi
fi

# HEAD_SHA backs the create-flow JSON `commit` field. Push-only emits the
# tag's target commit (captured above as TAG_TARGET_SHA) instead.
HEAD_SHA=""
if [ "$PUSH_ONLY" != "1" ]; then
  HEAD_SHA="$(git rev-parse HEAD 2>/dev/null)" || {
    emit false "$VERSION" "" false "git rev-parse HEAD failed (not a git repository?)" null null
    exit 1
  }
fi

# --- optional flag parsing (--override-release-version / --override-manifest-sha) ---
#
# These flags are the OPERATOR-SIDE override ceremony — the only way an
# override_required record can be honored: the wrapper refuses to forward
# --override-confirmed-version to the evaluator unless BOTH flags agree with
# the requested version AND the actual committed manifest blob SHA. A
# model/reviewer surface cannot supply these — G7 is advisory and does not
# invoke the wrapper.

shift  # consume $1 (VERSION)
while [ $# -gt 0 ]; do
  case "$1" in
    --override-release-version)
      if [ $# -lt 2 ]; then
        emit false "$VERSION" "" false "--override-release-version requires a value" null null
        exit 1
      fi
      OVERRIDE_RELEASE_VERSION="$2"
      shift 2
      ;;
    --override-manifest-sha)
      if [ $# -lt 2 ]; then
        emit false "$VERSION" "" false "--override-manifest-sha requires a value" null null
        exit 1
      fi
      OVERRIDE_MANIFEST_SHA="$2"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    --push-only)
      # Pre-scanned above; consume and continue. The push-only short-circuit
      # after this loop takes the actual branch.
      shift
      ;;
    *)
      emit false "$VERSION" "" false "unknown argument: $1" null null
      exit 1
      ;;
  esac
done

# --- push-only short-circuit ---
#
# When --push-only is set, the tag already exists locally (verified above)
# and already passed the DEFER gate at creation time. Skip the override
# ceremony, DEFER gate, pre-tag disclosure print, and `git tag -a` mutation;
# go straight to `git push origin <version>`. The override ceremony is
# meaningless in push-only mode (the DEFER gate is skipped entirely), so
# combining --push-only with --override-* refuses.
if [ "$PUSH_ONLY" = "1" ]; then
  if [ -n "$OVERRIDE_RELEASE_VERSION" ] || [ -n "$OVERRIDE_MANIFEST_SHA" ]; then
    emit false "$VERSION" "" false \
      "--push-only cannot be combined with --override-release-version / --override-manifest-sha (the DEFER gate is skipped in push-only mode)" \
      null null
    exit 1
  fi
  PUSH_ERR=""
  if ! PUSH_ERR=$(git push origin "$VERSION" 2>&1 1>/dev/null); then
    emit false "$VERSION" "$TAG_TARGET_SHA" false "git push failed: ${PUSH_ERR}" null null
    exit 1
  fi
  emit true "$VERSION" "$TAG_TARGET_SHA" true "" null null
  exit 0
fi

# --- override ceremony ---
#
# The wrapper is the SOLE pre-tag transition authority for override intent:
# it refuses to forward --override-confirmed-version to the evaluator unless
# BOTH flags agree with the requested version AND the actual committed
# manifest blob SHA.

CONFIRMED_VERSION=""
if [ -n "$OVERRIDE_RELEASE_VERSION" ] || [ -n "$OVERRIDE_MANIFEST_SHA" ]; then
  # Both must be supplied together — partial confirmation is a refusal.
  if [ -z "$OVERRIDE_RELEASE_VERSION" ] || [ -z "$OVERRIDE_MANIFEST_SHA" ]; then
    emit false "$VERSION" "" false \
      "override ceremony requires BOTH --override-release-version and --override-manifest-sha together" \
      null null
    exit 1
  fi
  # 1. Requested version must match the override-release-version argument.
  if [ "$OVERRIDE_RELEASE_VERSION" != "$VERSION" ]; then
    emit false "$VERSION" "" false \
      "override ceremony: --override-release-version ($OVERRIDE_RELEASE_VERSION) != requested version ($VERSION)" \
      null null
    exit 1
  fi
  # 2. Manifest blob SHA must match the COMMITTED manifest at HEAD.
  #
  # The override ceremony binds to the committed blob SHA (what CI will
  # also see), NOT to a `git hash-object` of the worktree file. A dirty
  # worktree edit could otherwise swap the bytes the ceremony confirms vs.
  # the bytes CI verifies. `git rev-parse HEAD:<path>` resolves the blob
  # recorded in the HEAD tree and is invariant under worktree edits. If
  # the manifest is not committed at HEAD, the ceremony refuses: there is
  # no committed authority to bind to.
  ACTUAL_SHA="$(git rev-parse "HEAD:$MANIFEST_REL" 2>/dev/null)" || {
    emit false "$VERSION" "" false \
      "override ceremony: manifest not committed at HEAD:$MANIFEST_REL (cannot confirm SHA)" \
      null null
    exit 1
  }
  if [ "$OVERRIDE_MANIFEST_SHA" != "$ACTUAL_SHA" ]; then
    emit false "$VERSION" "" false \
      "override ceremony: --override-manifest-sha ($OVERRIDE_MANIFEST_SHA) != actual committed manifest blob ($ACTUAL_SHA)" \
      null null
    exit 1
  fi
  CONFIRMED_VERSION="$VERSION"
fi

# --- release DEFER gate (authoritative hard enforcement) ---
#
# The deterministic evaluator at .opencode/scripts/check-defer-triggers.js is
# the SINGLE source of DEFER classification truth. G7 in harness-release-
# readiness consumes the same evaluator ADVISORY-only; THIS gate is
# AUTHORITATIVE: a blocker or evaluator-error classification REFUSES the
# release before any `git tag` mutation. DEFERs stay non-blocking at COMMIT
# time (hard non-goal) — this gate fires only at release-tag time.
#
# Fail-closed policy (manifest-authority mode, the sole release mode):
#   committed manifest missing      → REFUSE (evaluator-error); override CANNOT cure
#   manifest malformed / handshake  → REFUSE (evaluator-error); override CANNOT cure
#                                    mismatch; repair the committed manifest
#   blocker (release-relevant       → REFUSE before mutation; remedy = resolve
#            finding requires         the finding OR supply the override ceremony
#            disposition)             (--override-release-version + --override-manifest-sha)
#   evaluator-error (other)         → REFUSE before mutation
#   disclose-only                   → pass (disclosures printed + embedded)
#
# See AGENTS.md "DEFER / follow-up curation" for the candidate contract and
# the v1 trigger grammar (path_touched(<exact-file>) and after_tag(<tag>) only).

PRIOR_TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"
DEFER_ARGS=(--mode=release)
if [ -n "$PRIOR_TAG" ]; then
  DEFER_ARGS+=(--since "$PRIOR_TAG")
fi
DEFER_ARGS+=(--release-version "$VERSION")
if [ -n "$CONFIRMED_VERSION" ]; then
  DEFER_ARGS+=(--override-confirmed-version "$CONFIRMED_VERSION")
fi

DEFER_OUTPUT=""
DEFER_EXIT=0
DEFER_OUTPUT=$(node .opencode/scripts/check-defer-triggers.js "${DEFER_ARGS[@]}" 2>/dev/null) || DEFER_EXIT=$?

# Extract disclosures + accepted_overrides + classification + sorted IDs from
# the evaluator JSON. Best-effort: if the output is unparseable, fall back to
# null arrays and a generic evaluator-error reason (still fail-closed).
DISCLOSURES_JSON="null"
ACCEPTED_OVERRIDES_JSON="null"
PARSED_OK=0
PARSED_CLASSIFICATION=""
PARSED_REASON="release-defer-gate: evaluator-error (exit=${DEFER_EXIT})"
if [ -n "$DEFER_OUTPUT" ]; then
  PARSED_OUTPUT=$(printf '%s' "$DEFER_OUTPUT" | node -e '
    let data = "";
    process.stdin.on("data", (c) => (data += c));
    process.stdin.on("end", () => {
      try {
        const o = JSON.parse(data);
        const cls = o.classification || "evaluator-error";
        const ids = [].concat(o.blocking_ids || [], o.evaluator_error_ids || []).sort();
        const idPart = ids.length ? (" ids=[" + ids.join(",") + "]") : "";
        process.stdout.write(JSON.stringify({
          ok: true,
          classification: cls,
          reason: "release-defer-gate: " + cls + idPart,
          disclosures: Array.isArray(o.disclosures) ? o.disclosures : [],
          accepted_overrides: Array.isArray(o.accepted_overrides) ? o.accepted_overrides : [],
        }));
      } catch (e) {
        process.stdout.write(JSON.stringify({
          ok: false,
          classification: "evaluator-error",
          reason: "release-defer-gate: evaluator-error (unparseable output)",
          disclosures: [],
          accepted_overrides: [],
        }));
      }
    });
  ' 2>/dev/null) || true
  if [ -n "$PARSED_OUTPUT" ]; then
    PARSED_OK=$(printf '%s' "$PARSED_OUTPUT" | node -e '
      let data = ""; process.stdin.on("data", (c) => (data += c));
      process.stdin.on("end", () => {
        const o = JSON.parse(data); process.stdout.write(o.ok ? "1" : "0");
      });')
    PARSED_CLASSIFICATION=$(printf '%s' "$PARSED_OUTPUT" | node -e '
      let data = ""; process.stdin.on("data", (c) => (data += c));
      process.stdin.on("end", () => {
        const o = JSON.parse(data); process.stdout.write(o.classification);
      });')
    PARSED_REASON=$(printf '%s' "$PARSED_OUTPUT" | node -e '
      let data = ""; process.stdin.on("data", (c) => (data += c));
      process.stdin.on("end", () => {
        const o = JSON.parse(data); process.stdout.write(o.reason);
      });')
    DISCLOSURES_JSON=$(printf '%s' "$PARSED_OUTPUT" | node -e '
      let data = ""; process.stdin.on("data", (c) => (data += c));
      process.stdin.on("end", () => {
        const o = JSON.parse(data); process.stdout.write(JSON.stringify(o.disclosures));
      });')
    ACCEPTED_OVERRIDES_JSON=$(printf '%s' "$PARSED_OUTPUT" | node -e '
      let data = ""; process.stdin.on("data", (c) => (data += c));
      process.stdin.on("end", () => {
        const o = JSON.parse(data); process.stdout.write(JSON.stringify(o.accepted_overrides));
      });')
  fi
fi

if [ "$DEFER_EXIT" -ne 0 ]; then
  emit false "$VERSION" "" false "$PARSED_REASON" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit "$DEFER_EXIT"
fi

# --- operator-ceremony enforcement ---
#
# The evaluator accepts a well-formed committed override object even when
# --override-confirmed-version is not supplied (CI defense-in-depth contract:
# CI verifies Layer A from the committed manifest but cannot verify Layer B
# operator live intent). The wrapper is the SOLE pre-tag transition authority
# for Layer B: if the evaluator accepted any override AND the operator did
# not supply the ceremony flags (--override-release-version +
# --override-manifest-sha, captured as CONFIRMED_VERSION), refuse BEFORE the
# tag mutation. This preserves wrapper enforcement whole without weakening
# CI's verification role. CI never invokes this wrapper, so CI is unaffected.
if [ "$PARSED_OK" = "1" ]; then
  if [ -n "$ACCEPTED_OVERRIDES_JSON" ] \
     && [ "$ACCEPTED_OVERRIDES_JSON" != "null" ] \
     && [ "$ACCEPTED_OVERRIDES_JSON" != "[]" ] \
     && [ -z "$CONFIRMED_VERSION" ]; then
    emit false "$VERSION" "" false \
      "override ceremony required: evaluator accepted override(s) but operator did not supply --override-release-version + --override-manifest-sha; accepted_overrides=$ACCEPTED_OVERRIDES_JSON" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
    exit 1
  fi
fi

# --- release deterministic-readiness gate (G0/G0b) ---
#
# Independent re-computation at tag time of the deterministic readiness
# gates G0 (green tree) and G0b (clean worktree). The wrapper is the SOLE
# pre-tag transition authority for these gates: any failure refuses BEFORE
# the `git tag -a` mutation. These are the SAME four Go commands the
# harness-release-readiness agent recommends as release prerequisites;
# the wrapper now independently verifies them at tag time so a model
# surface (the readiness report) cannot bypass them. Path 2 of the
# release-boundary enforcement design.
#
# Gates:
#   G0   go test ./..., go vet ./..., go build ./..., gofmt -l .
#        (any nonzero exit OR any gofmt -l output -> refuse)
#   G0b  git status --short (dirty worktree -> refuse; a dirty tree can
#        mask what CI's fresh checkout will see and breaks the
#        committed-blob authority contract the override ceremony binds to)
#
# G6-structure (S2 cross-check) is intentionally DEFERRED to a follow-up
# phase: the backlog prose parsing for `s2-hold:` tokens is heuristic and
# the readiness agent's G6 model-driven judgment already covers this gate
# at the model layer. A wrapper-side deterministic re-computation will
# land when the parsing contract is firmly established.
#
# Cost: G0 typically takes ~15s on this repo (dominated by `go test ./...`).
# This is the price of mechanical readiness enforcement at the tag
# boundary — the wrapper is the sole tag-cutting surface and verifying
# readiness at tag time is its job.

# G0 — green tree. Run the four Go commands in order; capture the first
# failing command's output for the refusal reason. `gofmt -l` is special:
# it exits 0 even when it lists files, so we treat ANY non-empty stdout
# as a formatting failure.
G0_REASON=""
G0_OUTPUT=""
if ! G0_OUTPUT=$(go test ./... 2>&1); then
  G0_REASON="release-readiness-gate: G0 go test ./... failed"
elif ! G0_OUTPUT=$(go vet ./... 2>&1); then
  G0_REASON="release-readiness-gate: G0 go vet ./... failed"
elif ! G0_OUTPUT=$(go build ./... 2>&1); then
  G0_REASON="release-readiness-gate: G0 go build ./... failed"
else
  # gofmt -l exits 0 even when listing files; any non-empty stdout is a
  # formatting failure. (Parse errors exit nonzero and would have been
  # caught by `go build` above, but `|| true` defends the command
  # substitution under `set -e` regardless.)
  G0_OUTPUT=$(gofmt -l . 2>&1) || true
  if [ -n "$G0_OUTPUT" ]; then
    G0_REASON="release-readiness-gate: G0 gofmt -l . reported unformatted files"
  fi
fi
if [ -n "$G0_REASON" ]; then
  emit false "$VERSION" "" false \
    "$G0_REASON (output: $G0_OUTPUT)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# G0b — clean worktree. A dirty worktree at tag time means the tag would
# not capture uncommitted edits, breaking the contract that the tagged
# commit is exactly what was reviewed. This also closes the worktree-vs-
# HEAD bypass class at the wrapper level: even if a future gate read the
# worktree file, G0b prevents a dirty worktree from reaching the tag
# mutation.
G0B_OUTPUT=$(git status --short 2>&1) || true
if [ -n "$G0B_OUTPUT" ]; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G0b dirty worktree (commit or stash before tagging); git status --short: $G0B_OUTPUT" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# G0c — vh-agent-harness doctor HEALTHY (all 14 checks). This is the mandatory
# machine gate that makes `vh-agent-harness doctor` a HARD ceremony stop: any
# problem-tier or fail-tier check (including #12 defer-liveness, #13
# staged-errata-content, and #14 behavioral-closure) refuses the tag BEFORE the
# readiness-pass artifact gate or any tag mutation.
#
# This kills the "human-remembered pre-flight" anti-pattern: doctor is no longer
# a step the releaser agent might forget — it is a non-zero-exit gate the wrapper
# enforces at the tag boundary, the same boundary that already enforces G0/G0b.
#
# Why this lives in the WRAPPER (not the releaser prompt): opencode caches the
# releaser subagent prompt per-process, so a prompt-only ceremony step would be
# stale-cached and NOT active for the ceremony run in the current session. The
# wrapper reads from disk and is effective immediately. The binary is the
# authority, not the prose.
#
# SEAM-INSTALLATION GATE: doctor is a seam-health probe; it is meaningless on a
# tree that was never seam-installed (no .vh-agent-harness/lineage.yml means no
# managed .opencode/ tree to drift against — doctor's managed-drift check would
# false-positive). The lineage record is the authoritative "this is a
# seam-managed tree" marker, so G0c only runs doctor when it is present. This is
# a STRUCTURAL gate (file presence), not a runtime env-var bypass — a production
# release cannot select it, and deleting a committed managed file is itself a
# doctor problem. Push-only mode exits before reaching this gate (the tag
# already passed it at creation).
#
# STALENESS GUARD: the binary on PATH could predate check #13
# (staged-errata-content) and report HEALTHY while lacking the enforcement this
# gate exists to provide. The canary `vh-agent-harness release inject-errata
# --help` exits 0 only on a binary that has the new subcommand; a stale binary
# fails it and G0c refuses with a clear "rebuild" message.
if [ -f ".vh-agent-harness/lineage.yml" ]; then
  if ! vh-agent-harness release inject-errata --help >/dev/null 2>&1; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c staleness guard — the vh-agent-harness binary on PATH predates the staged-errata-content enforcement (no \`release inject-errata\` subcommand); run \`make build\` so doctor reflects the current source, then retry" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
    exit 1
  fi
  if ! G0C_OUTPUT=$(vh-agent-harness doctor 2>&1); then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c doctor not HEALTHY (a machine check FAILED — run \`vh-agent-harness doctor\` for the full report). Doctor output: $G0C_OUTPUT" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
    exit 1
  fi
fi

# --- release readiness-pass artifact gate (G1-G5, model-driven) ---
#
# Wrapper-side enforcement of the model-driven readiness gates G1-G5.
# The harness-release-readiness agent writes an exclusive-signing artifact
# at .vh-agent-harness/release-readiness-pass.json after evaluating the
# model-driven gates (G1-coverage, G2-significance, G3-docs, G4-visibility,
# G5-curated-note). The wrapper reads it from HEAD (committed) and refuses
# the tag unless all five model-driven gates report `ready`.
#
# Why this is here: deterministic gates (G0/G0b/G7-DEFER) are re-computed
# by the wrapper from primary state, so they cannot be bypassed by model
# output. Model-driven gates (G1-G5) require judgment and cannot be
# re-computed from primary state. Path 1 of the release-boundary
# enforcement design: the readiness agent (which has NO tag authority)
# authors the verdict, and the wrapper (which has tag authority but NO
# model judgment) checks the artifact before cutting the tag. Neither
# surface can both author AND authorize — that is the safety split.
#
# Commit-binding handshake (mirrors the DEFER manifest pattern, but one
# level deeper because the manifest is always HEAD):
#   - The release ceremony sequences: note → artifact → manifest. At tag
#     time HEAD is the manifest commit (DEFER handshake: HEAD^..HEAD diff
#     is exactly the manifest path). The artifact commit is at HEAD^,
#     authored on top of the release-prep commit at HEAD^^.
#   - The artifact must exist at `HEAD:.vh-agent-harness/release-readiness-pass.json`
#     (read via `git show HEAD:`, NOT the worktree — G0b already refused a
#     dirty tree).
#   - `commit_sha` in the artifact must equal HEAD^^ (the release-prep
#     commit the readiness agent evaluated). NOT HEAD, because HEAD is the
#     manifest commit (DEFER constraint — the manifest is always HEAD).
#   - `HEAD^^..HEAD^ diff` must be exactly the artifact path (the artifact
#     commit is a single-path child of the release-prep commit, mirroring
#     the manifest ceremony).
#
# Fail-closed (9 cases, all refuse BEFORE `git tag -a`):
#   1. Deterministic gate failure (G0/G0b) — already refused above
#   2. Dirty tree (G0b) — already refused above
#   3. DEFER/G6 failure — already refused by the DEFER gate
#   4. Missing artifact at HEAD:<path>
#   5. Invalid schema (malformed JSON, missing fields, unknown verdict)
#   6. commit_sha ≠ HEAD^^
#   7. Any gate = blocked
#   8. Any gate = skipped
#   9. Any unknown verdict value (case 5 covers schema; this is the enum)
#
# Closed verdict enum: `ready` | `blocked` | `skipped`. Any other value
# (including the empty string) → refuse. The wrapper NEVER defaults to
# `ready`.

READINESS_REL=".vh-agent-harness/release-readiness-pass.json"

# Resolve HEAD^ (expected: artifact commit) and HEAD^^ (expected:
# release-prep commit). If the history is too shallow, the readiness
# ceremony has not happened — refuse fail-closed.
HEAD_PARENT_SHA=""
if ! HEAD_PARENT_SHA=$(git rev-parse --verify "HEAD^" 2>/dev/null); then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness ceremony (HEAD^ does not exist; expected note -> artifact -> manifest sequencing with manifest at HEAD)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi
HEAD_GP_SHA=""
if ! HEAD_GP_SHA=$(git rev-parse --verify "HEAD^^" 2>/dev/null); then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness ceremony (HEAD^^ does not exist; expected note -> artifact -> manifest sequencing with manifest at HEAD)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# Artifact must exist at HEAD (committed). Use `git cat-file -e` to check
# existence without loading bytes.
if ! git cat-file -e "HEAD:${READINESS_REL}" 2>/dev/null; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness artifact at HEAD:${READINESS_REL} (readiness agent must evaluate and the releaser must commit the artifact as HEAD^ before the manifest commit)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# Artifact commit (HEAD^) must be a single-path child of HEAD^^, and the
# single changed path must be the readiness artifact. Mirrors the DEFER
# manifest's single-path child check one level deeper.
ARTIFACT_DIFF=$(git diff --name-only "HEAD^^" "HEAD^" 2>/dev/null) || true
if [ "$ARTIFACT_DIFF" != "$READINESS_REL" ]; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 artifact commit (HEAD^) must change only ${READINESS_REL}; got: ${ARTIFACT_DIFF}" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# Validate the artifact content (schema + extract verdict). Parse via
# node (mirrors the DEFER gate's JSON parsing pattern). The validator
# returns a JSON object: {ok, reason, commit_sha, gates}. `ok=false`
# means schema/enum validation failed and `reason` carries the refusal
# text. `ok=true` means the artifact is well-formed; the wrapper then
# checks commit_sha binding and all-ready below.
READINESS_BYTES=$(git show "HEAD:${READINESS_REL}" 2>/dev/null) || true
READINESS_VERDICT=$(printf '%s' "$READINESS_BYTES" | node -e '
  let data = ""; process.stdin.on("data", (c) => (data += c));
  process.stdin.on("end", () => {
    const ALLOWED = new Set(["ready", "blocked", "skipped"]);
    const GATES = ["G1_coverage", "G2_significance", "G3_docs", "G4_visibility", "G5_curated_note"];
    function refuse(reason) {
      process.stdout.write(JSON.stringify({ok: false, reason: reason, commit_sha: "", gates: ""}));
      process.exit(0);
    }
    let o;
    try { o = JSON.parse(data); } catch (e) {
      refuse("release-readiness-gate: G1-G5 artifact is not valid JSON (" + e.message + ")");
      return;
    }
    if (typeof o !== "object" || o === null) {
      refuse("release-readiness-gate: G1-G5 artifact is not a JSON object");
      return;
    }
    if (o.schema_version !== 1) {
      refuse("release-readiness-gate: G1-G5 artifact schema_version must be 1 (got " + JSON.stringify(o.schema_version) + ")");
      return;
    }
    if (typeof o.commit_sha !== "string" || !/^[0-9a-f]{40}$/.test(o.commit_sha)) {
      refuse("release-readiness-gate: G1-G5 artifact commit_sha must be a 40-char hex SHA (got " + JSON.stringify(o.commit_sha) + ")");
      return;
    }
    if (typeof o.model_gates !== "object" || o.model_gates === null) {
      refuse("release-readiness-gate: G1-G5 artifact missing model_gates object");
      return;
    }
    for (const g of GATES) {
      if (!(g in o.model_gates)) {
        refuse("release-readiness-gate: G1-G5 artifact missing gate " + g);
        return;
      }
      const v = o.model_gates[g];
      if (!ALLOWED.has(v)) {
        refuse("release-readiness-gate: G1-G5 gate " + g + " has unknown verdict " + JSON.stringify(v) + " (allowed: ready|blocked|skipped)");
        return;
      }
    }
    // Reject unknown gate keys — prevents a new model-driven gate from being
    // silently dropped if the GATES array has not been updated yet (fail-open
    // closure). A new gate emitting "blocked" must surface here, not pass.
    for (const k of Object.keys(o.model_gates)) {
      if (!GATES.includes(k)) {
        refuse("release-readiness-gate: G1-G5 artifact has unknown gate key " + JSON.stringify(k) + " (expected only: " + GATES.join(", ") + ")");
        return;
      }
    }
    const gates = GATES.map((g) => g + "=" + o.model_gates[g]).join(",");
    process.stdout.write(JSON.stringify({ok: true, reason: "", commit_sha: o.commit_sha, gates: gates}));
  });
' 2>/dev/null) || true

if [ -z "$READINESS_VERDICT" ]; then
  READINESS_VERDICT='{"ok":false,"reason":"release-readiness-gate: G1-G5 artifact validator crashed (no output)","commit_sha":"","gates":""}'
fi

# Extract fields from the verdict (mirrors the DEFER gate extraction).
RV_OK=$(printf '%s' "$READINESS_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).ok?"1":"0");})') || RV_OK="0"
RV_REASON=$(printf '%s' "$READINESS_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).reason||"");})') || RV_REASON=""
RV_COMMIT=$(printf '%s' "$READINESS_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).commit_sha||"");})') || RV_COMMIT=""
RV_GATES=$(printf '%s' "$READINESS_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).gates||"");})') || RV_GATES=""

if [ "$RV_OK" != "1" ]; then
  emit false "$VERSION" "" false \
    "$RV_REASON" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# commit_sha binding: the artifact must be pinned to the release-prep
# commit (HEAD^^), which is what the readiness agent evaluated. This
# prevents stale artifacts (evaluated against an older commit) from
# authorizing a release built on a different commit.
if [ "$RV_COMMIT" != "$HEAD_GP_SHA" ]; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 artifact commit_sha (${RV_COMMIT}) does not match release-prep commit HEAD^^ (${HEAD_GP_SHA}); the readiness artifact must be re-evaluated and re-committed against the current release-prep commit" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# All five model-driven gates must be `ready`. Any blocked/skipped
# (unknown was already refused at schema validation) → refuse.
if printf '%s' "$RV_GATES" | grep -Eq '=blocked|=skipped'; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 not all ready (${RV_GATES}); every model-driven gate must report ready before tagging" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# --- pre-tag disclosure print ---
#
# The operator sees disclosures + accepted overrides on stderr BEFORE the
# `git tag -a` mutation. The same data is embedded in the final JSON.
if [ "$PARSED_OK" = "1" ]; then
  if [ -n "$DISCLOSURES_JSON" ] && [ "$DISCLOSURES_JSON" != "null" ] && [ "$DISCLOSURES_JSON" != "[]" ]; then
    printf '[release-tag] disclosures from committed manifest:\n' >&2
    printf '%s\n' "$DISCLOSURES_JSON" >&2
  fi
  if [ -n "$ACCEPTED_OVERRIDES_JSON" ] && [ "$ACCEPTED_OVERRIDES_JSON" != "null" ] && [ "$ACCEPTED_OVERRIDES_JSON" != "[]" ]; then
    printf '[release-tag] accepted overrides (operator-approved):\n' >&2
    printf '%s\n' "$ACCEPTED_OVERRIDES_JSON" >&2
  fi
fi

# --- mutation: create the annotated tag from the message file ---

TAG_ERR=""
if ! TAG_ERR=$(git tag -a -F "$MSG_FILE" "$VERSION" 2>&1 1>/dev/null); then
  emit false "$VERSION" "" false "git tag -a failed: ${TAG_ERR}" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
  exit 1
fi

# --- optional opt-in push (default: local-only) ---

PUSHED=false
if [ "${RELEASE_TAG_PUSH-0}" = "1" ]; then
  PUSH_ERR=""
  if ! PUSH_ERR=$(git push origin "$VERSION" 2>&1 1>/dev/null); then
    # tag was created locally but the requested push failed
    emit false "$VERSION" "$HEAD_SHA" false "tag ${VERSION} created locally but git push failed: ${PUSH_ERR}" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
    exit 1
  fi
  PUSHED=true
fi

emit true "$VERSION" "$HEAD_SHA" "$PUSHED" "" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON"
