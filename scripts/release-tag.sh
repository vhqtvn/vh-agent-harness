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
#
# Output: exactly ONE valid JSON object on stdout, nothing else.
#   success: {"ok":true,"tag":"vX.Y.Z","commit":"<HEAD-sha>","pushed":false,
#             "error":null,"disclosures":[...],"accepted_overrides":[...]}
#   refusal: {"ok":false,"tag":"vX.Y.Z|null","commit":"<sha>|null","pushed":false,
#             "error":"<reason>","disclosures":[...],"accepted_overrides":[...]}
# `disclosures` and `accepted_overrides` are ALWAYS present, carrying the
# evaluator's disclosures and any operator-approved overrides.
# On any validation failure the script prints the refusal JSON and exits non-zero.
# Refuses (non-zero) if the tag already exists, so re-running after a failure is safe.
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

# --- version + message-file validation (unchanged) ---

if [ -z "$VERSION" ]; then
  emit false "" "" false "missing version argument (usage: scripts/release-tag.sh <vX.Y.Z>)" null null
  exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  emit false "$VERSION" "" false "version must match vX.Y.Z (e.g. v0.2.0)" null null
  exit 1
fi

if [ -z "$MSG_FILE" ]; then
  emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE is not set" null null
  exit 1
fi

if [ ! -s "$MSG_FILE" ]; then
  emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE ('$MSG_FILE') is missing or empty" null null
  exit 1
fi

# refuse if the tag already exists (idempotent-safe re-runs)
if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null 2>&1; then
  emit false "$VERSION" "" false "tag ${VERSION} already exists" null null
  exit 1
fi

HEAD_SHA="$(git rev-parse HEAD 2>/dev/null)" || {
  emit false "$VERSION" "" false "git rev-parse HEAD failed (not a git repository?)" null null
  exit 1
}

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
    *)
      emit false "$VERSION" "" false "unknown argument: $1" null null
      exit 1
      ;;
  esac
done

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
