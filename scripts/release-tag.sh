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
# Usage (historical-M tagging — HEAD has advanced past the manifest commit M,
#        e.g. an operator-directed chore landed after the ceremony completed):
#   RELEASE_TAG_MESSAGE_FILE=tmp/release-tag-msg-v0.29.0.txt scripts/release-tag.sh v0.29.0
#   The wrapper DERIVES the eligible manifest commit M from current committed
#   ceremony anchors (see the "historical-M derivation" block): H's committed
#   manifest names R=evaluated_commit; M is R's direct single-parent child on
#   H's first-parent chain; H and M must carry identical manifest /
#   readiness-artifact / migration-note content; every intervening commit
#   M..H is inspected for protected-path changes (edit, deletion, or revert
#   all count — the ceremony surface plus the lineage record); suffix merges
#   refuse (v1). When all eligibility holds, the wrapper runs G0 in a
#   DETACHED temporary verification worktree at M (wrapper-owned isolation
#   — created under <repo>/tmp/, cleaned up BEFORE tagging, cleanup failure
#   refuses), routes doctor checks by release-context class (target/both
#   classes are evaluated against M's tree; control class stays on the
#   original checkout / real environment; G0c applicability is PER-SIDE —
#   lineage.yml at H gates the control run, the lineage.yml BLOB at M gates
#   the target run), evaluates G7/G6 with --commit M, and cuts the tag
#   PINNED to M from the original repo.
#
#   --at <full-SHA> is ONLY an assertion: it must equal the DERIVED eligible M
#   or the wrapper refuses. It is NEVER a free target selector — there is no
#   way to ask this wrapper to tag an arbitrary commit.
#
# Arguments / environment:
#   $1                          -> version, must match ^v[0-9]+\.[0-9]+\.[0-9]+$
#   --at                        -> optional; ASSERTION ONLY. Full 40-hex SHA
#                                  that must equal the derived eligible M.
#                                  Refuses on mismatch (guards against pinning
#                                  a superseded ceremony's manifest commit).
#   --override-release-version  -> optional; explicit operator confirmation of
#                                  the release version bound to an override.
#                                  Must equal $1. Requires --override-manifest-sha.
#   --override-manifest-sha     -> optional; explicit operator confirmation of
#                                  the manifest blob SHA being overridden.
#                                  Must equal `git rev-parse <M>:<path>` of
#                                  the committed manifest at the derived
#                                  eligible M (NOT the worktree
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
#   success: {"ok":true,"tag":"vX.Y.Z","commit":"<validated-tag-target-sha>",
#             "pushed":false,"error":null,"disclosures":[...],
#             "accepted_overrides":[...],"head_drift":false,"historical":<bool>}
#   refusal: {"ok":false,"tag":"vX.Y.Z|null","commit":"<sha>|null","pushed":false,
#             "error":"<reason>","disclosures":[...],"accepted_overrides":[...],
#             "head_drift":false,"historical":<bool>}
# `historical` is true iff the tag was cut at a DERIVED historical M (HEAD had
# advanced past the manifest commit M at wrapper start); `commit` is then M.
# It is false on every classic path (M == HEAD at start) and on refusals that
# happen before the derivation completes.
# `disclosures` and `accepted_overrides` are ALWAYS present (both `null` in
# --push-only mode, which skips the DEFER gate; otherwise they carry the
# evaluator's disclosures and any operator-approved overrides).
# `head_drift` is ALWAYS present (boolean). It is `true` only on a tag cut AFTER
# the tag-ready->tag drift check detected HEAD had advanced past the validated
# commit during the gate window — the tag is still PINNED to the validated
# commit (see the drift-detection block near the mutation), but the operator is
# informed a concurrent commit landed and is NOT part of this release. `false`
# otherwise (including every refusal path before the mutation, which did not pin).
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
  #
  # Escaping routes through ONE node invocation — JSON.stringify emits the
  # identical contract (\" and \\ escapes, \b \t \n \f \r named-control
  # shorthand, \u00XX for every other C0 byte, multi-byte UTF-8 passed through
  # as-is). The former implementation ran one subshell per character for the
  # C0 catch-all loop (~1M forks for a ~1MB error payload — minutes of wall
  # time). The historical parameter-expansion pre-pass for named controls is
  # deliberately GONE: under JSON.stringify it would double-escape (its own
  # emitted backslash would be escaped again), so the contract is preserved
  # by node alone.
  #
  # The payload crosses via STDIN (the file's established node idiom — the
  # data/end accumulate pattern used by the readiness-verdict and gate
  # parsers), never argv: a payload beginning with a dash — e.g. an error
  # message naming the flag it refuses, "--push-only cannot be combined
  # with ..." — would otherwise be parsed by node as a bad option and abort
  # the emit mid-line. (A synchronous process.stdin.read() is NOT usable
  # here: stdin starts paused on a pipe and read() returns null.)
  #
  # Chunks accumulate as BUFFERS and decode exactly ONCE, in the end
  # handler: string accumulation (d+=c) decodes each Buffer chunk
  # independently, so a multi-byte UTF-8 sequence split across a pipe
  # chunk boundary would corrupt to U+FFFD on both sides of the split.
  # Buffer.concat + a single toString("utf8") reassembles the split
  # sequence — one decode point, byte-faithful at any chunking.
  local v="${1-}"
  if [ -z "$v" ]; then
    printf 'null'
    return
  fi
  printf '%s' "$v" | node -e 'let cs=[];process.stdin.on("data",c=>cs.push(c));process.stdin.on("end",()=>{process.stdout.write(JSON.stringify(Buffer.concat(cs).toString("utf8")));})'
}

emit() {
  # ok tag commit pushed error disclosures accepted_overrides head_drift [historical]
  # (booleans as bare true/false; nulls via json_str; disclosures/overrides are
  # pre-rendered JSON array literals, or the literal "null"). head_drift defaults
  # to false: only the post-drift-check emits (mutation success/failure and the
  # pin-verification failure) pass a real $HEAD_DRIFTED value; every earlier
  # refusal path leaves it false (no pin attempted). historical defaults to
  # false: only historical-M paths (and their refusals after derivation) pass
  # true.
  printf '{"ok":%s,"tag":%s,"commit":%s,"pushed":%s,"error":%s,"disclosures":%s,"accepted_overrides":%s,"head_drift":%s,"historical":%s}\n' \
    "$1" "$(json_str "$2")" "$(json_str "$3")" "$4" "$(json_str "$5")" "${6:-null}" "${7:-null}" "${8:-false}" "${9:-false}"
}

