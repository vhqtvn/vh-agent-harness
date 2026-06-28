# Template authoring guide

This guide is for anyone editing the embedded corpus under `templates/core/`
(the source that ships into consumer repos) or authoring an overlay pack. It
captures conventions that are easy to get wrong because the renderer is
intentionally narrow.

## The core principle: unenforced conventions are footguns

> **A convention the renderer doesn't enforce is a footgun.** If the renderer
> does not implement a transformation, you cannot rely on it happening at render
> time — no matter what a comment in the template says.

The renderer (`internal/substrate/renderer.go`, `SubstituteHarnessTokens`) is an
**allowlist-tight token pass**. It does exactly one thing: substitute the known
sentinel tokens (`{{PROJECT_NAME}}`, `{{PROJECT_SLUG}}`, `{{COORDINATOR_DIR}}`,
`{{MISSION_SUMMARY}}`, `{{ARCHITECTURE_SUMMARY}}`, `{{DB_USER}}`,
`{{DB_NAME}}`) with resolved values. Concretely it does **not**:

- strip or transform comments (a `#`/`//` line ships verbatim to the consumer),
- normalize whitespace, casing, or line endings beyond token substitution,
- interpret prose instructions directed at it (e.g. `# strip this on render`),
- expand conditionals, loops, or includes.

### The trap this guards against

The original bug this slice hardens against: a consumer shipped a `CLAUDE.md`
that still contained literal `{{TOKEN}}`s **and** template-author comments like
`# (the renderer strips this line)`. There is no renderer feature that strips
comments, so the comment shipped literally into the consumer's repo. Comments
addressed to the renderer are **not honored** and will ship as-is.

### Authoring rules

1. **Do not leave instructions to the renderer in templates.** If you need a
   note for human template-authors, put it in this doc or a code comment in the
   Go side — never as a comment inside an embedded template that reads like a
   render directive (`strip … on render`, `remove … on render`,
   `the renderer will …`).
2. **Tokens must be in the allowlist.** An unknown `{{FOO}}` is left literally
   in the output. If you need a new token, add it to the renderer's allowlist
   AND to `projectConfigAnswers` (or the install identity answers) in the same
   change.
3. **`{{PROJECT_SLUG}}` is case-aware** — UPPER when followed by `_`
   (`{{PROJECT_SLUG}}_JWT_SECRET` → `MYPROJ_JWT_SECRET`), lower otherwise
   (`{{PROJECT_SLUG}}-dev-1`). Write env/secret names with `_`, paths/services
   with `-`.
4. **Empty tokens resolve to empty string** — not an error. An unset
   `{{MISSION_SUMMARY}}` produces a blank section, silently. (Slice 1 adds a
   CLI warning for this; the renderer itself stays silent.)

## Build-time enforcement

The principle above is enforced at build time by
`internal/substrate/templates_lint_test.go`, which scans every embedded
`templates/core/` template for known-bad unenforceable conventions (comment
lines that read like render directives) and **fails the build** if found. This
is deliberately a narrow guard against the specific trap — it is **not** a
second renderer and does no normalization. If you add a new class of
footgun, add a pattern there.
