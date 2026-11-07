#!/bin/sh
# validate-packet.sh — structural validator for deep-research run packets.
#
# Usage:  sh validate-packet.sh <run-packet.md>
# Exit:   0 = pass   1 = violations / skeleton deviation   2 = usage error
#
# Checks (machine-verifiable invariants ONLY):
#   C1  every Evidence-record table: header must match the canonical
#       skeleton header EXACTLY (ordered cells "id | locator | excerpt |
#       read_depth | retrieved_at | source_version | source_family",
#       7 columns); every data row: id matches E<digits>, non-empty
#       locator, read_depth starts full|skim (paren annotation allowed),
#       non-empty source_family, no duplicate ids, canonical column count,
#       and EMPTY outer pipe-boundary fields (nothing before the leading
#       or after the trailing unescaped pipe) — header and data rows alike.
#   C2  the classification cell must be one of supported|partial|
#       contradicted|unsupported|inference (case-sensitive exact base word,
#       paren annotation allowed) before C2/C3 apply. Every token in a
#       claim row's evidence column matches E<digits> AND is a declared Evidence record (referential
#       integrity). A parenthetical is ignored ONLY as an annotation
#       immediately following a valid E<digits> token (e.g. "E1 (spec
#       doc)"); standalone or misplaced parenthesized content is a
#       violation. A claim classified supported|partial with ZERO valid
#       evidence tokens (empty cell / gap markers only) is a violation.
#   C3  a claim classified `supported` citing exactly ONE evidence record
#       must carry the exact basis note "direct support (authoritative
#       spec, single family)" in notes — as the whole trimmed notes value
#       or a "—"/";"-delimited segment, never a substring of a larger
#       phrase — AND that record's read_depth must start with "full".
#   Claim-table headers must likewise match "claim | classification |
#   evidence_ids | notes" (4 columns) exactly before any data row is
#   interpreted.
#   Inside the Evidence-records and Claim-classifications sections, any
#   non-blank line containing "|" that does not start with optional
#   whitespace + "|" (the leading-pipe table-row shape) is a malformed-row
#   violation — such a row is otherwise silently excluded by the row
#   selector and every check above would ride past it. Lines without any
#   "|" (prose, bullets) are not table rows and are left alone.
#   C4  a "## Final report" section exists and contains the exact locator
#       string of every Evidence record cited by >=1 claim.
#
# NOT checked (by design — the honesty boundary of this script):
#   semantic truth of claims; whether source families are ACTUALLY
#   independent (family labels are trusted as recorded); whether a read
#   genuinely supports a claim (excerpt-vs-claim semantics); freshness;
#   budgets; stop-reason semantics — a PASS never implies every secondary
#   question was resolved. PASS certifies structure, not substance.
#
# No network. No writes. Reads exactly one packet path. Expects the
# canonical skeleton table shapes (references/run-packet.md); a deviation
# is REPORTED as a finding, never silently tolerated.

