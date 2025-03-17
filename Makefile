# Makefile for dspy-go

# Go parameters
GOCMD=go
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=$(GOCMD) fmt
GOLINT=golangci-lint
BINARY_NAME=dspy-go
BINARY_UNIX=$(BINARY_NAME)_unix
GOFILES=$(shell find . -type f -name "*.go" -not -path "./vendor/*" -not -path "./examples/*")
TIMEOUT=5m
PKGS=$(shell $(GOCMD) list ./... | grep -v '/examples/')

# Define the default target
.PHONY: all
all: test lint

# Build the application
.PHONY: build
build:
	$(GOCMD) build -o $(BINARY_NAME) -v

# Clean up binary files and test artifacts
.PHONY: clean
clean:
	$(GOCMD) clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out

# Run all tests (excluding examples)
.PHONY: test
test:
	$(GOTEST) $(PKGS) -v

# Run tests with Redis build tag
.PHONY: test-tag-redis
test-tag-redis:
	$(GOTEST) -tags=redis $(PKGS) -v

# Run only Redis tests
.PHONY: test-only-redis
test-only-redis:
	$(GOTEST) -tags=redis -run=TestRedis ./pkg/agents/memory/... -v

# Run Redis tests with Docker
.PHONY: test-redis
test-redis:
	@echo "Starting Redis container for tests..."
	@docker run -d --name redis-test -p 6379:6379 redis:latest > /dev/null
	@echo "Running tests with Redis..."
	@REDIS_TEST_ADDR=localhost:6379 $(GOTEST) -tags=redis $(PKGS) -v
	@echo "Cleaning up Redis container..."
	@docker stop redis-test > /dev/null && docker rm redis-test > /dev/null
	@echo "Redis tests completed."

# Run integration tests that require external services
# These tests are skipped unless environment variables are set
.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	@if [ -z "$(REDIS_TEST_ADDR)" ]; then \
		echo "REDIS_TEST_ADDR not set, Redis tests will be skipped"; \
	else \
		echo "Using Redis at: $(REDIS_TEST_ADDR)"; \
	fi
	$(GOTEST) -tags=redis $(PKGS) -v

# Run tests with a custom timeout
.PHONY: test-timeout
test-timeout:
	$(GOTEST) -timeout $(TIMEOUT) $(PKGS) -v

# Run tests for a specific package
# Usage: make test-pkg PKG=./pkg/llms
.PHONY: test-pkg
test-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-pkg PKG=./pkg/llms"; \
		exit 1; \
	fi
	$(GOTEST) -v $(PKG)

# Create test file for a specific Go file
# Usage: make create-test FILE=pkg/llms/ollama.go
.PHONY: create-test
create-test:
	@if [ -z "$(FILE)" ]; then \
		echo "Usage: make create-test FILE=pkg/llms/ollama.go"; \
		exit 1; \
	fi
	@TESTFILE=$$(echo "$(FILE)" | sed 's/\.go/_test.go/'); \
	if [ -f "$$TESTFILE" ]; then \
		echo "Test file $$TESTFILE already exists"; \
	else \
		PKG=$$(grep "^package" "$(FILE)" | head -1 | cut -d' ' -f2); \
		echo "Creating test file $$TESTFILE for package $$PKG"; \
		echo "package $$PKG" > "$$TESTFILE"; \
		echo "" >> "$$TESTFILE"; \
		echo "import (" >> "$$TESTFILE"; \
		echo "	\"testing\"" >> "$$TESTFILE"; \
		echo "" >> "$$TESTFILE"; \
		echo "	\"github.com/stretchr/testify/assert\"" >> "$$TESTFILE"; \
		echo ")" >> "$$TESTFILE"; \
		echo "" >> "$$TESTFILE"; \
		echo "// TODO: Implement tests for $(FILE)" >> "$$TESTFILE"; \
		echo "Test file created at $$TESTFILE"; \
	fi

# Run tests with coverage
.PHONY: test-cover
test-cover:
	$(GOTEST) -coverprofile=coverage.out $(PKGS)
	$(GOCMD) tool cover -func=coverage.out

# Generate HTML coverage report
.PHONY: cover-html
cover-html: test-cover
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run race detector
.PHONY: test-race
test-race:
	$(GOTEST) -race $(PKGS)

# Lint the code with golangci-lint
.PHONY: lint
lint:
	$(GOLINT) run --skip-dirs examples ./...

# Fix linting issues that can be automatically fixed
.PHONY: lint-fix
lint-fix:
	$(GOLINT) run --fix --skip-dirs examples ./...

# Add nolint directives as a LAST RESORT for fixing linting errors
# Usage: make nolint FILE=path/to/file.go LINTER=errcheck LINE=123
.PHONY: nolint
nolint:
	@if [ -z "$(FILE)" ] || [ -z "$(LINTER)" ] || [ -z "$(LINE)" ]; then \
		echo "Usage: make nolint FILE=path/to/file.go LINTER=errcheck LINE=123"; \
		echo "Example: make nolint FILE=pkg/llms/ollama.go LINTER=errcheck LINE=42"; \
		exit 1; \
	fi
	@echo "Adding nolint directive to $(FILE) at line $(LINE) for $(LINTER)"
	@sed -i "$(LINE)s/$$/\t\/\/ nolint: $(LINTER)/" $(FILE)
	@echo "Added nolint directive. Please verify the change."

# Format code
.PHONY: fmt
fmt:
	$(GOFMT) ./pkg/... ./internal/...

# Vet code
.PHONY: vet
vet:
	$(GOVET) $(PKGS)

# Run tests and linting in CI environment
.PHONY: ci
ci: test-cover lint

