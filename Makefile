# vh-agent-harness developer tasks. The repo dogfoods its own harness; `update`
# regenerates this repo's rendered .opencode/ from templates/core after a build.
.PHONY: build test fmt vet check install update doctor

build: ## Build the binary into bin/
	go build -o bin/vh-agent-harness ./cmd/vh-agent-harness

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
