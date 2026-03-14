# Architecture

## Overview

Jabal uses Linux user namespaces and bind mounts to create unified workspaces from multiple source repositories. The core mechanism requires no root privileges and operates entirely in userspace.

## Core Technology

### Linux Namespaces

Jabal leverages three namespace types:

1. **User Namespace** (`--user`): Provides unprivileged container isolation
2. **Mount Namespace** (`--mount`): Isolates filesystem mount points
3. **PID Namespace** (`--pid`): Isolates process tree (for cleanup)

### System Call Flow

```bash
unshare --user --mount --map-root-user --pid --fork bash -c '
  mkdir -p /tmp/jabal/{workspace}
  mount --bind ~/Projects/repo-a /tmp/jabal/{workspace}/repo-a
  mount --bind ~/Projects/repo-b /tmp/jabal/{workspace}/repo-b
  mount --bind ~/.config/jabal/workspaces/{workspace}/.claude /tmp/jabal/{workspace}/.claude
  cd /tmp/jabal/{workspace}
  exec claude
'
```

## Package Structure

Jabal follows a layered architecture with clear dependency flow:

```
model (core domain types)
  ↑
  ├─ config (global configuration)
  ├─ validate (manifest validation)
  ├─ mount (namespace + bind mounts) ← CRITICAL
  └─ provider (config translators)
        ↑
        └─ cli, mcp, metrics (presentation)
```

### Dependency Rules

- **High fan-in**: Model package is imported by ~7 packages
- **Flat layout**: No nested package hierarchies
- **Clear boundaries**: Business logic (mount, provider) separated from presentation (cli, mcp)

## Package Responsibilities

### model/

Core domain types with no external dependencies (except stdlib and YAML):

- `Workspace`: Workspace entity
- `Manifest`: YAML manifest structure
- `Source`: Source directory specification
- `Lock`: Lock file format
- Domain errors

### config/

Global configuration management:

- Load `~/.config/jabal/config.yaml`
- Path resolution (`~`, `$HOME`)
- Default values

### validate/

Fail-fast manifest validation:

- YAML syntax validation
- Required field checks
- Source path validation (exists, not nested)
- Mount name validation (unique, alphanumeric)
- Provider registry checks

### mount/

Namespace creation and lifecycle management (**CRITICAL PATH**):

- Namespace creation via `unshare` syscall
- Bind mount operations
- Lock acquisition/release with stale detection
- Signal handling (SIGTERM, SIGINT, SIGCHLD)
- Cleanup orchestration

### provider/

Provider registry and config translators:

- Provider interface definition
- Claude Code translator (`.claude/settings.local.json`)
- Aider translator (`.aider/.aider.conf.yml`)
- Cursor translator (placeholder)
- Config precedence rules

### cli/

Cobra-based command interface:

- Command definitions (mount, unmount, peaks, summit, etc.)
- Argument parsing and validation
- Exit code standardization (0-10)
- Error formatting

### mcp/

Model Context Protocol server (future):

- MCP server implementation
- Workspace context exposure
- Integration with Claude Desktop

### metrics/

Structured logging and instrumentation:

- JSON logging with zerolog
- Metrics recording
- Optional Prometheus exporter

## Data Flow

### Mount Operation

```
User: jabal mount -w myworkspace -a claude
  ↓
CLI: Parse args → Load manifest
  ↓
Validate: Check manifest → Validate sources
  ↓
Mount: Acquire lock → Create namespace → Bind sources
  ↓
Provider: Generate .claude/settings.local.json
  ↓
Mount: cd workspace → exec claude
  ↓
Claude: Works in unified namespace
  ↓
User: Exit Claude
  ↓
Mount: Cleanup → Release lock → Destroy namespace
```

### Closure Operation

```
Parent Agent: closure("eco", "analyze dependencies")
  ↓
Mount: Validate relative path (no absolute, no ..)
  ↓
Mount: Spawn child with restricted namespace
  ↓
Child: Can only access parent-workspace/eco/
  ↓
Child: Hard error on violation (../other-mount)
  ↓
Mount: Wait for child exit → Cleanup
```

