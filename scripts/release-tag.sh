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
# Arguments / environment:
#   $1                          -> version, must match ^v[0-9]+\.[0-9]+\.[0-9]+$
#   $RELEASE_TAG_MESSAGE_FILE   -> path to the annotated tag message file (must
#                                  exist and be non-empty); passed to `git tag -a -F`
#   $RELEASE_TAG_PUSH           -> optional; set to "1" to also `git push origin <version>`
#
# Output: exactly ONE valid JSON object on stdout, nothing else.
#   success: {"ok":true,"tag":"vX.Y.Z","commit":"<HEAD-sha>","pushed":false,"error":null}
#   refusal: {"ok":false,"tag":"vX.Y.Z|null","commit":"<sha>|null","pushed":false,"error":"<reason>"}
# On any validation failure the script prints the refusal JSON and exits non-zero.
# Refuses (non-zero) if the tag already exists, so re-running after a failure is safe.
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
  # ok tag commit pushed error  (booleans as bare true/false; nulls via json_str)
  printf '{"ok":%s,"tag":%s,"commit":%s,"pushed":%s,"error":%s}\n' \
    "$1" "$(json_str "$2")" "$(json_str "$3")" "$4" "$(json_str "$5")"
}

VERSION="${1-}"
MSG_FILE="${RELEASE_TAG_MESSAGE_FILE-}"

# --- validation ---

if [ -z "$VERSION" ]; then
  emit false "" "" false "missing version argument (usage: scripts/release-tag.sh <vX.Y.Z>)"
  exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  emit false "$VERSION" "" false "version must match vX.Y.Z (e.g. v0.2.0)"
  exit 1
fi

if [ -z "$MSG_FILE" ]; then
  emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE is not set"
  exit 1
fi

if [ ! -s "$MSG_FILE" ]; then
  emit false "$VERSION" "" false "RELEASE_TAG_MESSAGE_FILE ('$MSG_FILE') is missing or empty"
  exit 1
fi

# refuse if the tag already exists (idempotent-safe re-runs)
if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null 2>&1; then
  emit false "$VERSION" "" false "tag ${VERSION} already exists"
  exit 1
fi

HEAD_SHA="$(git rev-parse HEAD 2>/dev/null)" || {
  emit false "$VERSION" "" false "git rev-parse HEAD failed (not a git repository?)"
  exit 1
}

# --- mutation: create the annotated tag from the message file ---

TAG_ERR=""
if ! TAG_ERR=$(git tag -a -F "$MSG_FILE" "$VERSION" 2>&1 1>/dev/null); then
  emit false "$VERSION" "" false "git tag -a failed: ${TAG_ERR}"
  exit 1
fi

# --- optional opt-in push (default: local-only) ---

PUSHED=false
if [ "${RELEASE_TAG_PUSH-0}" = "1" ]; then
  PUSH_ERR=""
  if ! PUSH_ERR=$(git push origin "$VERSION" 2>&1 1>/dev/null); then
    # tag was created locally but the requested push failed
    emit false "$VERSION" "$HEAD_SHA" false "tag ${VERSION} created locally but git push failed: ${PUSH_ERR}"
    exit 1
  fi
  PUSHED=true
fi

emit true "$VERSION" "$HEAD_SHA" "$PUSHED" ""
