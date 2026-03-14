# Misbah Testing Infrastructure

This directory contains all tests for misbah, following the same patterns as Scribe, Lex, and Locus MCP servers.

## Test Hierarchy

```
test/
├── e2e/               # End-to-end tests (Go-native, podman containers)
│   ├── e2e_test.go        # Basic E2E workflow tests
│   ├── mcp_test.go        # MCP-based E2E tests
│   ├── llm_mcp_test.go    # LLM-driven tests with Qwen2.5-Coder
│   └── claude_test.go     # Claude Code integration tests
├── integration/       # Integration tests (real Linux namespaces)
│   └── mount_test.go      # Namespace, lock, and mount tests
└── fixtures/          # Test fixtures
    ├── manifests/         # Sample manifests
    └── mock-repos/        # Mock repository structures
```

## Test Types

### Unit Tests (60%)
Located in each package (`*_test.go` files):
```bash
make test-unit
```

**Packages tested:**
- `model/` - Domain types and validation
- `config/` - Configuration loading
- `validate/` - Manifest validation rules
- `metrics/` - Logging and instrumentation
- `mount/` - Lock management and bind mounts
- `provider/` - Provider registry and config generation
- `cli/` - Command parsing

### Integration Tests (30%)
Real Linux namespace and mount operations:
```bash
make test-integration
```

**Requirements:**
- Linux kernel 3.8+
- Unprivileged user namespaces enabled
- `unshare` command available

**What's tested:**
- Actual namespace creation
- Lock file operations with real PIDs
- Bind mount preparation
- Cleanup operations

### E2E Tests (10%)

#### Basic E2E Tests
Complete workflow without LLM:
```bash
make test-e2e
```

**Tests:**
- Create workspace
- Edit manifest
- Validate manifest
- List workspaces
- Check status

#### MCP-Based E2E Tests
Tests using MCP (Model Context Protocol) server:
```bash
make test-e2e-mcp
```

**What's tested:**
- MCP server initialization
- Tool discovery via MCP
- Workspace operations via MCP tools
- Structured JSON responses

#### LLM-Driven E2E Tests (CLI)
Agent-driven tests using Qwen2.5-Coder via Ollama:
```bash
make test-e2e-llm
```

**Requirements:**
- Ollama installed and running
- Qwen2.5-Coder model available (`ollama pull qwen2.5-coder:7b-instruct`)

**Tests:**
- LLM generates workspace manifest
- LLM validates manifest structure
- LLM explains workspace purpose

#### LLM + MCP E2E Tests
LLM agent interacting with misbah via MCP protocol:
```bash
make test-e2e-llm-mcp
```

**Requirements:**
- Ollama installed and running
- Qwen2.5-Coder model available

**Tests:**
- LLM discovers MCP tools
- LLM generates JSON-RPC requests
- LLM creates/manages workspaces via MCP
- LLM troubleshoots validation errors
- Full agent-driven workflow

#### Containerized E2E Tests
Tests in isolated containers:
```bash
make test-e2e-container
```

**Requirements:**
- Podman installed

**Uses:**
- `Dockerfile.test` for test image
- Isolated environment for reproducibility

#### Claude Code E2E Tests
Integration tests with Claude Code CLI:
```bash
MISBAH_E2E_CLAUDE=true make test-e2e-claude
```

**Requirements:**
- Claude Code CLI binary in PATH (`claude`)
- `MISBAH_E2E_CLAUDE=true` environment variable (opt-in)

**What's tested:**
- Workspace creation and validation
- MCP tool discovery by Claude
- Claude querying workspaces via MCP
- Automated verification without interactive session

**Note:** This uses the MCP server interface for automation. Interactive Claude mounting tests are skipped (require manual verification).

## Build Tags

Tests use Go build tags for selective execution:

- `//go:build e2e` - E2E tests only
- `//go:build e2e && llm` - LLM-driven tests only
- `//go:build e2e && claude` - Claude Code integration tests only
- `//go:build integration && linux` - Integration tests (Linux only)

