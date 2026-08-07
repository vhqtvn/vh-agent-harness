// JS-layer shell-guard regression test for the `git-mutation-bypass` rule's
// GIT_MUTATION_RE trailing boundary.
//
// CRUX (perm-allowlist feature, blocked by commit-gate): the regex previously
// ended in `\b`. Because `-` is non-`\w`, `\b` fires at the `merge`→`-`
// boundary, so `git merge-base HEAD HEAD` matched `git merge` + `\b` and was
// DENIED by shell-guard — killing the read-only `git merge-base` allowlist
// entry (tables.go git_readonly) dead-on-arrival. `git rev-list` was unaffected
// (no mutation verb prefixes it).
//
// The fix changed the trailing boundary to a hyphen-aware negative lookahead
// `(?![\w-])`. This test locks BOTH directions of that boundary against the REAL
// rule object (the `git-mutation-bypass` entry of FORBIDDEN_PATTERNS), using the
// SAME deny decision shell-guard applies (`re.test(cmd) && !(allowIf &&
// allowIf.test(cmd))`). It imports the TEMPLATE source-of-truth (matching the
// check-defer-triggers.test.js convention) so a stale rendered .opencode/ copy
// can never shadow the template under test.
//
// Run:  vh-agent-harness exec node --test tests/scripts/forbidden-patterns-git-mutation.test.js
//       (or: node --test tests/scripts/forbidden-patterns-git-mutation.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const CORE = join(__dirname, "..", "..", "templates", "core", ".opencode", "repo-configs", "forbidden-patterns.core.js");

const mod = await import(pathToFileURL(CORE).href);
const RULE = mod.FORBIDDEN_PATTERNS.find((r) => r.id === "git-mutation-bypass");
assert.ok(RULE, "git-mutation-bypass rule must exist in FORBIDDEN_PATTERNS");
assert.ok(RULE.re instanceof RegExp, "rule.re must be a RegExp");
assert.ok(RULE.allowIf instanceof RegExp, "rule.allowIf must be a RegExp");

// Replicates shell-guard's denyByForbiddenPatterns decision exactly:
//   if re.test(cmd) AND NOT (allowIf && allowIf.test(cmd)) -> DENY.
function isDenied(cmd) {
    if (!RULE.re.test(cmd)) return false;
    if (RULE.allowIf && RULE.allowIf.test(cmd)) return false;
    return true;
}

// --- ALLOW: read-only verbs that must NOT be flagged as git mutations ---------
//
// These are the allowlist entries the perm-allowlist feature added. They MUST
// pass shell-guard's mutation backstop. `git merge-base` is the regression that
// was broken by the `\b` boundary; the rest are sibling read-only verbs.
test("git-mutation-bypass does NOT deny read-only merge-base / rev-list forms", () => {
    const allow = [
        "git merge-base HEAD HEAD",
        "git merge-base --is-ancestor A B",
        "git merge-base origin/main HEAD",
        "git rev-list HEAD",
        "git rev-list --count A..B",
        "git rev-list HEAD ^origin/main",
        // sibling read-only verbs (never mutation verbs) — sanity
        "git log",
        "git diff",
        "git show HEAD",
    ];
    for (const cmd of allow) {
        assert.equal(isDenied(cmd), false, `should NOT be denied: ${cmd}`);
    }
});

// --- DENY: bare mutation verbs + separator-followed mutations ----------------
//
// Every GitMutationVerb must STILL be denied. The separator-followed cases
// (`git merge;echo done`, `git merge&&git push`, `git merge|cat`) are the hole
// that `(?=\s|$)` would have opened; they lock that the chosen boundary is the
// hyphen-aware `(?![\w-])`, not the whitespace/EOL lookahead.
test("git-mutation-bypass STILL denies every bare mutation verb + separators", () => {
    const deny = [
        "git merge feature",
        "git merge",            // bare verb at EOL
        "git merge;echo done",  // statement separator — must stay denied
        "git merge&&git push",  // && separator
        "git merge|cat",        // pipe separator
        "git tag v1",
        "git reset --hard",
        "git checkout feature",
        "git commit -m x",
        "git push origin main",
        "git add -A",
        "git rebase main",
        "git stash",
        "git branch -d x",
        "git restore x",
        "git revert HEAD",
        "git clean -fd",
        "git rm x",
        "git mv a b",
        "git am x.patch",
        "git apply x.patch",
        "git switch feature",
    ];
    for (const cmd of deny) {
        assert.equal(isDenied(cmd), true, `should be DENIED: ${cmd}`);
    }
});

