// Focused unit tests for isProductSurface slash-correctness in
// templates/core/.opencode/scripts/coordination-hints-lib.js.
//
// CRUX (DEFER defer-product-prefixes-trailing-slash-overmatch): isProductSurface
// used a raw startsWith against the configured product prefixes. A slash-less
// entry such as "apps" would over-match any path beginning with those literal
// characters (e.g. "appsfoo/", "appsbar"), not just the intended product
// directory. Safe today only because DEFAULT_PRODUCT_PREFIXES = ["apps/",
// "packages/"] and the verify fixtures all carry a trailing slash; brittle
// against an adopter .vh-agent-harness/product-prefixes.json entry written
// without one. The fix normalizes each prefix to end with "/" before the
// startsWith check, making the match directory-bounded. These tests pin the
// fix directly against the source lib (the node --test gate), complementing
// the parseProductPrefixes + integration coverage in verify-coordination-hints.js.
//
// Run:  vh-agent-harness exec node --test tests/scripts/coordination-hints-lib-product-prefix.test.js
//       (or: node --test tests/scripts/coordination-hints-lib-product-prefix.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { isProductSurface, DEFAULT_PRODUCT_PREFIXES } from "../../templates/core/.opencode/scripts/coordination-hints-lib.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

test("isProductSurface: the shipped defaults stay unchanged (apps/ + packages/)", () => {
    assert.deepEqual(
        [...DEFAULT_PRODUCT_PREFIXES],
        ["apps/", "packages/"],
        "The monorepo default must stay apps/ + packages/ (the override path's fallback).",
    );
    // Defaults already carry a trailing slash, so the fix is a no-op for them.
    assert.equal(isProductSurface("apps/api/main.py"), true);
    assert.equal(isProductSurface("packages/core/index.ts"), true);
    assert.equal(isProductSurface("docs/coordination/README.md"), false);
});

test("isProductSurface CRUX: a slash-less prefix must NOT over-match a same-stem directory", () => {
    // The exact defect: an adopter entry "apps" (no trailing slash) must NOT
    // match "appsfoo/..." — a raw startsWith would. This is the load-bearing
    // assertion the fix exists to make pass.
    assert.equal(
        isProductSurface("appsfoo/handlers.ts", ["apps"]),
        false,
        "Slash-less prefix 'apps' must NOT match the unrelated directory 'appsfoo/'.",
    );
    assert.equal(
        isProductSurface("appsbar", ["apps"]),
        false,
        "Slash-less prefix 'apps' must NOT match a same-stem filename 'appsbar'.",
    );
    assert.equal(
        isProductSurface("applications/web/index.ts", ["apps"]),
        false,
        "Slash-less prefix 'apps' must NOT match 'applications/' (prefix-of-name, not a directory child).",
    );
});

test("isProductSurface: a slash-less prefix must still match genuine children", () => {
    // The fix must not over-correct: "apps" should still match a file under apps/.
    assert.equal(
        isProductSurface("apps/x", ["apps"]),
        true,
        "Slash-less prefix 'apps' must match its child 'apps/x'.",
    );
    assert.equal(
        isProductSurface("apps/api/src/main.py", ["apps"]),
        true,
        "Slash-less prefix 'apps' must match a nested child 'apps/api/src/main.py'.",
    );
});

test("isProductSurface: a trailing-slash prefix keeps identical behavior (slash and slash-less are equivalent)", () => {
    // Slash-less and trailing-slash forms of the SAME entry must agree on every
    // path — this is the parity invariant the normalization guarantees.
    const cases = [
        "apps/x",
        "apps/api/main.py",
        "appsfoo/handlers.ts",
        "appsbar",
        "applications/web",
        "packages/core/index.ts",
        "docs/coordination/README.md",
    ];
    for (const p of cases) {
        assert.equal(
            isProductSurface(p, ["apps"]),
            isProductSurface(p, ["apps/"]),
            `Slash-less and trailing-slash forms must agree on '${p}'.`,
        );
    }
});

test("isProductSurface: a nested slash-less prefix (e.g. 'apps/web') is bounded at its own directory", () => {
    // A multi-segment slash-less entry is bounded at ITS directory, not at the
    // first segment: 'apps/web' must match 'apps/web/x' but not 'apps/webfoo/y'.
    assert.equal(isProductSurface("apps/web/index.ts", ["apps/web"]), true);
    assert.equal(isProductSurface("apps/webfoo/index.ts", ["apps/web"]), false);
});

test("isProductSurface: defaults used when no prefix list is passed (regression guard)", () => {
    // The default-param path (productPrefixes = DEFAULT_PRODUCT_PREFIXES) must
    // keep the shipped behavior: apps/ + packages/ are product surfaces.
    assert.equal(isProductSurface("apps/api/main.py"), true);
    assert.equal(isProductSurface("packages/core/index.ts"), true);
    assert.equal(isProductSurface("infra/terraform/main.tf"), false);
});