## Running Tests

### All Tests
```bash
make test
```

### Specific Test Types
```bash
make test-unit                        # Unit tests only
make test-integration                 # Integration tests (Linux only)
make test-e2e                         # Basic E2E tests
make test-e2e-mcp                     # MCP-based E2E tests
make test-e2e-llm                     # LLM-driven E2E tests
make test-e2e-llm-mcp                 # LLM + MCP E2E tests
make test-e2e-container               # Containerized E2E tests
MISBAH_E2E_CLAUDE=true test-e2e-claude # Claude Code E2E tests
```

### Coverage
```bash
make coverage
```

Generates `coverage.html` with detailed coverage report.

## LLM Testing with Qwen2.5-Coder

### Setup Ollama + Qwen2.5-Coder

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Pull Qwen2.5-Coder
ollama pull qwen2.5-coder:7b-instruct

# Verify
ollama list | grep qwen
```

### Model Selection

- **qwen2.5-coder:7b-instruct** - Recommended (fast, capable)
- **qwen2.5-coder:1.5b-instruct** - Lightweight (faster)
- **qwen2.5:32b** - Maximum capability (if available)

### LLM Test Approach

LLM tests follow this pattern:
1. Query Ollama API with structured prompt
2. Parse LLM response
3. Execute misbah commands with LLM output
4. Validate results

Example:
```go
prompt := "Create a YAML manifest for workspace 'test' with 2 sources..."
manifest := queryLLM(t, prompt)
// Save manifest, validate with misbah
run(t, "./misbah", "validate", "-w", "test")
```

## Continuous Integration

GitHub Actions workflow (`.github/workflows/test.yml`):
- Runs unit tests on every push
- Runs integration tests on Linux runners
- Runs E2E tests (non-LLM) on pull requests
- LLM tests run manually or on releases

## Test Philosophy

Following Scribe/Lex/Locus patterns:
- **Pure Go** - No Python, all tests in Go
- **Container isolation** - E2E tests use Podman
- **Build tags** - Separate slow tests from fast tests
- **Linux-first** - Core functionality requires Linux
- **Reproducible** - Deterministic tests, no flaky tests

## Adding New Tests

### Unit Test
Add `*_test.go` file in the package:
```go
func TestNewFeature(t *testing.T) {
    // Test implementation
}
```

### Integration Test
Add to `test/integration/`:
```go
//go:build integration && linux

func TestFeatureIntegration(t *testing.T) {
    // Test with real system resources
}
```

### E2E Test
Add to `test/e2e/e2e_test.go`:
```go
//go:build e2e

func TestFeatureE2E(t *testing.T) {
    // End-to-end workflow test
}
```

### LLM Test
Add to `test/e2e/llm_agent_test.go`:
```go
//go:build e2e && llm

func TestLLMFeature(t *testing.T) {
    manifest := queryLLM(t, "prompt...")
    // Validate LLM output
}
```

## Debugging Tests

### Verbose Output
```bash
go test -v -tags=e2e ./test/e2e/...
```

### Run Single Test
```bash
go test -v -tags=e2e -run TestBasicWorkflow ./test/e2e/...
```

### Debug LLM Responses
LLM tests log full responses:
```bash
go test -v -tags=e2e,llm -run TestLLM ./test/e2e/...
```

## Test Fixtures

### Manifests
Sample manifests in `fixtures/manifests/`:
- `valid.yaml` - Valid workspace
- `invalid-*.yaml` - Various invalid cases

### Mock Repos
Mock repository structures in `fixtures/mock-repos/`:
- Simple directory structures for testing

## Performance

Target test execution times:
- Unit tests: < 5 seconds
- Integration tests: < 30 seconds
- E2E tests: < 2 minutes
- LLM tests: < 5 minutes (depends on model)

## Known Limitations

1. **Linux-only**: Namespace tests require Linux
2. **Ollama dependency**: LLM tests need Ollama running
3. **Podman for containers**: E2E container tests need podman
4. **Permissions**: Some tests need unprivileged namespaces enabled