// --- DENY: hyphenated MUTATION verbs stay denied ------------------------------
//
// The hyphen-aware boundary must NOT over-correct: hyphenated verbs that ARE in
// GitMutationVerbs (commit-tree, cherry-pick, update-ref) are STILL denied,
// because the alternation matches the full hyphenated token first and then the
// boundary sees the following whitespace. This is the dual of the merge-base
// allow case (merge-base is NOT a mutation verb, so it is allowed).
test("git-mutation-bypass STILL denies hyphenated mutation verbs (commit-tree / cherry-pick / update-ref)", () => {
    const deny = [
        "git commit-tree HEAD^{tree}",
        "git cherry-pick abc123",
        "git update-ref refs/heads/x y",
    ];
    for (const cmd of deny) {
        assert.equal(isDenied(cmd), true, `hyphenated mutation verb should be DENIED: ${cmd}`);
    }
});

// --- DENY: hyphenated mutating PLUMBING sharing a mutation-verb prefix -------
//
// CRUX (F1 regression lock): switching the trailing boundary from `\b` to the
// hyphen-aware `(?![\w-])` (to fix the merge-base false-deny) ALSO removed the
// INCIDENTAL denial of hyphenated mutating plumbing whose first token is a
// mutation verb (e.g. `git merge-file`, `git checkout-index`). The old `\b`
// fired at the `verb`→`-` boundary and denied them; the new boundary refuses to
// match a verb followed by `-`, so `git merge` no longer catches `git
// merge-file`. The fix is to list every such hyphenated mutating plumbing as a
// FULL token in GitMutationVerbs (tables.go). This test locks that EACH one is
// denied, so the boundary change can never re-open a plumbing hole.
//
// Enumerated via a `git --exec-path` binary sweep + `git help -a`. Read-only
// `git merge-base` is deliberately NOT in this deny set (it is allowlisted in
// git_readonly and asserted ALLOW above).
test("git-mutation-bypass denies hyphenated mutating plumbing sharing a mutation-verb prefix", () => {
    const deny = [
        // checkout prefix — index→working-tree writes
        "git checkout-index --all",
        "git checkout-index -f -a",
        "git checkout--worker --prefix=.", // parallel-checkout worker
        // commit prefix — writes commit-graph
        "git commit-graph write",
        // merge prefix (excl. read-only merge-base) — merge-strategy helpers / file merge
        "git merge-file base ours theirs",
        "git merge-index",
        "git merge-octopus br1 br2 br3",
        "git merge-one-file",
        "git merge-ours br",
        "git merge-recursive base -- head remote",
        "git merge-resolve base -- head remote",
        "git merge-subtree base -- head remote",
        "git merge-tree --write-tree A B", // --write-tree writes tree objects
        // add prefix — interactive staging backend (git add -i/-p)
        "git add--interactive",
        // update prefix (separate from update-ref) — index entry mutation
        "git update-index --add x",
        "git update-index --cacheinfo 100644 sha path",
    ];
    for (const cmd of deny) {
        assert.equal(isDenied(cmd), true, `hyphenated mutating plumbing should be DENIED: ${cmd}`);
    }
});

// --- ALLOW (regression): the hyphenated plumbing fix must NOT re-deny ---------
// read-only merge-base. Locks that adding `merge-*` tokens to GitMutationVerbs
// did not accidentally include `merge-base`.
test("git-mutation-bypass STILL allows read-only merge-base after the plumbing sweep", () => {
    const allow = [
        "git merge-base HEAD HEAD",
        "git merge-base --is-ancestor A B",
        "git --no-pager merge-base HEAD HEAD",
    ];
    for (const cmd of allow) {
        assert.equal(isDenied(cmd), false, `read-only merge-base must stay ALLOWED: ${cmd}`);
    }
});
