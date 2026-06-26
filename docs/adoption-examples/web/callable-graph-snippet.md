<!-- web-overlay pack (ownership: overlay_extension) -->
<!-- Appended to callable-graph.md when vh-harness-profile.yml: overlays:[web-overlay]. -->

## web-overlay specialists

- **browser-qa** (read-only): reproduces frontend issues, captures Playwright traces/screenshots, returns DOM-level reproduction details and the smallest stable regression test.
- **web-builder** (editable): implements focused `apps/web` slices plus browser-smoke wiring; hands off cross-app backend changes rather than drifting into them.

### web-overlay routing

- `build`, `coordination`, and `project-coordinator` may delegate to `browser-qa` and `web-builder` (task allow).
- `browser-qa` stays read-only and routes git writes through `committer`.
- `web-builder` may edit frontend files only; backend-contract changes are handed back.
- Commands surfaced by this pack: `frontend`, `web-smoke`, `web-stack-up`, `browser-view`.
