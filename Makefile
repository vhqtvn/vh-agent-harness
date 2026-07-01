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

## Run the full test suite in the dev container.
test:
	vh-agent-harness exec pytest

## Run unit tests only.
test-unit:
	vh-agent-harness exec pytest tests/unit/

## Lint.
lint:
	vh-agent-harness exec ruff check .

## Format.
fmt:
	vh-agent-harness exec ruff format .

## Run DB migrations.
## DB creds come from project.config.json (project.db_user / project.db_name).
## The actual migrate command is project-specific; the runtime container exposes it
## via DEVENV_MIGRATE_CMD (empty = skip). Set this in your .env or compose override.
migrate:
	vh-agent-harness exec bash -c 'if [ -n "$${DEVENV_MIGRATE_CMD:-}" ]; then $$DEVENV_MIGRATE_CMD; else echo "DEVENV_MIGRATE_CMD not set; skipping migrations"; fi'

## Run the project smoke check.
## `smoke` is a custom verb the consumer declares in
## .vh-agent-harness/run-shape.yml under verbs: (e.g. a hook leaf). The harness
## core ships no project-specific smoke runner; declare your own.
smoke:
	vh-agent-harness smoke

## Remove build artifacts and repo-scoped tmp/.
clean:
	vh-agent-harness exec bash -c 'rm -rf tmp/*.log tmp/*.tmp || true'