# Install dependencies
.PHONY: deps
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy

# Install development tools
.PHONY: tools
tools:
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run Redis agent example using Docker
.PHONY: redis-start
redis-start:
	@echo "Starting Redis container for example..."
	@docker inspect redis-example >/dev/null 2>&1 || \
	  docker run -d --name redis-example -p 6379:6379 redis:latest > /dev/null && \
	  echo "Redis container started."

# Run Redis agent example
.PHONY: run-redis-example
run-redis-example:
	@echo "Running Redis agent example..."
	@cd examples/agent_redis && go run main.go --redis-addr=localhost:6379

# Run Redis agent example using Docker (combined target)
.PHONY: redis-example
redis-example: redis-start run-redis-example

# Stop Redis example container
.PHONY: redis-stop
redis-stop:
	@echo "Stopping Redis container..."
	@docker stop redis-example >/dev/null 2>&1 && docker rm redis-example >/dev/null 2>&1 || true
	@echo "Redis container stopped and removed."

# Run all examples
.PHONY: examples
examples: redis-example

# Run AsyncChainWorkflow tests with Redis
.PHONY: test-async-workflow
test-async-workflow:
	@echo "Running AsyncChainWorkflow tests with Redis..."
	@docker inspect redis-test >/dev/null 2>&1 || \
	  docker run -d --name redis-test -p 6379:6379 redis:latest > /dev/null
	@sleep 2
	@REDIS_TEST_ADDR=localhost:6379 go test -tags=redis -v ./pkg/agents/workflows -run TestAsyncChainWorkflow_Redis
	@echo "Cleaning up Redis container..."
	@docker stop redis-test >/dev/null 2>&1 && docker rm redis-test >/dev/null 2>&1 || true
	@echo "AsyncChainWorkflow tests completed."

# Run Faktory queue tests
.PHONY: test-faktory-queue
test-faktory-queue:
	@echo "Running Faktory queue tests..."
	@docker inspect faktory-test >/dev/null 2>&1 || \
	  docker run -d --name faktory-test -p 7419:7419 -p 7420:7420 contribsys/faktory:latest > /dev/null
	@sleep 2
	@FAKTORY_URL=localhost:7419 go test -tags=faktory -v ./pkg/agents/workflows -run TestFaktoryQueue
	@echo "Cleaning up Faktory container..."
	@docker stop faktory-test >/dev/null 2>&1 && docker rm faktory-test >/dev/null 2>&1 || true
	@echo "Faktory queue tests completed."

# Run Redis queue tests
.PHONY: test-redis-queue
test-redis-queue:
	@echo "Running Redis queue tests..."
	@docker inspect redis-test >/dev/null 2>&1 || \
	  docker run -d --name redis-test -p 6379:6379 redis:latest > /dev/null
	@sleep 2
	@REDIS_TEST_ADDR=localhost:6379 go test -tags=redis -v ./pkg/agents/workflows -run TestRedisQueue
	@echo "Cleaning up Redis container..."
	@docker stop redis-test >/dev/null 2>&1 && docker rm redis-test >/dev/null 2>&1 || true
	@echo "Redis queue tests completed."

# Run all queue tests
.PHONY: test-queues
test-queues: test-redis-queue test-faktory-queue
	@echo "All queue tests completed successfully."

.PHONY: test-distributed-worker
test-distributed-worker:
	go test -count=1 -v -tags=distribworker ./pkg/agents/workflows

.PHONY: test-all
test-all: test test-redis-queue test-faktory-queue test-distributed-worker

.PHONY: help
help:
	@echo "Make targets:"
	@echo "  all         - Run tests and linting"
	@echo "  build       - Build the application"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run all tests (excluding examples)"
	@echo "  test-tag-redis - Run tests with Redis build tag"
	@echo "  test-only-redis - Run only Redis tests"
	@echo "  test-redis  - Run tests with Redis in Docker (automatically handles container setup/cleanup)"
	@echo "  test-integration - Run tests for all external integrations (respects env vars)"
	@echo "  test-timeout - Run tests with custom timeout (default: $(TIMEOUT))"
	@echo "  test-pkg    - Run tests for specific package (usage: make test-pkg PKG=./pkg/llms)"
	@echo "  create-test - Create test file for a Go file (usage: make create-test FILE=pkg/llms/ollama.go)"
	@echo "  test-cover  - Run tests with coverage"
	@echo "  cover-html  - Generate HTML coverage report"
	@echo "  test-race   - Run tests with race detector"
	@echo "  lint        - Run linter"
	@echo "  lint-fix    - Fix lint issues automatically"
	@echo "  nolint      - Add nolint directive (last resort, usage: make nolint FILE=path/to/file.go LINTER=errcheck LINE=123)"
	@echo "  fmt         - Format code"
	@echo "  vet         - Run go vet"
	@echo "  ci          - Run tests and linting (for CI)"
	@echo "  deps        - Download and tidy dependencies"
	@echo "  tools       - Install development tools"
	@echo "  redis-start - Start Redis container for examples"
	@echo "  run-redis-example - Run Redis agent example"
	@echo "  redis-example - Run Redis agent example using Docker (starts container automatically)"
	@echo "  redis-stop - Stop Redis example container"
	@echo "  examples - Run all examples"
	@echo "  test-async-workflow - Run AsyncChainWorkflow tests with Redis"
	@echo "  test-faktory-queue - Run Faktory queue tests"
	@echo "  test-redis-queue - Run Redis queue tests"
	@echo "  test-queues - Run all queue tests"
	@echo "  test-distributed-worker - Run distributed worker tests"
	@echo "  test-all - Run all tests" 