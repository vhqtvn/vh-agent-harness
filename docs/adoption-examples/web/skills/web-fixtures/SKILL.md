---
name: web-fixtures
description: Use deterministic frontend fixture mode for the {{PROJECT_NAME}} web app when browser smoke tests or UI debugging should not depend on live API/database state. Use this when the user asks for stable frontend fixtures, fixture-backed Playwright runs, canned operator UI data, or reproducible browser checks while frontend code is still moving quickly.
compatibility: opencode
---

# Web Fixtures

Use this for deterministic frontend data in `apps/web`.

## When to use

- fixture-backed Playwright smoke tests
- UI work that should not depend on current API, DB, or dataset state
- stable repros for visual/layout/debug loops

## When not to use

- do not use this to fake completed backend integrations in product docs
- do not use this for permanent production seed data
- do not use this for backend contract decisions without checking the real API contract
- do not use this to decide whether to run fixture-backed or live — `web-dev-loop` owns that decision
- do not use this to drive browser automation; `web-dev-loop` owns the Playwright lane

## Workflow

1. Keep fixtures local to the web app and production-shaped.
2. Gate fixture mode through `{{PROJECT_SLUG}}_WEB_USE_FIXTURES=1`.
3. Preserve the real API path as the default.
4. Prefer one small operator-facing fixture set over many inconsistent samples.
5. If a real API contract changes, update the fixture shape so browser smoke still reflects the current contract.

## Current fixture entrypoints

- `apps/web/src/fixtures/operator-ui.json`
- `apps/web/src/lib/server-api.ts`

<!-- Update when fixture entrypoints are added, renamed, or removed -->

## Output

- which fixture entrypoint changed
- which browser smoke path it stabilizes
- whether live API mode was also checked