VERSION="${1-}"
MSG_FILE="${RELEASE_TAG_MESSAGE_FILE-}"
AT_FLAG=""
OVERRIDE_RELEASE_VERSION=""
OVERRIDE_MANIFEST_SHA=""
MANIFEST_REL=".vh-agent-harness/release-defer-dispositions.json"
READINESS_REL=".vh-agent-harness/release-readiness-pass.json"
LINEAGE_REL=".vh-agent-harness/lineage.yml"

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
    --at)
      if [ $# -lt 2 ]; then
        emit false "$VERSION" "" false "--at requires a value (full 40-hex commit SHA)" null null
        exit 1
      fi
      AT_FLAG="$2"
      shift 2
      ;;
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
  if [ -n "$AT_FLAG" ]; then
    emit false "$VERSION" "" false \
      "--push-only cannot be combined with --at (push-only pushes an already-cut tag and performs no M derivation)" \
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

# --- historical-M derivation (eligible-M policy: uninterrupted current ceremony) ---
#
# The release ceremony completes at the manifest commit M (N -> R -> M; at tag
# time the classic flow expects HEAD == M). When HEAD has ADVANCED past M
# (classically: an operator-directed commit lands after M), the old wrapper had
# no path forward — G0b/G7/G1-G5 all bind to HEAD, so the ceremony had to be
# re-run (README "Failure mode B" operator history surgery). This block
# DERIVES the eligible M from current committed ceremony anchors so the
# eligible case is agent-only: no detached-HEAD steps, no re-ceremony.
#
# Derivation (deterministic, from committed state only):
#   1. Read H's committed manifest (HEAD:<MANIFEST_REL>). Missing/unparseable
#      -> M_SHA stays empty and the DEFER gate below produces the canonical
#      refusal (the evaluator's own "manifest missing/malformed" error).
#   2. R := manifest.evaluated_commit (the release-prep state the manifest
#      evaluated and handshake-binds to).
#   3. M := the commit on H's FIRST-PARENT chain whose ONLY parent is exactly
#      R (R's direct single-parent child). A merge whose first parent is R is
#      NOT selected (multi-parent line never matches) and is separately
#      refused by the suffix-merge check.
#   4. If M == H  -> CLASSIC flow (byte-identical to the pre-historical
#      wrapper: every gate anchors at HEAD, no --commit, no worktree).
#      If M is derivable and != H -> HISTORICAL flow, subject to eligibility:
#        (a) no suffix merges in M..H on H's first-parent chain (v1 refusal);
  #        (b) EVERY intervening commit M..H inspected for protected-path
  #            changes — edit, deletion, rename, or revert all count (a
  #            revert, deletion, or rename-away produces a diff against the
  #            parent like any edit; the scan disables rename detection so a
  #            rename cannot hide the source).
#            Protected = the ceremony surface: the manifest, the readiness
#            artifact, the about-to-release migration note's directory
#            (templates/migrations/), this wrapper, the two gate evaluators,
#            and the lineage record (.vh-agent-harness/lineage.yml — the G0c
#            applicability marker: an M..H edit or deletion changes whether
#            and where doctor runs at all). Anything that changes what
#            M's gates MEAN breaks the "uninterrupted ceremony" premise.
#        (c) H and M carry IDENTICAL manifest / readiness-artifact /
#            migration-note content (blob-SHA equality; all three must EXIST
#            at M — the note and artifact are ceremony outputs).
#        (d) --at, when supplied, must equal the derived M (assertion only).
#
# The derived M is then verified AUTHORITATIVELY by the gates below with the
# current trusted evaluators anchored at --commit M (G7's freshness handshake
# at M, G1-G5's artifact handshake at M^/M^^, G6 at M) — the derivation only
# SELECTS the anchor; the evaluators verify it.
#
# Authority note: this wrapper is the sole sanctioned surface for the
# temporary verification worktree it creates in the HISTORICAL flow (see the
# isolation block below). Agents retain NO raw checkout/reset/tag/push/
# worktree authority — this block changes nothing about that split.
M_SHA=""
HISTORICAL=0
ANCHOR_REF="HEAD"
H_SHA="$HEAD_SHA"
DERIV_NOTE_REL="templates/migrations/${VERSION}.md"
H_MANIFEST_BYTES="$(git show "HEAD:$MANIFEST_REL" 2>/dev/null)" || H_MANIFEST_BYTES=""
if [ -n "$H_MANIFEST_BYTES" ]; then
  DERIVED_R="$(printf '%s' "$H_MANIFEST_BYTES" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{try{const o=JSON.parse(d);process.stdout.write(typeof o.evaluated_commit==="string"?o.evaluated_commit:"");}catch(e){process.stdout.write("");}})' 2>/dev/null)" || DERIVED_R=""
  if [ -n "$DERIVED_R" ] && printf '%s' "$DERIVED_R" | grep -Eq '^[0-9a-f]{40}$'; then
    # Find R's direct single-parent child on H's first-parent chain. The %P
    # field lists ALL parents; requiring it to EQUAL R exactly excludes
    # merges (multi-parent) and non-children. Newest-first walk stops at the
    # first match (on a linear chain there is at most one).
    M_SHA="$(git log --first-parent --format='%H %P' "${DERIVED_R}..${H_SHA}" 2>/dev/null | awk -v r="$DERIVED_R" '$2==r && NF==2 {print $1; exit}')" || M_SHA=""
    if [ -n "$M_SHA" ] && [ "$M_SHA" != "$H_SHA" ]; then
      HISTORICAL=1
      ANCHOR_REF="$M_SHA"
    fi
  fi
fi

# --at is an ASSERTION, never a selector: it must match the derived eligible M
# (or, in the classic flow, HEAD). A mismatch is the superseded-ceremony
# refusal: e.g. after a re-ceremony (M2 superseding M1), --at <M1> must refuse
# because the CURRENT committed anchors select M2.
if [ -n "$AT_FLAG" ]; then
  EXPECTED_AT="$H_SHA"
  if [ "$HISTORICAL" = "1" ]; then
    EXPECTED_AT="$M_SHA"
  fi
  if ! printf '%s' "$AT_FLAG" | grep -Eq '^[0-9a-f]{40}$'; then
    emit false "$VERSION" "" false "--at must be a full 40-char lowercase hex commit SHA (got ${AT_FLAG})" null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  if [ "$AT_FLAG" != "$EXPECTED_AT" ]; then
    emit false "$VERSION" "" false \
      "--at assertion failed: requested ${AT_FLAG} but the eligible ceremony commit is ${EXPECTED_AT} (derived from current committed anchors; --at is an assertion, not a target selector — re-ceremonied manifests are superseded)" \
      null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
fi

# Eligibility checks (historical flow only).
if [ "$HISTORICAL" = "1" ]; then
  # (a) suffix merges refuse (v1). A merge on H's first-parent chain above M
  # means the branch history is not the uninterrupted linear ceremony
  # continuation this flow is certified for.
  MERGE_LIST="$(git rev-list --first-parent --merges "${M_SHA}..${H_SHA}" 2>/dev/null)" || MERGE_LIST=""
  if [ -n "$MERGE_LIST" ]; then
    emit false "$VERSION" "" false \
      "historical-M eligibility: suffix merge(s) between M (${M_SHA}) and HEAD (${H_SHA}) are refused in v1 (merge commits: $(printf '%s' "$MERGE_LIST" | tr '\n' ' '))" \
      null null false true
    exit 1
  fi

  # (b) protected-path scan of EVERY intervening commit (edit, deletion,
  # rename, or revert — all produce a diff against the parent and all count;
  # deleted paths appear in `git diff --name-only` like any edit, and rename
  # detection is DISABLED (--no-renames) so a rename OUT of a protected path
  # cannot hide its source: with git's default rename detection a pure
  # rename lists ONLY the — typically unprotected — destination, which is
  # exactly the round-6 F7 eligibility bypass. --no-renames also makes the
  # scan independent of the operator's diff.renames config).
  PROTECT_HIT=""
  while IFS= read -r IC; do
    [ -z "$IC" ] && continue
    while IFS= read -r P; do
      [ -z "$P" ] && continue
      case "$P" in
        "$MANIFEST_REL"|"$READINESS_REL"|"$LINEAGE_REL"|scripts/release-tag.sh|.opencode/scripts/check-defer-triggers.mjs|.opencode/scripts/check-s2-holds.mjs|templates/migrations/*)
          PROTECT_HIT="${IC}:${P}"
          break
          ;;
      esac
    done <<< "$(git diff --name-only --no-renames "${IC}^" "${IC}" 2>/dev/null)"
    [ -n "$PROTECT_HIT" ] && break
  done <<< "$(git rev-list --first-parent --reverse "${M_SHA}..${H_SHA}" 2>/dev/null)"
  if [ -n "$PROTECT_HIT" ]; then
    emit false "$VERSION" "" false \
      "historical-M eligibility: protected-path change in intervening commit ${PROTECT_HIT} (ceremony surface — manifest, readiness artifact, migration notes, wrapper, gate evaluators, or the lineage record — must be unchanged between M and HEAD; edits, deletions, renames, AND reverts all refuse)" \
      null null false true
    exit 1
  fi

  # (c) identical manifest / readiness-artifact / migration-note content at
  # H and M (blob-SHA equality; all three must exist at M).
  for RELP in "$MANIFEST_REL" "$READINESS_REL" "$DERIV_NOTE_REL"; do
    BLOB_AT_H="$(git rev-parse "HEAD:${RELP}" 2>/dev/null)" || BLOB_AT_H=""
    BLOB_AT_M="$(git rev-parse "${M_SHA}:${RELP}" 2>/dev/null)" || BLOB_AT_M=""
    if [ -z "$BLOB_AT_M" ]; then
      emit false "$VERSION" "" false \
        "historical-M eligibility: ${RELP} missing at derived M (${M_SHA}) — the ceremony output must exist at the manifest commit" \
        null null false true
      exit 1
    fi
    if [ "$BLOB_AT_H" != "$BLOB_AT_M" ]; then
      emit false "$VERSION" "" false \
        "historical-M eligibility: ${RELP} content differs between M (${M_SHA}) and HEAD (${H_SHA}) — the ceremony surface changed after the manifest commit; re-run the ceremony" \
        null null false true
      exit 1
    fi
  done
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
      null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  # 1. Requested version must match the override-release-version argument.
  if [ "$OVERRIDE_RELEASE_VERSION" != "$VERSION" ]; then
    emit false "$VERSION" "" false \
      "override ceremony: --override-release-version ($OVERRIDE_RELEASE_VERSION) != requested version ($VERSION)" \
      null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  # 2. Manifest blob SHA must match the COMMITTED manifest at the ceremony
  #    anchor (HEAD in the classic flow; the derived M in the historical
  #    flow — overrides read from M's manifest, which is what the gates
  #    evaluate).
  #
  # The override ceremony binds to the committed blob SHA (what CI will
  # also see), NOT to a `git hash-object` of the worktree file. A dirty
  # worktree edit could otherwise swap the bytes the ceremony confirms vs.
  # the bytes CI verifies. `git rev-parse <anchor>:<path>` resolves the blob
  # recorded in the anchor's tree and is invariant under worktree edits. If
  # the manifest is not committed at the anchor, the ceremony refuses: there
  # is no committed authority to bind to.
  ACTUAL_SHA="$(git rev-parse "${ANCHOR_REF}:$MANIFEST_REL" 2>/dev/null)" || {
    emit false "$VERSION" "" false \
      "override ceremony: manifest not committed at ${ANCHOR_REF}:$MANIFEST_REL (cannot confirm SHA)" \
      null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  }
  if [ "$OVERRIDE_MANIFEST_SHA" != "$ACTUAL_SHA" ]; then
    emit false "$VERSION" "" false \
      "override ceremony: --override-manifest-sha ($OVERRIDE_MANIFEST_SHA) != actual committed manifest blob ($ACTUAL_SHA)" \
      null null false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  CONFIRMED_VERSION="$VERSION"
fi

# --- release DEFER gate (authoritative hard enforcement) ---
#
# The deterministic evaluator at .opencode/scripts/check-defer-triggers.mjs is
# the SINGLE source of DEFER classification truth. The readiness agent's G7 is
# phase-honest (it runs NO evaluator — release mode's freshness handshake is
# satisfiable only at HEAD=M, which never exists at the readiness phase); THIS
# gate is the AUTHORITATIVE post-M surface: a blocker or evaluator-error
# classification REFUSES the release before any `git tag` mutation. DEFERs
# stay non-blocking at COMMIT time (hard non-goal) — this gate fires only at
# release-tag time.
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

# PRIOR_TAG feeds the G0c doctor env below (VH_HARNESS_DEFER_DIFF_SINCE) and,
# in historical-M mode, the defer-liveness anchor supplement's --since operand
# (resolved to its full commit SHA there). Release mode takes no --since
# operand: mainRelease derives its release base itself (options.since is read
# only by promoter and release-prep modes).
#
# Release-base reconciliation (contradiction fix): the canonical prior tag is
# derived ONCE, from the SAME anchor the gates evaluate — `git describe --tags
# --abbrev=0 <anchor>`. In the classic flow the anchor is HEAD (byte-identical
# to the historical behavior of a bare describe); in the historical flow it is
# the derived M, so G0c's diff predicate and G7's evaluator see the SAME
# release boundary (M's reachable prior tag), never HEAD's. At pre-tag time
# (the only time this wrapper creates tags) the describe form and the
# evaluator's `tag --merged <anchor> --sort=-v:refname` exclusion form agree,
# because the about-to-be-cut version is not yet a tag; a re-run after the tag
# exists is refused earlier by the tag-existence check.
PRIOR_TAG="$(git describe --tags --abbrev=0 "$ANCHOR_REF" 2>/dev/null || true)"
DEFER_ARGS=(--mode=release)
DEFER_ARGS+=(--release-version "$VERSION")
if [ "$HISTORICAL" = "1" ]; then
  # Historical flow: the evaluator anchors at the DERIVED M (manifest blob,
  # blob SHA, freshness handshake, single-path diff, and release-base
  # reachability all resolve from M — see check-defer-triggers.mjs --commit).
  # The classic flow deliberately passes NO --commit so the evaluator's
  # anchor stays the literal HEAD — identical classic anchor and error
  # behavior, with one additive envelope field (evaluated_ref, which reads
  # "HEAD" here and carries the explicit SHA in the historical flow); the
  # envelope is NOT byte-identical to the pre-historical shape.
  DEFER_ARGS+=(--commit "$M_SHA")
fi
if [ -n "$CONFIRMED_VERSION" ]; then
  DEFER_ARGS+=(--override-confirmed-version "$CONFIRMED_VERSION")
fi

DEFER_OUTPUT=""
DEFER_EXIT=0
DEFER_OUTPUT=$(node .opencode/scripts/check-defer-triggers.mjs "${DEFER_ARGS[@]}" 2>/dev/null) || DEFER_EXIT=$?

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
  emit false "$VERSION" "" false "$PARSED_REASON" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
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
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
fi

# --- historical-M verification isolation (wrapper-owned worktree) ---
#
# In the HISTORICAL flow, G0 must evaluate M's TREE, not H's. The wrapper
# creates a DETACHED temporary git worktree at M under the repo's own tmp/
# scratch space and runs the Go commands there. This is the ONLY sanctioned
# use of `git worktree` in the release path, and it is WRAPPER-OWNED:
# agents retain no raw checkout/reset/worktree authority (shell-guard's
# git-mutation-bypass still denies those verbs to every agent); the wrapper
# is the sole administration surface for this scratch worktree, exactly as
# it already is for `git tag`.
#
# Isolation contract:
#   - the worktree path is constructed by THIS wrapper as
#     <repo-root>/tmp/release-verify-<version>-<pid> and canonicalized;
#     any candidate path that would resolve OUTSIDE <repo-root>/tmp/ is
#     rejected (path-escape refusal) — the worktree can never land in
#     .local/, .git/, or any out-of-repo location;
#   - the checkout is DETACHED (no branch is created, moved, or checked
#     out; the original branch/HEAD/index are untouched);
#   - .local/ is NEVER copied or symlinked into the worktree — live
#     transport stays on the original checkout, which is why doctor's
#     control-class checks are evaluated there and only target/both
#     classes are read from the isolated tree;
#   - stale-run recovery: a previous wrapper instance killed by SIGKILL
#     cannot clean up after itself. At historical-flow start, registered
#     worktrees under tmp/release-verify-<version>-* from earlier runs are
#     removed (validated: listed by `git worktree list` AND under the
#     repo's tmp/) before a fresh one is created;
#   - cleanup runs BEFORE the tag mutation and again on EXIT (interruption
#     safety for gate failures). A pre-tag cleanup FAILURE refuses the
#     release — a leaked verification worktree must never be papered over
#     by cutting the tag anyway.
X_DIR=""
X_CLEANED=0
cleanup_verify_worktree() {
  if [ "$X_CLEANED" = "1" ] || [ -z "$X_DIR" ]; then
    return 0
  fi
  if git worktree list --porcelain 2>/dev/null | grep -Fqx "worktree ${X_DIR}"; then
    git worktree remove --force "$X_DIR" >/dev/null 2>&1 || true
  fi
  if [ -d "$X_DIR" ]; then
    # worktree remove can leave the dir when the gitfile dance fails; the dir
    # is wrapper-owned scratch under repo tmp/ — direct removal is safe and
    # sanctioned for THIS path only.
    rm -rf "$X_DIR"
  fi
  git worktree prune >/dev/null 2>&1 || true
  if [ -d "$X_DIR" ]; then
    X_CLEANED=0
    return 1
  fi
  X_CLEANED=1
  return 0
}
if [ "$HISTORICAL" = "1" ]; then
  # G0b (PRE-ISOLATION) — the dirty-tree check on the ORIGINAL checkout
  # happens BEFORE the verification worktree is created (and again at the
  # mutation boundary below): a dirty original refuses no matter how clean
  # the detached X at M is, and refusing early means no worktree exists to
  # clean up at all. X being clean by construction cannot substitute for a
  # clean original — the tag-time state and the gates' observed state must
  # not diverge.
  G0B_EARLY_OUTPUT=$(git status --short 2>&1) || true
  if [ -n "$G0B_EARLY_OUTPUT" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0b dirty worktree (historical pre-isolation check; commit or stash before tagging); git status --short: $G0B_EARLY_OUTPUT" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || REPO_ROOT=""
  if [ -z "$REPO_ROOT" ]; then
    emit false "$VERSION" "" false "historical-M isolation: cannot resolve repo root (git rev-parse --show-toplevel failed)" null null false true
    exit 1
  fi
  # Stale-run sweep FIRST (SIGKILL recovery): remove registered leftover
  # verification worktrees for THIS version from earlier runs.
  STALE_DIRS="$(git worktree list --porcelain 2>/dev/null | awk -v root="$REPO_ROOT" '$1=="worktree" && index($2, root "/tmp/release-verify-'"$VERSION"'-")==1 {print $2}')"
  if [ -n "$STALE_DIRS" ]; then
    while IFS= read -r SD; do
      [ -z "$SD" ] && continue
      git worktree remove --force "$SD" >/dev/null 2>&1 || true
      [ -d "$SD" ] && rm -rf "$SD"
    done <<< "$STALE_DIRS"
    git worktree prune >/dev/null 2>&1 || true
  fi
  X_DIR="${REPO_ROOT}/tmp/release-verify-${VERSION}-$$"
  # Path-escape rejection: the canonical parent must be <root>/tmp.
  X_PARENT="$(cd "${REPO_ROOT}/tmp" 2>/dev/null && pwd -P)" || X_PARENT=""
  if [ -z "$X_PARENT" ] || [ "${X_PARENT}" != "${REPO_ROOT}/tmp" ]; then
    emit false "$VERSION" "" false "historical-M isolation: repo tmp/ scratch directory missing or unresolved (${REPO_ROOT}/tmp)" null null false true
    exit 1
  fi
  if ! git worktree add --detach "$X_DIR" "$M_SHA" >/dev/null 2>&1; then
    emit false "$VERSION" "" false "historical-M isolation: git worktree add --detach at M (${M_SHA}) failed" null null false true
    exit 1
  fi
  # Interruption safety: any exit after worktree creation attempts cleanup.
  # The pre-tag cleanup (below) is the one whose FAILURE refuses; this trap
  # is best-effort so gate refusals and signals do not leak the worktree.
  trap 'cleanup_verify_worktree >/dev/null 2>&1 || true' EXIT INT TERM HUP
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
# G6-structure (S2 skill-pilot cross-check) is now ENFORCED deterministically
# below — see the "release S2-hold deterministic gate (G6)" block that runs
# after G1-G5 and before `git tag -a`. The evaluator at
# .opencode/scripts/check-s2-holds.mjs derives G6 from committed backlog rows
# (active + archive) + evidence packets joined by a stable hold ID, so a model
# surface (the readiness report's G6 verdict) cannot bypass it.
#
# Cost: G0 typically takes ~15s on this repo (dominated by `go test ./...`).
# This is the price of mechanical readiness enforcement at the tag
# boundary — the wrapper is the sole tag-cutting surface and verifying
# readiness at tag time is its job.

# G0 — green tree. Run the four Go commands in order; capture the first
# failing command's output for the refusal reason. `gofmt -l` is special:
# it exits 0 even when it lists files, so we treat ANY non-empty stdout
# as a formatting failure.
#
# HISTORICAL flow: G0 runs INSIDE the detached verification worktree X at M
# (subshell cd, so the wrapper's own cwd is unchanged). The verdict is
# therefore bound to M's tree — a defect introduced before M still refuses
# even if repaired at H, and a repair that landed after M cannot backdate
# greenness onto M. CLASSIC flow: unchanged, in the original checkout
# (M == HEAD, so the two are the same tree).
G0_REASON=""
G0_OUTPUT=""
if [ "$HISTORICAL" = "1" ]; then
  if ! G0_OUTPUT=$( (cd "$X_DIR" && go test ./... 2>&1) ); then
    G0_REASON="release-readiness-gate: G0 go test ./... failed at M (verification worktree)"
  elif ! G0_OUTPUT=$( (cd "$X_DIR" && go vet ./... 2>&1) ); then
    G0_REASON="release-readiness-gate: G0 go vet ./... failed at M (verification worktree)"
  elif ! G0_OUTPUT=$( (cd "$X_DIR" && go build ./... 2>&1) ); then
    G0_REASON="release-readiness-gate: G0 go build ./... failed at M (verification worktree)"
  else
    G0_OUTPUT=$( (cd "$X_DIR" && gofmt -l . 2>&1) ) || true
    if [ -n "$G0_OUTPUT" ]; then
      G0_REASON="release-readiness-gate: G0 gofmt -l . reported unformatted files at M (verification worktree)"
    fi
  fi
else
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
fi
if [ -n "$G0_REASON" ]; then
  emit false "$VERSION" "" false \
    "${G0_REASON} (output: $G0_OUTPUT)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# G0b — clean worktree. A dirty worktree at tag time means the tag would
# not capture uncommitted edits, breaking the contract that the tagged
# commit is exactly what was reviewed. This also closes the worktree-vs-
# HEAD bypass class at the wrapper level: even if a future gate read the
# worktree file, G0b prevents a dirty worktree from reaching the tag
# mutation.
#
# HISTORICAL flow: this mid-run G0b is SKIPPED — the ORIGINAL checkout was
# already checked BEFORE isolation (the early pre-isolation block) and is
# re-checked at the mutation boundary (after cleanup, below). Between those
# two points the wrapper's own verification worktree X lives under <repo>/tmp/
# and would appear as untracked scratch (`?? tmp/`) in a repo that does not
# gitignore tmp/, producing a FALSE refusal; the early + final checks bound
# the window without that artifact. CLASSIC flow: unchanged, in place.
if [ "$HISTORICAL" != "1" ]; then
  G0B_OUTPUT=$(git status --short 2>&1) || true
  if [ -n "$G0B_OUTPUT" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0b dirty worktree (commit or stash before tagging); git status --short: $G0B_OUTPUT" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false false
    exit 1
  fi
fi

# G0c — vh-agent-harness doctor HEALTHY. The check count is deliberately NOT
# pinned in prose here (this comment has said 21, then 28, and drifted both
# times): the release-context routing map in internal/cli/doctor.go
# (doctorReleaseRouting) is the source of truth — every name doctor can emit
# carries a target/control/both class there (31 names today), and
# doctor_json_test.go fails when a new check is added without one.
# This is the mandatory machine gate that makes `vh-agent-harness doctor` a
# HARD ceremony stop: any
# problem-tier or fail-tier check (including #12 defer-liveness, #13
# staged-errata-content, #14 behavioral-closure, #15 dev-stale-embed,
# #20 complexity-advisory, and #21 recurrence-state)
# refuses the tag BEFORE the readiness-pass artifact gate or any tag mutation.
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
# seam-managed tree" marker, so each G0c doctor side runs only when the lineage
# record is present ON ITS SIDE (see the per-side applicability block below).
# This is a STRUCTURAL gate (file/blob presence), not a runtime env-var bypass
# — a production release cannot select it, and deleting a committed managed
# file is itself refused by the historical eligibility scan (lineage.yml is a
# protected path). Push-only mode exits before reaching this gate (the tag
# already passed it at creation).
#
# CEREMONY-BINARY RESOLUTION (Layer 1 of the stale-PATH-binary recurrence fix,
# §4.7 correction-as-specification): the release ceremony already requires
# `make build` at start, so ./bin/vh-agent-harness EXISTS and is the
# authoritative ceremony binary. G0c resolves it DETERMINISTICALLY and NEVER
# from PATH. A stale PATH-resolved binary (e.g. an older installed copy at
# ~/.local/bin/ or ~/go/bin/) is the defect class that halted THREE consecutive
# releases — v0.15.0 (dev-stale-binary: PATH 0.15.1 predated f2-pairs #18, so
# doctor on PATH did not run #18), v0.17.0 (residual "reinstall PATH binary"
# for stale b4fccab), and v0.18.0 (STOP-AND-ASK #1: doctor UNHEALTHY on the
# stale PATH binary, recovered via PATH="$PWD/bin:$PATH") — all for a
# mechanical, decision-free recovery. After this pin, PATH staleness is at most
# an informational closeout note ("your installed binary is older than the
# release — run your install step"), never a gate input.
#
# If ./bin/vh-agent-harness is missing (the ceremony skipped `make build`),
# G0c refuses with a clear "run `make build` first" message. This is a
# recipe-list red (the operator-interaction contract's AUTO-RECOVER class: the
# releaser applies the recipe — `make build` + retry via ./bin/vh-agent-harness
# — and retries once, logging the recovery), NOT a PATH staleness.
HARNESS_BIN="./bin/vh-agent-harness"

# --- G0c applicability is PER-SIDE (round-2 activation-bypass fix) ---
#
# The old shape nested BOTH doctor runs under a single
# `[ -f .vh-agent-harness/lineage.yml ]` evaluated on the ORIGINAL checkout
# (H). A post-M commit DELETING the lineage record then passed every
# eligibility check (lineage.yml was in neither the protected-path scan nor
# the byte-equality set) while switching the entire G0c block off — a
# target-class-defective M could be tagged with NO health verdict at all.
# Two independent fixes:
#
#   1. PER-SIDE APPLICABILITY — each doctor run is gated by lineage
#      existence ON ITS OWN SIDE:
#        - the CONTROL run (full doctor, original checkout) gates on the
#          worktree file at H (unchanged behavior); and
#        - the TARGET run (release-context doctor against X) gates on the
#          blob at the DERIVATION commit: `git cat-file -e M:<lineage>`.
#          Blob existence at M — never the H worktree file — decides whether
#          M's tree gets a doctor verdict.
#   2. PROTECTED-PATH SCAN — .vh-agent-harness/lineage.yml joined the
#      protected set (see the eligibility scan above), so an M..H edit OR
#      deletion refuses eligibility before G0c is even reached. The per-side
#      gates remain the structural defense: each side derives applicability
#      from its own tree, so neither can be toggled by the other side's
#      state even if the scan were ever narrowed.
G0C_CONTROL=0
G0C_TARGET=0
if [ -f "$LINEAGE_REL" ]; then
  G0C_CONTROL=1
fi
if [ "$HISTORICAL" = "1" ] && git cat-file -e "${M_SHA}:${LINEAGE_REL}" 2>/dev/null; then
  G0C_TARGET=1
fi

# CEREMONY-BINARY GUARD: ANY side that needs doctor requires the ceremony
# binary to exist and be executable — a missing binary is a NAMED refusal,
# never a silent skip. (The target side can be the only side that needs it:
# when H's worktree lost the lineage record, the control side is off but the
# M-side blob still demands a verdict.)
if [ "$G0C_CONTROL" = "1" ] || [ "$G0C_TARGET" = "1" ]; then
  if [ ! -x "$HARNESS_BIN" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c ceremony binary missing — $HARNESS_BIN not found or not executable; run \`make build\` first (the ceremony-local binary is authoritative, never a PATH-resolved copy)" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  # STALENESS GUARD: the ceremony binary could predate check #13
  # (staged-errata-content) and report HEALTHY while lacking the enforcement
  # this gate exists to provide. The canary `$HARNESS_BIN release inject-errata
  # --help` exits 0 only on a binary that has the new subcommand; a stale
  # ceremony binary fails it and G0c refuses with a clear "rebuild" message.
  if ! "$HARNESS_BIN" release inject-errata --help >/dev/null 2>&1; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c staleness guard — the ceremony binary $HARNESS_BIN predates the staged-errata-content enforcement (no \`release inject-errata\` subcommand); run \`make build\` so it reflects the current source, then retry" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
fi

# CONTROL RUN — full doctor on the ORIGINAL checkout (H's real environment).
# G0c runs doctor AFTER the ceremony created the about-to-release migration
# note, so the F4-C release-diff-recurrence predicate (doctor #12, second
# contradiction class) is ACTIVE here: an OPEN defer card whose path_touched
# target re-fires in PRIOR_TAG..HEAD without a disposition makes doctor
# UNHEALTHY and refuses the tag. The diff context is threaded explicitly via
# VH_HARNESS_DEFER_DIFF_SINCE so the predicate and this script agree on the
# release boundary (defense-in-depth: the predicate self-derives the same ref
# via `git describe --tags --abbrev=0` when the env var is unset).
if [ "$G0C_CONTROL" = "1" ]; then
  if ! G0C_OUTPUT=$(VH_HARNESS_DEFER_DIFF_SINCE="$PRIOR_TAG" "$HARNESS_BIN" doctor 2>&1); then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c doctor not HEALTHY (a machine check FAILED — run \`$HARNESS_BIN doctor\` for the full report). Doctor output: $G0C_OUTPUT" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
fi

# TARGET RUN (historical flow only — G0C_TARGET implies HISTORICAL=1) —
# release-context routed doctor target run. The control run above retained
# the FULL doctor at H (original checkout). This run points the SAME ceremony
# binary at the verification worktree X (M's tree) with --json and enforces
# the per-check release-context classes:
#     target | both  -> must NOT be FAIL at M (a defect at M repaired at H
#                       still refuses — the tag publishes M's declarations)
#     control        -> IGNORED here (evaluated by the control run above;
#                       X structurally lacks .local/, the .git ledger, and
#                       user-global state — a control verdict from X would
#                       be a structural false-FAIL, never an honest one)
#     unclassified   -> coverage_complete=false FAILS (an unknown check can
#                       never silently escape release-context routing)
# Warning/advisory semantics are PRESERVED on both runs: only FAIL-tier
# verdicts refuse (a WARN/INFO/SKIP in either run is surfaced, never
# escalated by the class routing).
if [ "$G0C_TARGET" = "1" ]; then
  # Capture stdout REGARDLESS of exit code. Doctor's exit contract is
  # nonzero-on-problems: ANY FAIL — including the control-class FAILs this
  # run deliberately IGNORES (X structurally lacks .local/, the .git
  # ledger, and user-global state) — exits non-zero while the FULL JSON
  # envelope still lands on stdout. A blanket `if !` pre-check on the exit
  # code would dead-code the class routing below: in a repo that commits
  # the coordinator-adoption marker, defer-liveness state-matrix row 4
  # (tasks absent + valid marker) FAILs on EVERY target run (the worktree
  # checkout never contains gitignored .local/coordinator/tasks), so the
  # exit-code pre-check would refuse every historical release before the
  # router ever ran. Refuse ONLY on spawn failure (126/127: not found /
  # not executable) or empty stdout; unparseable stdout is refused by the
  # router's own parse-failure arm. The routing decisions (target|both
  # FAIL, coverage_complete=false) stay exactly where they were.
  G0CT_OUTPUT=""
  G0CT_EXIT=0
  G0CT_OUTPUT=$("$HARNESS_BIN" doctor --target "$X_DIR" --json 2>/dev/null) || G0CT_EXIT=$?
  if [ "$G0CT_EXIT" -eq 126 ] || [ "$G0CT_EXIT" -eq 127 ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c release-context target run failed to spawn (doctor --target $X_DIR --json exit=${G0CT_EXIT} — ceremony binary not found or not executable)" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  if [ -z "$G0CT_OUTPUT" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0c release-context target run produced no report (doctor --target $X_DIR --json exit=${G0CT_EXIT}, empty stdout)" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  G0CT_VERDICT="$(printf '%s' "$G0CT_OUTPUT" | node -e '
    let d = ""; process.stdin.on("data", (c) => (d += c));
    process.stdin.on("end", () => {
      function out(ok, reason) {
        process.stdout.write(JSON.stringify({ ok: ok, reason: reason }));
      }
      let o;
      try { o = JSON.parse(d); } catch (e) {
        out(false, "release-readiness-gate: G0c release-context target run produced unparseable JSON");
        return;
      }
      // Malformed-envelope hardening (round-2 vacuous-pass advisory): a
      // parseable but wrong-shaped envelope used to sail through — a `{}`
      // payload skipped the coverage_complete === false probe (undefined
      // is not false) and a missing/empty checks array produced zero
      // routed failures, yielding a vacuous ok:true. Require the doctor
      // --json contract — schema_version === 1, BOOLEAN coverage_complete,
      // and a NON-EMPTY checks array — and treat anything else as
      // unparseable. Round-5 (rejection #4 blockers C-F1/C-F2) extends the
      // same fail-closed posture to per-ENTRY shape: a checks:[null] or
      // checks:[{}] envelope still vacuously passes out(true, "") — the
      // FAIL-routing loop below matches nothing — which would permit tag
      // mutation with no meaningful target verdict, so every checks[]
      // entry is validated by the per-entry guard below.
      if (o === null || typeof o !== "object" ||
          o.schema_version !== 1 ||
          typeof o.coverage_complete !== "boolean" ||
          !Array.isArray(o.checks) || o.checks.length === 0) {
        out(false, "release-readiness-gate: G0c release-context target run produced unparseable or malformed JSON (envelope must carry schema_version=1, boolean coverage_complete, and a non-empty checks array)");
        return;
      }
      if (o.coverage_complete === false) {
        const un = Array.isArray(o.unclassified) ? o.unclassified.join(", ") : "?";
        out(false, "release-readiness-gate: G0c release-context coverage INCOMPLETE — unclassified doctor check(s): " + un + " (every registered check must carry a release-context class)");
        return;
      }
      // Per-entry shape guard (round-5, rejection #4 blockers C-F1/C-F2):
      // the doctor --json per-check schema (README.agent.md, section
      // "Machine-readable doctor") requires every checks[] entry to be a
      // non-null object carrying (a) a NON-EMPTY string name, (b) a tier
      // from the closed vocabulary the envelope itself uses
      // (PASS/WARN/FAIL/SKIP/INFO — the tokens the doctor emitter admits),
      // and (c) a class in the routing vocabulary target|control|both.
      // Degenerate entries (null, {}, missing/unknown tier, missing/
      // unknown class) carry no meaningful verdict, so without this guard
      // out(true, "") fires vacuously — tag mutation with no target
      // verdict at all. The guard runs AFTER the coverage probe on
      // purpose: the REAL emitter writes class:"" for unclassified checks
      // together with coverage_complete:false, and that envelope keeps its
      // sharper coverage-INCOMPLETE refusal; an envelope that CLAIMS
      // coverage_complete:true while carrying an entry outside the
      // vocabularies is self-inconsistent and lands HERE as malformed —
      // fail-closed, same refusal shape as the top-level guard above.
      const tierVocab = { PASS: true, WARN: true, FAIL: true, SKIP: true, INFO: true };
      const classVocab = { target: true, control: true, both: true };
      const vocabHit = (m, v) => Object.prototype.hasOwnProperty.call(m, v);
      for (const c of o.checks) {
        if (c === null || typeof c !== "object" ||
            typeof c.name !== "string" || c.name === "" ||
            !vocabHit(tierVocab, c.tier) ||
            !vocabHit(classVocab, c.class)) {
          out(false, "release-readiness-gate: G0c release-context target run produced unparseable or malformed JSON (every checks[] entry must be a non-null object with a non-empty string name, a tier of PASS/WARN/FAIL/SKIP/INFO, and a class of target/control/both)");
          return;
        }
      }
      const bad = [];
      for (const c of o.checks) {
        if (c && c.tier === "FAIL" && (c.class === "target" || c.class === "both")) {
          bad.push(c.name + " (" + c.class + ")");
        }
      }
      if (bad.length) {
        out(false, "release-readiness-gate: G0c release-context TARGET failure at M — " + bad.join(", ") + " (defect at M refuses the tag even when repaired at HEAD)");
        return;
      }
      out(true, "");
    });
  ' 2>/dev/null)" || G0CT_VERDICT='{"ok":false,"reason":"release-readiness-gate: G0c release-context verdict parse crashed"}'
  G0CT_OK=$(printf '%s' "$G0CT_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).ok?"1":"0");})') || G0CT_OK="0"
  G0CT_REASON=$(printf '%s' "$G0CT_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).reason||"");})') || G0CT_REASON=""
  if [ "$G0CT_OK" != "1" ]; then
    emit false "$VERSION" "" false "$G0CT_REASON" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
fi

# --- historical-M defer-liveness anchor supplement (G0c, round-4 C-F1) ---
#
# THE GAP THIS CLOSES: in historical-M mode the G0c control run (doctor on
# the ORIGINAL checkout H with VH_HARNESS_DEFER_DIFF_SINCE=$PRIOR_TAG)
# evaluates defer-liveness recurrence over PRIOR_TAG..H (a one-ended
# `git diff` against H's working tree), while the G0c target run at the M
# worktree routes defer-liveness as control-class and ignores it. An OPEN
# undisposed path_touched(<path>) card whose path is touched in the M
# release arc (PRIOR_TAG..M) and then RESTORED to prior-tag bytes by an
# eligible post-M chore is invisible to BOTH runs — the restored path has
# no entry in H's diff — so the tag would cut despite a firing undisposed
# card. This supplement evaluates recurrence over the arc the tag actually
# certifies:
#
#   ANCHOR — PRIOR_TAG..M (M's release arc), never PRIOR_TAG..H (H's arc).
#     The tag certifies M's diff; recurrence must be judged over exactly
#     that window. Mechanism: run the same rendered evaluator
#     (check-defer-triggers.mjs) in --mode=release-prep FROM the M worktree
#     X, so its one-ended `git diff --name-only <since>` (cwd = the
#     script's own repoRoot(), which IS X) evaluates the clean M checkout —
#     the diff is exactly PRIOR_TAG..M.
#
#   CARD SOURCE — H's live registry ($REPO_ROOT/.local/coordinator/tasks).
#     Cards live at H, not at M: the M worktree checkout never contains the
#     gitignored .local/ transport. H's registry is the only live card
#     state at tag time; --tasks pins the evaluator to it. X's evaluator
#     and manifest bytes equal H's committed bytes (the protected-path scan
#     gating M's eligibility guarantees no M..H drift there), so cards
#     disposed in M's committed manifest are honored — this is anchoring,
#     not evaluator/manifest substitution.
#
#   DETERMINISM — --since is the full 40-hex commit SHA of PRIOR_TAG
#     (rev-parse '<tag>^{commit}'), never the tag name or a short ref.
#
#   GREENFIELD — no PRIOR_TAG: doctor's own recurrence predicate is
#     fail-safe dormant on the same boundary (nothing to diff against), so
#     the supplement skips identically rather than inventing a stricter
#     window than the control-side predicate itself.
#
#   CLASSIC FLOW — this step exists ONLY in historical-M mode (H != M).
#     The classic flow's single control run already evaluates recurrence
#     over the exact arc being tagged; its behavior is byte-unchanged.
#
#   FAIL-CLOSED — evaluator missing at X, spawn failure, empty stdout, an
#     unparseable or wrong-shaped envelope, or exit 2 (git-unusable inside
#     the worktree) each refuse the ceremony as an evaluator error, never a
#     silent pass. H's registry being ABSENT is not an error (release-prep
#     exits clear): the control run's defer-liveness row 4 (valid adoption
#     marker + absent transport) owns that verdict.
if [ "$HISTORICAL" = "1" ] && [ -n "$PRIOR_TAG" ]; then
  MDL_EVAL="$X_DIR/.opencode/scripts/check-defer-triggers.mjs"
  if [ ! -f "$MDL_EVAL" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: historical-M defer-liveness anchor check unavailable — evaluator missing at the M worktree ($MDL_EVAL); refusing fail-closed" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  MDL_SINCE="$(git rev-parse "${PRIOR_TAG}^{commit}" 2>/dev/null)"
  if ! printf '%s' "$MDL_SINCE" | grep -Eq '^[0-9a-f]{40}$'; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: historical-M defer-liveness anchor check unavailable — could not resolve prior tag $PRIOR_TAG to a full commit SHA (got '${MDL_SINCE:-nothing}'); refusing fail-closed" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  MDL_OUTPUT=""
  MDL_EXIT=0
  MDL_OUTPUT="$(node "$MDL_EVAL" --mode=release-prep --since "$MDL_SINCE" --tasks "$REPO_ROOT/.local/coordinator/tasks" 2>/dev/null)" || MDL_EXIT=$?
  if [ "$MDL_EXIT" -eq 126 ] || [ "$MDL_EXIT" -eq 127 ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: historical-M defer-liveness anchor check failed to spawn (node $MDL_EVAL --mode=release-prep exit=${MDL_EXIT})" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  MDL_VERDICT="$(printf '%s\n%s' "$MDL_EXIT" "$MDL_OUTPUT" | node -e '
    let d = ""; process.stdin.on("data", (c) => (d += c));
    process.stdin.on("end", () => {
      function out(ok, reason) {
        process.stdout.write(JSON.stringify({ ok: ok, reason: reason }));
      }
      const nl = d.indexOf("\n");
      const exitCode = nl === -1 ? d.trim() : d.slice(0, nl);
      const body = nl === -1 ? "" : d.slice(nl + 1);
      if (body.trim() === "") {
        out(false, "release-readiness-gate: historical-M defer-liveness anchor check unusable — evaluator in the M worktree exited " + exitCode + " with no report (exit 2 = git-unusable in the worktree); refusing fail-closed");
        return;
      }
      let o;
      try { o = JSON.parse(body); } catch (e) {
        out(false, "release-readiness-gate: historical-M defer-liveness anchor check unusable — evaluator in the M worktree exited " + exitCode + " with unparseable output; refusing fail-closed");
        return;
      }
      if (o === null || typeof o !== "object" || o.mode !== "release-prep" ||
          typeof o.classification !== "string" || !Array.isArray(o.missing_disposition)) {
        out(false, "release-readiness-gate: historical-M defer-liveness anchor check unusable — evaluator in the M worktree returned a wrong-shaped envelope (expected mode=release-prep with classification + missing_disposition); refusing fail-closed");
        return;
      }
      if (o.classification === "clear") {
        out(true, "");
        return;
      }
      if (o.classification !== "blocker" || o.missing_disposition.length === 0) {
        out(false, "release-readiness-gate: historical-M defer-liveness anchor check unusable — evaluator in the M worktree returned classification=" + o.classification + " with " + o.missing_disposition.length + " missing_disposition entries (malformed envelope); refusing fail-closed");
        return;
      }
      const cards = o.missing_disposition.map((m) => {
        const t = Array.isArray(m.fired_targets) ? m.fired_targets.join(", ") : "?";
        return m.defer_id + " [status=" + (m.status || "open") + ", card=" + (m.file || "?") + "] path_touched fired on: " + t;
      });
      out(false, "release-readiness-gate: historical-M defer-liveness REFUSED — OPEN undisposed card(s) firing in the M release arc (" + o.diff_since + "..M); the control run could not see these because the touched path was restored after M: " + cards.join("; ") + " — dispose each firing card (complete/cancel it, or carry its disposition in the release manifest) before tagging");
    });
  ' 2>/dev/null)" || MDL_VERDICT='{"ok":false,"reason":"release-readiness-gate: historical-M defer-liveness anchor check verdict parse crashed"}'
  MDL_OK=$(printf '%s' "$MDL_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).ok?"1":"0");})') || MDL_OK="0"
  MDL_REASON=$(printf '%s' "$MDL_VERDICT" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).reason||"");})') || MDL_REASON=""
  if [ "$MDL_OK" != "1" ]; then
    emit false "$VERSION" "" false "$MDL_REASON" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
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

# Resolve the ceremony anchors from the EVALUATION anchor (classic: HEAD —
# byte-identical to the historical behavior; historical: the derived M).
# Expected: ANCHOR^ = artifact commit (R), ANCHOR^^ = release-prep commit
# (N). If the history is too shallow, the readiness ceremony has not
# happened — refuse fail-closed.
HEAD_PARENT_SHA=""
if ! HEAD_PARENT_SHA=$(git rev-parse --verify "${ANCHOR_REF}^" 2>/dev/null); then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness ceremony (${ANCHOR_REF}^ does not exist; expected note -> artifact -> manifest sequencing with manifest at ${ANCHOR_REF})" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi
HEAD_GP_SHA=""
if ! HEAD_GP_SHA=$(git rev-parse --verify "${ANCHOR_REF}^^" 2>/dev/null); then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness ceremony (${ANCHOR_REF}^^ does not exist; expected note -> artifact -> manifest sequencing with manifest at ${ANCHOR_REF})" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# Artifact must exist at the anchor (committed). Use `git cat-file -e` to
# check existence without loading bytes.
if ! git cat-file -e "${ANCHOR_REF}:${READINESS_REL}" 2>/dev/null; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 missing readiness artifact at ${ANCHOR_REF}:${READINESS_REL} (readiness agent must evaluate and the releaser must commit the artifact as the manifest commit's parent before the manifest commit)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# Artifact commit (ANCHOR^) must be a single-path child of ANCHOR^^, and the
# single changed path must be the readiness artifact. Mirrors the DEFER
# manifest's single-path child check one level deeper.
ARTIFACT_DIFF=$(git diff --name-only "${ANCHOR_REF}^^" "${ANCHOR_REF}^" 2>/dev/null) || true
if [ "$ARTIFACT_DIFF" != "$READINESS_REL" ]; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 artifact commit (${ANCHOR_REF}^) must change only ${READINESS_REL}; got: ${ARTIFACT_DIFF}" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# Validate the artifact content (schema + extract verdict). Parse via
# node (mirrors the DEFER gate's JSON parsing pattern). The validator
# returns a JSON object: {ok, reason, commit_sha, gates}. `ok=false`
# means schema/enum validation failed and `reason` carries the refusal
# text. `ok=true` means the artifact is well-formed; the wrapper then
# checks commit_sha binding and all-ready below.
READINESS_BYTES=$(git show "${ANCHOR_REF}:${READINESS_REL}" 2>/dev/null) || true
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
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# commit_sha binding: the artifact must be pinned to the release-prep
# commit (ANCHOR^^), which is what the readiness agent evaluated. This
# prevents stale artifacts (evaluated against an older commit) from
# authorizing a release built on a different commit.
if [ "$RV_COMMIT" != "$HEAD_GP_SHA" ]; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 artifact commit_sha (${RV_COMMIT}) does not match release-prep commit ${ANCHOR_REF}^^ (${HEAD_GP_SHA}); the readiness artifact must be re-evaluated and re-committed against the current release-prep commit" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# All five model-driven gates must be `ready`. Any blocked/skipped
# (unknown was already refused at schema validation) → refuse.
if printf '%s' "$RV_GATES" | grep -Eq '=blocked|=skipped'; then
  emit false "$VERSION" "" false \
    "release-readiness-gate: G1-G5 not all ready (${RV_GATES}); every model-driven gate must report ready before tagging" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# --- release S2-hold deterministic gate (G6) ---
#
# Independent re-computation at tag time of the G6 (skill-pilot S2-hold) gate.
# The wrapper DERIVES G6 from committed primary state (HEAD:), so a model
# surface (the readiness report's G6 verdict) cannot bypass it. This preserves
# the sacred invariant: model output is never transition authority. The
# readiness agent's G6 verdict is NOT consumed here.
#
# The deterministic evaluator at .opencode/scripts/check-s2-holds.mjs reads
# COMMITTED backlog rows (active + archive) and evidence packets
# (researches/sources/) joined by a stable hold ID, and refuses on any PENDING
# hold, status/verdict disagreement, or structurally invalid input. Mirrors
# the G7 two-tier model (check-defer-triggers.mjs): the same surface contract
# is consumed ADVISORY by the readiness agent and AUTHORITATIVELY here.
#
# This deterministic EVALUATOR + its machine-parseable contract ship from
# templates/core/ (platform_managed), so every consumer receives them. THIS
# wrapper (release-tag.sh, dogfood-local — not in templates/core) enforces the
# contract authoritatively at tag time; a consumer that wires its own release
# wrapper to the same evaluator gets the same enforcement. The readiness agent
# remains the dogfood demonstrator of the same contract (advisory).
#
# Revision binding: the evaluator is invoked with --commit at the EVALUATION
# anchor (the derived M in the historical flow; HEAD_SHA in the classic flow,
# where M == HEAD) so G6 is derived from the EXACT commit the wrapper will
# tag, not the moving HEAD. This closes the race where a concurrent commit
# resolves a hold in a newer HEAD while the tag stays pinned to the earlier
# captured commit.
#
# Classification (top-level): clear | blocker | evaluator-error (NO disclose —
# S2 holds are a binary gate). Both blocker and evaluator-error force ready:no
# and refuse BEFORE `git tag -a`. NO override cures an S2-hold block.
#   clear            — no holds, or every hold resolved (done+SATISFIED or
#                      cancelled+WITHDRAWN).
#   blocker          — structurally VALID, legitimate no-release: a PENDING
#                      hold or a status/verdict disagreement. Remedy: resolve
#                      the hold.
#   evaluator-error  — structurally INVALID: evidence missing/malformed/
#                      duplicate/unreadable; backlog token malformed; unknown
#                      verdict; join ambiguous (zero/>1); cancelled+s2-hold
#                      unresolved. Remedy: REPAIR records.
S2_OUTPUT=""
S2_EXIT=0
S2_COMMIT="$HEAD_SHA"
if [ "$HISTORICAL" = "1" ]; then
  S2_COMMIT="$M_SHA"
fi
S2_OUTPUT=$(node .opencode/scripts/check-s2-holds.mjs --commit "$S2_COMMIT" 2>/dev/null) || S2_EXIT=$?

# Parse the evaluator JSON, cross-check exit + classification, and produce a
# single pass/refuse verdict. Fail-closed: missing/crashed/unparseable
# evaluator, a classification other than clear, OR an exit code that disagrees
# with the classification all refuse BEFORE `git tag -a`.
S2_PARSED=$(printf '%s' "$S2_OUTPUT" | node -e '
  let data = "";
  process.stdin.on("data", (c) => (data += c));
  process.stdin.on("end", () => {
    function refuse(reason, classification) {
      process.stdout.write(JSON.stringify({ pass: false, classification: classification || "evaluator-error", reason: reason }));
    }
    if (!data) {
      refuse("release-s2-gate: evaluator produced no output (missing or crashed .opencode/scripts/check-s2-holds.mjs)");
      return;
    }
    let o;
    try { o = JSON.parse(data); } catch (e) {
      refuse("release-s2-gate: evaluator output unparseable");
      return;
    }
    const exitMap = { clear: 0, blocker: 1, "evaluator-error": 2 };
    const cls = o.classification;
    if (exitMap[cls] === undefined) {
      refuse("release-s2-gate: evaluator returned unknown classification " + JSON.stringify(cls));
      return;
    }
    const ids = Array.isArray(o.blocking_ids) && o.blocking_ids.length
      ? (" ids=[" + o.blocking_ids.join(",") + "]") : "";
    const errTail = o.error ? (" (" + o.error + ")") : "";
    process.stdout.write(JSON.stringify({
      pass: cls === "clear",
      classification: cls,
      expected_exit: exitMap[cls],
      reason: cls === "clear" ? "" : ("release-s2-gate: " + cls + ids + errTail),
    }));
  });
' 2>/dev/null) || true

if [ -z "$S2_PARSED" ]; then
  S2_PARSED='{"pass":false,"classification":"evaluator-error","reason":"release-s2-gate: evaluator verdict parse crashed"}'
fi

S2_PASS=$(printf '%s' "$S2_PARSED" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).pass?"1":"0");})') || S2_PASS=0
S2_CLASSIFICATION=$(printf '%s' "$S2_PARSED" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).classification||"");})') || S2_CLASSIFICATION="evaluator-error"
S2_EXPECTED_EXIT=$(printf '%s' "$S2_PARSED" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{const v=JSON.parse(d).expected_exit;process.stdout.write(v===undefined?"":String(v));})') || S2_EXPECTED_EXIT=""
S2_REASON=$(printf '%s' "$S2_PARSED" | node -e 'let d="";process.stdin.on("data",c=>d+=c);process.stdin.on("end",()=>{process.stdout.write(JSON.parse(d).reason||"");})') || S2_REASON="release-s2-gate: failed"

# Cross-check: the evaluator's exit code must match its classification. A
# mismatch (e.g. classification=clear but exit≠0, or a crash exit like 127 with
# a parseable payload) is a fail-closed evaluator-error — a buggy or stale
# evaluator must never silently pass.
S2_CROSS_OK=1
if [ -n "$S2_EXPECTED_EXIT" ] && [ "$S2_EXIT" != "$S2_EXPECTED_EXIT" ]; then
  S2_CROSS_OK=0
  S2_REASON="release-s2-gate: evaluator exit (${S2_EXIT}) disagrees with its classification (${S2_CLASSIFICATION}, expected exit ${S2_EXPECTED_EXIT})"
fi

if [ "$S2_PASS" != "1" ] || [ "$S2_CROSS_OK" != "1" ]; then
  emit false "$VERSION" "" false "$S2_REASON" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  if [ "$S2_CROSS_OK" = "1" ] && [ "$S2_CLASSIFICATION" = "blocker" ]; then
    exit 1
  fi
  exit 2
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

# --- tag-ready -> tag HEAD-drift detection (pin-to-validated-commit) ---
#
# The release ceremony establishes tag-ready state against ONE specific commit
# — the EVALUATION anchor (classic: HEAD_SHA captured before any gate ran;
# historical: the derived M). Every readiness gate (the DEFER manifest
# handshake, G0/G0b/G0c, and the G1-G5 artifact handshake bound to the
# anchor's parent chain) validated THAT commit. The window between that
# tag-ready assertion and the `git tag -a` mutation is non-trivial (~15s+ for
# G0's `go test ./...`), and in a dogfood repo where build agents run
# concurrently, a concurrent landing CAN advance HEAD mid-window.
#
# Before this guard, `git tag -a <version>` (no explicit commit) resolved HEAD
# at mutation time, so a concurrent commit silently re-targeted the tag off the
# validated manifest commit M onto whatever main pointed at mid-window. This is
# the v0.15.0 incident's defect class: tag target 40e5f74 while a concurrent
# study commit c3dcf29 landed on main, requiring operator history surgery to
# preserve the intended pin-to-M.
#
# Fix — PIN + DETECT, never silent — with an honesty correction on what the
# pin proves (contradiction fix: the original comment overstated the
# captured-SHA guarantee):
#   - CLASSIC flow: re-read HEAD at the mutation boundary. If it advanced past
#     HEAD_SHA, set HEAD_DRIFTED=true and print a LOUD stderr warning naming
#     both SHAs. The tag is still created with the EXPLICIT validated commit
#     argument, so the tag target is the manifest-pinned commit M regardless
#     of the drift — the explicit-commit pin bounds the TAG TARGET, which is
#     all it can honestly claim; it does NOT re-validate the gates against the
#     new HEAD (they validated the pinned commit), which is exactly why the
#     drift is surfaced loudly and the concurrent commit is excluded from the
#     release rather than silently absorbed.
#   - HISTORICAL flow: HEAD advancing past the H captured at start is NOT
#     drift-tolerable — eligibility (intervening-commit scan M..H) was
#     computed against THAT H, and a newer commit H2 has not been scanned for
#     protected-path changes. The historical flow therefore REVERSES the
#     classic tolerance: final drift check re-reads HEAD, and any movement
#     (vs the captured H) REFUSES. The re-run re-derives eligibility against
#     the new H cheaply (the scan is deterministic), restoring the
#     uninterrupted-ceremony guarantee. G0b is also re-checked here (final
#     drift checks on the original checkout before tagging).
HEAD_DRIFTED=false
CURRENT_HEAD=""
if ! CURRENT_HEAD=$(git rev-parse HEAD 2>/dev/null); then
  emit false "$VERSION" "" false \
    "head-drift check: git rev-parse HEAD failed at the mutation boundary (cannot verify the tag-ready commit is still current)" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi
if [ "$CURRENT_HEAD" != "$HEAD_SHA" ]; then
  if [ "$HISTORICAL" = "1" ]; then
    emit false "$VERSION" "" false \
      "historical-M final drift check: HEAD moved during the tag window (start ${H_SHA}, now ${CURRENT_HEAD}); eligibility was derived against ${H_SHA} — refusing, re-run the wrapper to re-derive against the new HEAD" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  HEAD_DRIFTED=true
  printf '[release-tag] HEAD-DRIFT DETECTED between tag-ready and tag: the validated commit was %s, but current HEAD is %s (a concurrent commit landed during the tag-ready window). The tag will be PINNED to the validated commit %s (manifest-pinned M); the concurrent commit is NOT part of this release.\n' \
    "$HEAD_SHA" "$CURRENT_HEAD" "$HEAD_SHA" >&2
fi

# HISTORICAL flow — final gates on the ORIGINAL checkout, in this order:
# cleanup FIRST (so the G0b re-check below sees a tree with no wrapper
# scratch in it), then the final G0b re-check, then the tag. The
# verification worktree is cleaned up BEFORE tagging; a cleanup FAILURE
# refuses the release (a leaked verification worktree must never be papered
# over by cutting the tag). The EXIT trap is disarmed implicitly: a
# successful cleanup sets X_CLEANED, making the trap's own call a no-op.
if [ "$HISTORICAL" = "1" ]; then
  if ! cleanup_verify_worktree; then
    emit false "$VERSION" "" false \
      "historical-M isolation: cleanup of the verification worktree (${X_DIR}) FAILED — refusing to tag; remove the worktree manually ('git worktree remove --force ${X_DIR}') and re-run" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
  # Final G0b re-check (post-cleanup, pre-tag): the original must still be
  # clean at the mutation boundary — nothing may dirty it mid-window.
  G0B2_OUTPUT=$(git status --short 2>&1) || true
  if [ -n "$G0B2_OUTPUT" ]; then
    emit false "$VERSION" "" false \
      "release-readiness-gate: G0b dirty worktree at the mutation boundary (historical flow); git status --short: $G0B2_OUTPUT" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi

  # Pre-tag HEAD re-compare (round-6 F8): the final drift check above ran
  # BEFORE worktree cleanup and the G0b re-check — HEAD may have advanced in
  # exactly that window, and eligibility (the M..H scan) was derived against
  # the captured H, so a HEAD that moved here has NOT been scanned. Refuse
  # with the same message shape as the final drift check instead of tagging
  # a superseded window; a re-run re-derives eligibility against the new HEAD
  # cheaply (the scan is deterministic).
  PRETAG_HEAD=""
  if ! PRETAG_HEAD=$(git rev-parse HEAD 2>/dev/null) || [ "$PRETAG_HEAD" != "$H_SHA" ]; then
    emit false "$VERSION" "" false \
      "historical-M pre-tag drift check: HEAD moved between the final drift check and the tag mutation (expected ${H_SHA}, now ${PRETAG_HEAD:-<unreadable>}); eligibility was derived against ${H_SHA} — refusing, re-run the wrapper to re-derive against the new HEAD" \
      "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" false true
    exit 1
  fi
fi

# --- mutation: create the annotated tag from the message file, PINNED to the validated commit ---
#
# The tag is created with the EXPLICIT validated commit (the evaluation
# anchor: HEAD_SHA in the classic flow, the derived M in the historical
# flow) — never bare HEAD — so a concurrent landing between tag-ready and
# tag cannot silently re-target the manifest pin. See the HEAD-drift
# detection block above for what this pin does and does not guarantee.
TAG_TARGET="$HEAD_SHA"
if [ "$HISTORICAL" = "1" ]; then
  TAG_TARGET="$M_SHA"
fi
TAG_ERR=""
if ! TAG_ERR=$(git tag -a -F "$MSG_FILE" "$VERSION" "$TAG_TARGET" 2>&1 1>/dev/null); then
  emit false "$VERSION" "" false "git tag -a failed: ${TAG_ERR}" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" "$HEAD_DRIFTED" "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  exit 1
fi

# Defense-in-depth: verify the created annotated tag points at the validated
# commit. This confirms the explicit-commit pin took effect; if it did not
# (e.g. a future git regression or an unexpected ref state), refuse BEFORE any
# push rather than ship a tag at the wrong commit.
TAGGED_SHA="$(git rev-parse -q --verify "refs/tags/${VERSION}^{commit}" 2>/dev/null)" || TAGGED_SHA=""
if [ "$TAGGED_SHA" != "$TAG_TARGET" ]; then
  emit false "$VERSION" "$TAGGED_SHA" false \
    "post-tag pin verification failed: tag ${VERSION} target (${TAGGED_SHA}) does not match the validated commit (${TAG_TARGET}); refusing before push" \
    "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" "$HEAD_DRIFTED" "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
  # Clean up the just-created local tag so a retry is not blocked by a stale
  # dangling tag ("tag already exists" at the tag-existence check). The
  # pin-verify failure was already recorded by emit above; this only removes
  # the orphaned local ref. Fail-safe: if the delete itself fails, still exit 1
  # so the original pin-verify failure is never masked.
  if ! git tag -d "$VERSION" >/dev/null 2>&1; then
    echo "release-tag: WARNING — failed to clean up local tag ${VERSION} after pin-verify failure; remove it manually with 'git tag -d ${VERSION}'" >&2
  else
    echo "release-tag: removed local tag ${VERSION} (cleaning up local tag after pin-verify failure)" >&2
  fi
  exit 1
fi

# --- optional opt-in push (default: local-only) ---
# The tag (and any push) is issued from the ORIGINAL repository only, after
# all final drift checks above — never from the verification worktree (which
# is already cleaned up by this point in the historical flow).
PUSHED=false
if [ "${RELEASE_TAG_PUSH-0}" = "1" ]; then
  PUSH_ERR=""
  if ! PUSH_ERR=$(git push origin "$VERSION" 2>&1 1>/dev/null); then
    # tag was created locally (at the validated commit) but the requested push failed
    emit false "$VERSION" "$TAG_TARGET" false "tag ${VERSION} created locally but git push failed: ${PUSH_ERR}" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" "$HEAD_DRIFTED" "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
    exit 1
  fi
  PUSHED=true
fi

emit true "$VERSION" "$TAG_TARGET" "$PUSHED" "" "$DISCLOSURES_JSON" "$ACCEPTED_OVERRIDES_JSON" "$HEAD_DRIFTED" "$([ "$HISTORICAL" = "1" ] && printf true || printf false)"
