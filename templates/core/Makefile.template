# Makefile — thin convenience wrapper over vh-agent-harness (NOT the primary
# interface; vh-agent-harness is). Keep targets delegating to it. Project values
# (slug, db_user, db_name) come from .vh-agent-harness/project.config.json.

.PHONY: env dev-up dev-down dev-logs clean smoke test lint fmt migrate shell

## Print the runtime container environment.
env:
	@vh-agent-harness exec env

## Start the dev container stack.
dev-up:
	vh-agent-harness up

## Stop the dev container stack.
dev-down:
	vh-agent-harness down

## Tail dev container logs.
dev-logs:
	vh-agent-harness logs

## Open a shell in the dev container.
shell:
	vh-agent-harness shell

# --- Project runner targets ---------------------------------------------------
# WIRE-VS-RESPEC (card makefile-verbs-dispatch-gap, settled 2026-11-15): an
# earlier revision of the test/lint/fmt/smoke targets prescribed
# `vh-agent-harness test|lint|fmt|smoke` — verbs the CLI never dispatched
# (bare verbs hit unknown-command; the run-shape loader ignores the `verbs:`
# block). Wiring four new CLI verbs was REJECTED: CLI-surface expansion
# requires a bootstrap-inventory default-in/out ledger decision. These
# targets are RE-SPEC'd to the dispatched exec-family surface instead: your
# project runner, run through `vh-agent-harness exec`, executes inside the
# project runtime (host under host-shell, the container under
# docker-compose). The dispatched runtime verbs are: exec, exec-ro,
# exec-sandbox, shell, up, down, logs, ps.

## Run the project test suite.
## No harness `test` verb exists; swap the placeholder for your runner, e.g.
##   vh-agent-harness exec go test ./...     (Go)
##   vh-agent-harness exec npm test          (Node)
test:
	vh-agent-harness exec bash -c 'echo "test: no project test runner configured - edit the Makefile test target (vh-agent-harness exec <your-runner>)"; exit 1'

## Lint. Same contract as `test`: swap the placeholder for your linter, e.g.
##   vh-agent-harness exec go vet ./...
lint:
	vh-agent-harness exec bash -c 'echo "lint: no project linter configured - edit the Makefile lint target (vh-agent-harness exec <your-linter>)"; exit 1'

## Format. Same contract as `test`: swap the placeholder for your formatter,
## e.g.   vh-agent-harness exec gofmt -w .
fmt:
	vh-agent-harness exec bash -c 'echo "fmt: no project formatter configured - edit the Makefile fmt target (vh-agent-harness exec <your-formatter>)"; exit 1'

## Run DB migrations.
## DB creds come from project.config.json (project.db_user / project.db_name).
## The actual migrate command is project-specific; the runtime container exposes it
## via DEVENV_MIGRATE_CMD (empty = skip). Set this in your .env or compose override.
migrate:
	vh-agent-harness exec bash -c 'if [ -n "$${DEVENV_MIGRATE_CMD:-}" ]; then $$DEVENV_MIGRATE_CMD; else echo "DEVENV_MIGRATE_CMD not set; skipping migrations"; fi'

## Run the project smoke check.
## No harness `smoke` verb exists; same contract as `test` — swap the
## placeholder for your smoke runner, e.g.
##   vh-agent-harness exec make smoke-real
smoke:
	vh-agent-harness exec bash -c 'echo "smoke: no project smoke runner configured - edit the Makefile smoke target (vh-agent-harness exec <your-smoke-runner>)"; exit 1'

## Remove build artifacts and repo-scoped tmp/.
clean:
	vh-agent-harness exec bash -c 'rm -rf tmp/*.log tmp/*.tmp || true'