## File Layout

```
~/.config/jabal/
├── config.yaml                      # Global config
└── workspaces/
    ├── workspace-a/
    │   ├── manifest.yaml            # Workspace manifest
    │   └── .claude/                 # Generated provider config
    │       └── settings.local.json
    └── workspace-b/
        └── manifest.yaml

/tmp/jabal/
├── .locks/
│   ├── workspace-a.lock             # Lock files (PID, timestamp)
│   └── workspace-b.lock
└── workspace-a/                     # Active mount (destroyed on exit)
    ├── repo-a/                      # Bind mount
    ├── repo-b/                      # Bind mount
    └── .claude/                     # Bind mount
```

## Locking Strategy

### Lock File Format

```json
{
  "workspace": "myworkspace",
  "provider": "claude",
  "pid": 12345,
  "started_at": "2026-03-13T10:30:00Z",
  "user": "dpopsuev"
}
```

### Stale Lock Detection

1. Check if PID exists: `/proc/{pid}/`
2. Check if PID belongs to same user
3. Check if process is provider binary
4. If stale: remove lock and acquire

### Force Termination

```bash
jabal unmount -w myworkspace --force
  → SIGTERM to PID
  → Wait 5s
  → SIGKILL if still alive
  → Remove lock
```

## Closure Semantics

### Namespace Hierarchy

```
Root workspace
└── eco/
    └── sys/
        └── com/
            └── mod/
```

### Enforcement

- Child agents receive **restricted mount namespace**
- Path validation: reject absolute paths, `..`, symlinks
- Hard error on violations (no warnings)
- Cleanup on child exit

### Example

```bash
# Parent: Working in full workspace
closure("eco", "analyze dependencies")
  → Child can access: workspace/eco/* only
  → Child CANNOT access: workspace/sys/, workspace/com/, etc.
```

## Performance Characteristics

### Mount Time

- Typical workspace (5 sources): **~200-500ms**
- Dominated by bind mount operations
- No filesystem copying (zero-copy bind mounts)

### Validation Time

- YAML parsing: **~10-50ms**
- Path checks: **~10-20ms per source**
- Total: **<100ms** for typical workspace

### Cleanup Time

- Signal delivery: **~10-50ms**
- Namespace destruction: **~100-200ms**
- Lock removal: **~10ms**
- Total: **<500ms**

## Error Handling

### Exit Codes

```
0  - Success
1  - General error
2  - Validation error
3  - Lock acquisition failed
4  - Mount operation failed
5  - Provider error
6  - Namespace error
7  - Signal error
8  - Cleanup error
9  - Configuration error
10 - Unknown error
```

### Error Propagation

- Business logic: Return domain errors
- CLI layer: Convert to exit codes
- Logs: Structured JSON with context

## Security Considerations

### Namespace Isolation

- No privilege escalation (unprivileged namespaces)
- Filesystem isolation (mount namespace)
- Process isolation (PID namespace)

### Path Validation

- Reject absolute paths in manifests (use `~` or relative)
- Validate no directory nesting (prevent overlaps)
- Symlink resolution and validation

### Lock File Safety

- Atomic write (write + rename)
- PID validation prevents stale locks
- User ownership checks

## Testing Strategy

### Unit Tests (60%)

- All packages have `*_test.go` files
- Mock dependencies (filesystem, syscalls)
- Table-driven tests for validation rules

### Integration Tests (30%)

- Real namespace creation (Linux only)
- Real bind mounts
- Lock file operations
- Cleanup verification

### E2E Tests (10%)

- Full workflow: Mount → Launch → Edit → Unmount
- Real provider binaries (Claude Code)
- Multi-repository scenarios
- Nested closure tests

## Future Enhancements

### v0.2.0+

- MCP server implementation
- Remote source support (git clone on mount)
- Workspace templates
- Multi-provider mounting (Claude + Aider simultaneously)
- Workspace snapshots
- Performance metrics dashboard
