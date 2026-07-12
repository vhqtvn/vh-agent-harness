# vh-agent-harness developer tasks. The repo dogfoods its own harness; `update`
# regenerates this repo's rendered .opencode/ from templates/core after a build.
.PHONY: build test fmt vet check install update doctor test-auto-gate-live test-e2e-auto-gate test-e2e-auto-gate-opencode

# Version: bare tag on an exact-tag commit (release); <latest-tag>+dev otherwise.
# Semver build metadata (+dev) sorts equal to the tag, not below — honest "dev build
# on top of <tag>" without claiming an undecided next version. Fallback "dev" when
# built without ldflags (e.g. `go run`).
VERSION ?= $(shell \
  exact=$$(git describe --tags --exact-match 2>/dev/null); \
  if [ -n "$$exact" ]; then \
    echo "$$exact" | sed 's/^v//'; \
  else \
    latest=$$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'); \
    if [ -n "$$latest" ]; then echo "$$latest+dev"; \
    else echo "0.0.0+dev"; fi; \
  fi)

build: ## Build the binary into bin/
	go build -ldflags "-X github.com/vhqtvn/vh-agent-harness/internal/cli.Version=$(VERSION)" -o bin/vh-agent-harness ./cmd/vh-agent-harness

test: ## Run the full test suite
	go test ./...

fmt: ## Format all Go sources
	gofmt -w .

vet: ## Static analysis
	go vet ./...

check: fmt vet test ## fmt + vet + test (pre-commit gate)

install: ## Install the binary into GOBIN
	go install ./cmd/vh-agent-harness

update: build ## Dogfood: refresh this repo's rendered harness from templates/core
	./bin/vh-agent-harness update

doctor: build ## Verify this repo's harness install
	./bin/vh-agent-harness doctor

test-auto-gate-live: ## Run auto-gate live HTTP integration tests (requires Docker Compose; fully isolated)
	@command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 || \
		{ echo "[test-auto-gate-live] Docker Compose is not available; install Docker to run this suite."; exit 1; }
	docker compose -f tests/integration/auto-gate-live-http/docker-compose.yml run --rm tester

test-e2e-auto-gate: ## Run auto-gate-classifier plugin e2e (requires Docker Compose; fully isolated)
	@command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 || \
		{ echo "[test-e2e-auto-gate] Docker Compose is not available; install Docker to run this suite."; exit 1; }
	docker compose -f tests/e2e/auto-gate-classifier/docker-compose.yml run --rm e2e-runner

test-e2e-auto-gate-opencode: ## Run auto-gate real-runtime e2e (requires Docker; single container, fully isolated)
	@command -v docker >/dev/null 2>&1 || \
		{ echo "[test-e2e-auto-gate-opencode] Docker is not available; install Docker to run this suite."; exit 1; }
	docker build -t auto-gate-opencode-e2e -f tests/e2e/auto-gate-opencode/Dockerfile .
	docker run --rm auto-gate-opencode-e2e
