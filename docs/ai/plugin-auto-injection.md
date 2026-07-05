# `@opencode-ai/plugin` auto-injection and the `plan-state` tool

This note records a load-bearing interaction between the harness template and
the opencode runtime that is easy to misread: `tools/plan-state.js` has a live
**runtime** dependency on `@opencode-ai/plugin`, yet `.opencode/package.json`
intentionally does **not** pin it. The package is satisfied by opencode's
runtime auto-injection, not by the template's own `node_modules`.

> **Do not strip `@opencode-ai/plugin` references on the assumption it is
> unused.** `plan-state.js` imports it at runtime; removing or breaking that
> import disables the `plan-state` tool wired into the coordination agent.

This was authored as decision **B3** of the C→A→B maintainer fix (agreed with
consumer TrueAI): document the interaction rather than pin the package.

## The runtime dependency

`templates/core/.opencode/tools/plan-state.js` opens with:

```js
import { tool } from "@opencode-ai/plugin";
```

That `tool` is the live opencode tool-builder used to define the `planStateTool`
export (the `plan-state_planStateTool` surface wired into the coordination
agent). The same import also provides `tool.schema`, used throughout the file to
type every argument (`operation`, `session_name`, `slug`, `body`, …). This is a
real runtime import — not a type-only reference, not a cosmetic include.

## Why the template does not pin `@opencode-ai/plugin`

`.opencode/package.json` pins only the shell-guard engine's WASM deps:

```json
"dependencies": {
  "tree-sitter-bash": "0.25.0",
  "web-tree-sitter": "0.25.10"
}
```

`@opencode-ai/plugin` is absent on purpose. The opencode runtime auto-injects
`@opencode-ai/plugin@<version>` into its cache `node_modules` at startup
(visible in opencode logs as `opencode add @opencode-ai/plugin@... --exact`).
That auto-injected copy already resolves the `plan-state.js` import at runtime.

Pinning `@opencode-ai/plugin` in the template `package.json` would be redundant
(the runtime already provides it) and could version-conflict with whatever
opencode injects — two resolutions of the same package is exactly the failure
mode npm hoisting exists to avoid. The template pins only the deps opencode does
not provide itself.

## What `@opencode-ai/plugin` actually is

A brief correction, because a "type-only / cosmetic / unused at runtime"
characterization circulated and was retracted: `@opencode-ai/plugin` is a
**runtime ESM package**, not a types-only package. Every `exports` subpath pairs
a `types` entry (`.d.ts`) with a real `import` entry (`./dist/*.js`); it ships
~49KB of JS unpacked and pulls four runtime dependencies (`zod`, `effect`,
`@ai-sdk/provider`, `@opencode-ai/sdk`). The `plan-state.js` import above is one
of its intended runtime consumers.

## Maintenance warning (load-bearing)

Anyone touching `.opencode/package.json` or `tools/plan-state.js` must preserve
this dependency-resolution path:

1. **Do not remove the `import { tool } from "@opencode-ai/plugin";` line** in
   `plan-state.js`. It is the tool-builder for the entire `planStateTool`
   surface; stripping it disables the plan-state tool the coordination agent
   depends on.
2. **Do not add `@opencode-ai/plugin` to `.opencode/package.json`** to "fix" the
   apparent missing dep. The runtime auto-injection is the intended resolution;
   a template pin risks a version-conflict with the injected copy.
3. **If opencode stops auto-injecting the package** (upstream behavior change),
   the symptom is a runtime import failure in `plan-state.js`, not a build
   error — the template has no build step for this file. At that point the fix
   is to pin the package here, coordinated with the opencode-injected version.

The verification path for "is this still true?" is: read `plan-state.js` line 1
(the import), read `.opencode/package.json` (confirm `@opencode-ai/plugin` is
absent), and confirm opencode still logs `opencode add @opencode-ai/plugin@…
--exact` at startup.

## Related known doc seam (out of scope)

The committer's scoped edit-permission is emitted by `internal/permconfig/` as
the object form `{"*":"deny","tmp/commit-gate-message/**":"allow"}`, but
`templates/core/opencode.jsonc.tmpl` still carries a flat `"edit":"deny"` literal
that disagrees with the emitter. That template-vs-emitter seam is tracked
separately (noted in the `stage-message` docstring in
`templates/core/.opencode/scripts/commit-gate.sh`, commit `8875f39`) and is out
of scope for this note — mentioned only because both are "template literal
disagrees with the real runtime behavior" seams that a maintainer may encounter
together.