if [ $# -ne 1 ]; then
  echo "usage: sh validate-packet.sh <run-packet.md>" >&2
  exit 2
fi
if [ ! -f "$1" ]; then
  echo "validate-packet: cannot read packet: $1" >&2
  exit 2
fi
awk '
function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
# exact-marker segment match: m must equal the WHOLE trimmed value of s or
# one "—"/";"-delimited segment of it — a substring inside a larger word or
# a negated phrase ("NOT <m>") must NOT match.
function seg_match(s, m,   t, seg, i, k) {
  t = s; gsub(/—/, ";", t)
  k = split(t, seg, ";")
  for (i = 1; i <= k; i++) if (trim(seg[i]) == m) return 1
  return 0
}
function bad(msg) { printf "  - %s\n", msg; nf++ }
function nbs(s,   t, k) {          # number of trailing backslashes in s
  t = s; k = 0
  while (sub(/\\$/, "", t)) k++
  return k
}
# split a table line into cells on UNESCAPED |; a \| pair stays one literal |
function cells(line, f,   n, raw, i, j) {
  split("", f)
  n = split(line, raw, "|")
  j = 0
  for (i = 1; i <= n; i++) {
    if (j > 0 && nbs(f[j]) % 2 == 1)
      f[j] = substr(f[j], 1, length(f[j]) - 1) "|" raw[i]
    else { j++; f[j] = raw[i] }
  }
  return j
}
/^## / {
  h = tolower($0)
  inev = (h ~ /^## evidence records/)
  incl = (h ~ /^## claim classifications/)
  irep = (h ~ /^## final report/)
  if (inev) have_evs = 1
  if (incl) have_cls = 1
  if (irep) have_rep = 1
  tb = 0
  next
}
irep { rep = rep $0 "\n"; next }
# A table row missing its LEADING pipe (e.g. "A claim | partial | E999 |
# dangling reference |") is silently excluded by the leading-pipe row selector
# below — evidence-binding violations ride in the unselected row. Any
# non-blank line in these sections that CONTAINS a pipe but does not start
# with optional whitespace + "|" is not the valid leading-pipe table-row
# shape (header/delimiter/data), so it is REPORTED as a malformed row, never
# silently skipped. Lines without any "|" (prose, bullets) are not table
# rows and stay untouched.
(inev || incl) && /[^ \t]/ && /\|/ && !/^[ \t]*\|/ {
  bad((inev ? "evidence" : "claim") " table: malformed row without leading pipe — not the skeleton table-row shape, so it is excluded from all validation: \"" $0 "\"")
  next
}
(inev || incl) && /^[ \t]*\|/ && $0 ~ /[A-Za-z0-9]/ {
  tb++
  nc_ = cells($0, f)
  # t1b-F1: the outer boundary fields of the split (text before the first and
  # after the last unescaped pipe) must be EMPTY for every header and data
  # row of both table types. A dropped trailing pipe plus an extra column
  # yields the canonical field count, passes every cell check, and leaves
  # the extra column unvalidated — so a non-empty boundary is a violation.
  if (trim(f[1]) != "")
    bad((inev ? "evidence" : "claim") " table " (tb == 1 ? "header" : "row " (tb - 1)) ": non-empty field before the leading pipe (\"" trim(f[1]) "\") — deviates from skeleton shape")
  if (trim(f[nc_]) != "")
    bad((inev ? "evidence" : "claim") " table " (tb == 1 ? "header" : "row " (tb - 1)) ": non-empty field after the trailing pipe (\"" trim(f[nc_]) "\") — deviates from skeleton shape")
  if (tb == 1) {                                  # header row
    # F1: the header must match the canonical skeleton header EXACTLY —
    # same ordered cells, same width — before any data row is interpreted.
    # Substring presence is not enough: a reordered header would shift the
    # fixed cell positions read below and let a pseudo-ID land in notes.
    if (inev) {
      if (nc_ != 9 || trim(f[2]) != "id" || trim(f[3]) != "locator" \
          || trim(f[4]) != "excerpt" || trim(f[5]) != "read_depth" \
          || trim(f[6]) != "retrieved_at" || trim(f[7]) != "source_version" \
          || trim(f[8]) != "source_family") {
        bad("evidence table header is not exactly \"id | locator | excerpt | read_depth | retrieved_at | source_version | source_family\" (ordered cells, 7 columns) — deviates from skeleton shape")
        evhdr = 0
      } else evhdr = 1
    }
    if (incl) {
      if (nc_ != 6 || trim(f[2]) != "claim" || trim(f[3]) != "classification" \
          || trim(f[4]) != "evidence_ids" || trim(f[5]) != "notes") {
        bad("claim table header is not exactly \"claim | classification | evidence_ids | notes\" (ordered cells, 4 columns) — deviates from skeleton shape")
        clhdr = 0
      } else clhdr = 1
    }
    next
  }
  if (inev && evhdr) {                            # C1: evidence data row
    ne++
    id = trim(f[2]); loc = trim(f[3]); depth = trim(f[5]); fam = trim(f[8])
    if (nc_ != 9) bad("evidence row " ne ": " nc_ - 2 " columns, expected 7 — deviates from skeleton shape")
    if (id !~ /^E[0-9]+$/) bad("evidence row " ne ": id \"" id "\" does not match E<digits>")
    else if (id in ev) bad("evidence row " ne ": duplicate id " id)
    if (loc == "") bad("evidence row " ne ": empty locator")
    if (depth !~ /^(full|skim)( ?\(.*\))?$/) bad("evidence row " ne ": read_depth \"" depth "\" is not full|skim (paren annotation allowed)")
    if (fam == "") bad("evidence row " ne ": empty source_family")
    ev[id] = loc
    evd[id] = depth                               # retained for C3 full-depth check
  }
  if (incl && clhdr) {                            # C2+C3: claim data row
    nc++
    cls = trim(f[3]); cell = trim(f[4]); notes = trim(f[5])
    if (nc_ != 6) bad("claim row " nc ": " nc_ - 2 " columns, expected 4 — deviates from skeleton shape")
    # classification vocabulary: the documented 5-class set, case-sensitive
    # exact base word (paren annotation allowed, mirroring read_depth) —
    # case variants ("Supported") or unrecognized labels would otherwise
    # escape C2/C3 entirely (same vacuous-pass family).
    if (cls !~ /^(supported|partial|contradicted|unsupported|inference)( ?\(.*\))?$/)
      bad("claim row " nc ": classification \"" cls "\" is not one of supported|partial|contradicted|unsupported|inference (case-sensitive exact match, paren annotation allowed)")
    # F2: a parenthetical is ignored ONLY as an annotation immediately
    # following a valid E<digits> token ("E1 (spec doc)"). Standalone or
    # misplaced parenthesized content is reported, never silently stripped
    # (stripping everything let "(E6-derivative-stale)" pass with zero
    # tokens).
    work = cell; ecell = ""
    while (match(work, /\([^)]*\)/)) {
      pre = substr(work, 1, RSTART - 1)
      p = pre
      sub(/[ \t]+$/, "", p)
      if (p !~ /(^|[^A-Za-z0-9])E[0-9]+$/)
        bad("claim row " nc ": standalone or misplaced parenthetical in evidence column (only an annotation directly after a valid E<digits> id is ignored): \"" substr(work, RSTART, RLENGTH) "\"")
      ecell = ecell pre
      work = substr(work, RSTART + RLENGTH)
    }
    ecell = ecell work
    gsub(/[,+;&]/, " ", ecell)                    # separators -> spaces
    m = split(ecell, tk, /[ \t]+/)
    d = 0; dtid = ""
    for (i = 1; i <= m; i++) {
      t = tk[i]
      if (t == "" || t !~ /[A-Za-z0-9]/) continue # gap markers like —
      if (t !~ /^E[0-9]+$/) {
        bad("claim row " nc ": evidence token \"" t "\" is not a valid E<digits> Evidence-record id")
        continue
      }
      if (!(t in ev)) {
        bad("claim row " nc ": evidence id " t " is not declared in Evidence records")
        continue
      }
      if (!((nc SUBSEP t) in dt)) { dt[nc SUBSEP t] = 1; d++; dtid = t }
      cited[t] = 1
    }
    # a supported/partial claim with ZERO valid evidence tokens is a
    # vacuous pass (empty cell / gap markers only) — a violation.
    if ((cls ~ /^supported([^A-Za-z]|$)/ || cls ~ /^partial([^A-Za-z]|$)/) && d == 0)
      bad("claim row " nc ": classification \"" cls "\" cites zero valid Evidence-record ids (evidence column empty or gap markers only)")
    # F3: the direct-support basis must be the FULL literal note (a bare
    # "direct support" substring matched negations and shortenings), and
    # the single cited record must have been read at full depth.
    if (cls ~ /^supported([^A-Za-z]|$)/ && d == 1) {
      if (!seg_match(notes, "direct support (authoritative spec, single family)"))
        bad("claim row " nc ": supported on a single evidence record must carry the exact basis note \"direct support (authoritative spec, single family)\" in notes")
      if (substr(evd[dtid], 1, 4) != "full")
        bad("claim row " nc ": direct support requires the single cited record " dtid " to be read at full depth (read_depth \"" evd[dtid] "\" does not start with \"full\")")
    }
  }
}
END {
  if (!have_evs) bad("missing required section \"## Evidence records\"")
  if (!have_cls) bad("missing required section \"## Claim classifications\"")
  if (!have_rep) bad("missing required section \"## Final report\" (cited-locator containment not checked)")
  else for (id in cited) {
    loc = ev[id]
    if (index(rep, loc) == 0) bad("final report is missing the exact locator of " id ": " loc)
  }
  if (nf > 0) { print "validate-packet: FAIL (" nf " violation(s))"; exit 1 }
  print "validate-packet: PASS"
  exit 0
}
' "$1"
