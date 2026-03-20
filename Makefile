.PHONY: build test test-unit test-integration test-e2e test-e2e-netns test-e2e-claude install install-system uninstall-system clean lint fmt vet coverage help

# Build variables
BINARY_NAME=misbah
BUILD_DIR=bin
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

## build: Build all binaries
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/misbah
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-vsock-fwd ./cmd/misbah-vsock-fwd

## install: Install misbah to $GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(LDFLAGS) ./cmd/misbah

## install-system: Install binaries, systemd unit, and default config (run as root)
install-system: build
	@echo "Installing system-wide..."
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME)-vsock-fwd /usr/local/lib/misbah/$(BINARY_NAME)-vsock-fwd
	install -Dm644 assets/misbah-daemon.service /etc/systemd/system/misbah-daemon.service
	install -dm755 /etc/misbah
	@test -f /etc/misbah/daemon.yaml || install -Dm644 assets/daemon.yaml /etc/misbah/daemon.yaml
	@getent group misbah >/dev/null 2>&1 || groupadd misbah
	@echo "Adding $(SUDO_USER) to misbah group..." && usermod -aG misbah $(SUDO_USER) 2>/dev/null || true
	systemctl daemon-reload
	@echo ""
	@echo "Installed. To start: sudo systemctl start misbah-daemon"
	@echo "Re-login or use 'sg misbah' to activate group membership."

## uninstall-system: Remove binaries, systemd unit (run as root)
uninstall-system:
	@echo "Uninstalling..."
	systemctl stop misbah-daemon 2>/dev/null || true
	systemctl disable misbah-daemon 2>/dev/null || true
	rm -f /usr/local/bin/$(BINARY_NAME)
	rm -f /etc/systemd/system/misbah-daemon.service
	systemctl daemon-reload
	@echo "Uninstalled. Config at /etc/misbah/ preserved."

## setup-kata: Configure host for Kata backend (run as root)
setup-kata: build
	@sudo ./scripts/setup-kata.sh

## test: Run all tests
test: test-unit test-integration

## test-unit: Run unit tests
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v -race -cover ./...

## test-integration: Run integration tests (Linux only)
test-integration:
	@echo "Running integration tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping integration tests (requires Linux)"; \
	else \
		$(GO) test -v -tags=integration ./test/integration/...; \
	fi

## test-e2e: Run end-to-end tests (basic workflow)
test-e2e:
	@echo "Running E2E tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping E2E tests (requires Linux)"; \
	else \
		$(GO) build -o ./misbah ./cmd/misbah; \
		$(GO) test -v -tags=e2e ./test/e2e/...; \
	fi

## test-e2e-llm: Run E2E tests with LLM agent (requires Ollama + Qwen2.5-Coder)
test-e2e-llm:
	@echo "Running LLM-driven E2E tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping E2E tests (requires Linux)"; \
	else \
		$(GO) build -o ./misbah ./cmd/misbah; \
		$(GO) test -v -tags=e2e,llm ./test/e2e/...; \
	fi

## test-e2e-container: Run E2E tests in container (requires podman)
test-e2e-container:
	@echo "Running containerized E2E tests..."
	@if ! command -v podman >/dev/null 2>&1; then \
		echo "podman not found, install from https://podman.io"; \
		exit 1; \
	fi
	@$(GO) build -o ./misbah ./cmd/misbah
	@$(GO) test -v -tags=e2e -run TestContainerized ./test/e2e/...

## test-e2e-mcp: Run MCP-based E2E tests
test-e2e-mcp:
	@echo "Running MCP E2E tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping E2E tests (requires Linux)"; \
	else \
		$(GO) build -o ./misbah ./cmd/misbah; \
		$(GO) test -v -tags=e2e -run TestMCPWorkflow ./test/e2e/...; \
	fi

## test-e2e-llm-mcp: Run LLM + MCP E2E tests (requires Ollama + Qwen2.5-Coder)
test-e2e-llm-mcp:
	@echo "Running LLM + MCP E2E tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping E2E tests (requires Linux)"; \
	else \
		$(GO) build -o ./misbah ./cmd/misbah; \
		$(GO) test -v -tags=e2e,llm -run TestLLMWithMCP ./test/e2e/...; \
	fi

## test-e2e-netns: Run NetworkIsolator iptables integration tests (requires root)
test-e2e-netns:
	@echo "Running netns iptables integration tests..."
	sudo $(GO) test -v -tags='e2e netns' ./daemon/...

## test-e2e-claude: Run E2E tests with Claude Code (requires claude binary + MISBAH_E2E_CLAUDE=true)
test-e2e-claude:
	@echo "Running Claude Code E2E tests..."
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "Skipping E2E tests (requires Linux)"; \
	else \
		if ! command -v claude >/dev/null 2>&1; then \
			echo "claude binary not found in PATH"; \
			exit 1; \
		fi; \
		if [ "$$MISBAH_E2E_CLAUDE" != "true" ]; then \
			echo "Set MISBAH_E2E_CLAUDE=true to run Claude tests"; \
			exit 1; \
		fi; \
		$(GO) build -o ./misbah ./cmd/misbah; \
		$(GO) test -v -tags=e2e,claude ./test/e2e/...; \
	fi

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/"; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.txt coverage.html
	@$(GO) clean

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' Makefile | column -t -s ':' | sed -e 's/^/ /'
