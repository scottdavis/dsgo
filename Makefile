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

.PHONY: help
help:
	@echo "Make targets:"
	@echo "  all         - Run tests and linting"
	@echo "  build       - Build the application"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run all tests (excluding examples)"
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