---
name: web-dev-loop
description: Run browser-assisted frontend work in this repo through the Compose-managed Playwright lane. Use this when the user asks to test or debug the Next.js frontend with browser automation, capture traces/screenshots, choose between fixture-backed vs live-web checks, or drive the frontend loop without adding a separate browser stack.
compatibility: opencode
---

# Web Dev Loop

Use this for browser-assisted frontend work in {{PROJECT_NAME}}.

## When to use

- run Playwright smoke tests for `apps/web`
- choose between fixture-backed UI checks and live `web` service checks
- capture traces, screenshots, videos, and HTML reports under repo-scoped `tmp/`
- keep browser work inside `docker-compose.dev.yml` rather than adding host-only tooling

## When not to use

- do not use this for backend-only tasks
- do not use this for production browser infrastructure decisions
- do not use this as a substitute for deterministic backend evaluation or dataset validation

## Default workflow

1. Start from an OpenCode session alias when the work is non-trivial.
2. Prefer the fixture-backed smoke lane first:
   - `./dev.sh docker-compose-dev run --rm --profile frontend-tools playwright npm run test:e2e`
3. If the bug only reproduces against the live app stack, start the relevant services and reuse the running `web` server:
   - `./dev.sh docker-compose-dev up -d api web`
   - `./dev.sh docker-compose-dev run --rm --profile frontend-tools -e PLAYWRIGHT_USE_EXISTING_WEB_SERVER=1 -e PLAYWRIGHT_BASE_URL=http://web:3000 -e {{PROJECT_SLUG}}_WEB_USE_FIXTURES=0 playwright npm run test:e2e`
4. Use headed or UI mode only when a trace is not enough:
   - `./dev.sh docker-compose-dev run --rm --profile frontend-tools playwright npm run test:e2e:headed`
   - `./dev.sh docker-compose-dev run --rm --service-ports --profile frontend-tools playwright npm run test:e2e:ui`
5. For manual local viewing, start the noVNC-based browser viewer:
   - `./dev.sh web-view up`
   - `./dev.sh web-view url`
   - `./dev.sh web-view down`
6. Keep artifacts under `tmp/agent-runs/...`; the default location is `tmp/agent-runs/playwright/`.

## Notes

- The browser viewer is local-only by default and binds to `127.0.0.1`.
- If `BROWSER_VIEW_EXPOSE_CMD` is set, the viewer container will run that command with the internal web port appended so you can layer your own later publish/tunnel mechanism on top.
- The `playwright` service is the browser tool lane. The `web` service is still the interactive Next.js dev server.
- When the task is about actual app behavior, name whether you are using fixture mode or the live web stack in the task contract or checkpoint.

## Output

- exact command used
- whether the run used fixtures or the live web stack
- artifact location under `tmp/agent-runs/...`
- smallest next step: fix, add regression test, or switch to the live stack
