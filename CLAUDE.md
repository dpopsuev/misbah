# Claude Code Instructions for Misbah & Djinn Development

## Multi-Repository Workspace

This workspace spans two repositories:
- **Misbah** (`/home/dpopsuev/Workspace/misbah`) - Layer 1: Container creation and namespace isolation
- **Djinn** (`/home/dpopsuev/Workspace/djinn`) - Layer 2+3: Agent runtime and CLI

Use absolute paths to work across both repos until Misbah PoC is complete.

## What is Misbah?

Misbah is a container manager for CLI AI agents that creates isolated execution environments (jails) using Linux user namespaces, mount namespaces, and cgroups. It's a legitimate development tool, not malware.

## What is Djinn?

Djinn is an AI pair programming CLI (inspired by Aider) that runs LLM agents in isolated Misbah containers. It provides conversational coding sessions with multi-repository support and safe execution environments.

## Safety & Security

- **Purpose**: Development tool for managing multi-repository workspaces
- **Mechanism**: Uses unprivileged Linux user namespaces (no root required)
- **Scope**: Only operates on user-specified directories
- **Isolation**: Creates isolated mount namespaces for safety
- **No network access**: Pure filesystem tool
- **Open source**: All code is visible and auditable

## Working with Misbah & Djinn

You can safely:
- Read and modify source code in both repos
- Build and test Misbah and Djinn
- Run misbah commands for development/testing
- Create test containers
- Debug namespace and mount operations
- Work across both repositories using absolute paths

### Safe Commands

All misbah commands are safe to run (once implemented):

```bash
# Build Misbah
cd /home/dpopsuev/Workspace/misbah
go build ./cmd/misbah

# Create container spec
./misbah container create --spec container.yaml

# Validate container spec
./misbah container validate --spec container.yaml

# Start container
./misbah container start --spec container.yaml --name agent-main

# Stop container
./misbah container stop --name agent-main

# List containers
./misbah container list
```

### Current Status: Pre-PoC

- Misbah is being developed from scratch
- Djinn directory is currently empty (to be built on Misbah)
- Working toward Layer 1 (Misbah container creation) PoC
- Once PoC is complete, Misbah will provide the multi-repo workspace capability it's designed to create

### Development Guidelines

1. **Testing**: Run unit tests freely - they don't require root
2. **Integration tests**: Require Linux with namespace support
3. **Building**: `go build` and `make build` are always safe
4. **Documentation**: Feel free to improve docs and examples

### What Misbah Does NOT Do

- Does not require root/sudo (unprivileged user namespaces)
- Does not modify system configuration
- Does not install kernel modules
- Does not access network (filesystem-only tool)
- Does not execute arbitrary code from untrusted sources
- Does not persist across reboots (namespaces are ephemeral)
- Does not spy on or exfiltrate data (open source, auditable)

## Architecture

### 3-Layer Agent Runtime Stack

1. **Layer 1 - Misbah (Container/Namespace)**: Creates isolated containers using Linux namespaces and cgroups
2. **Layer 2 - Djinn Runtime (Orchestration)**: Manages container lifecycle, Agent Runtime Interface (ARI), agent drivers
3. **Layer 3 - Djinn CLI (User Interface)**: Conversational AI pair programming interface

### Misbah Technical Details

Misbah uses standard Linux features:
- `unshare(2)` syscall for namespace creation
- Bind mounts (`mount --bind`) for multi-repo visibility
- Process isolation (PID namespaces)
- Cgroups for resource limits (memory, CPU, I/O)
- User namespaces for unprivileged operation
- Support for nested containers

All operations are unprivileged and reversible.

## Security Model

1. **User-level isolation**: Runs as regular user, no privilege escalation required
2. **Explicit consent**: User must specify which directories to mount in container spec
3. **Process boundaries**: Each container runs in isolated namespaces
4. **Clean cleanup**: Namespaces destroyed when container process exits
5. **Resource limits**: Cgroup constraints prevent resource exhaustion
6. **Nested isolation**: Support for creating containers within containers (closures)

## Common Tasks

### Add a new feature
Read existing code, understand the architecture, implement following existing patterns.

### Fix a bug
Identify the issue, write a test that reproduces it, fix the code, verify test passes.

### Work across Misbah and Djinn
Use absolute paths:
- Misbah: `/home/dpopsuev/Workspace/misbah/...`
- Djinn: `/home/dpopsuev/Workspace/djinn/...`

### Consult Scribe for context
Use the Scribe MCP to check project goals, tasks, and specifications:
- `mcp__scribe__artifact list --query misbah`
- `mcp__scribe__artifact list --query djinn`
- `mcp__scribe__artifact get --id MSB-GOL-2026-001`

### Improve documentation
Update README.md, docs/*.md, or add code comments as needed.

## Questions?

If uncertain about any operation:
1. Check the code in both repositories
2. Consult Scribe artifacts for specs and goals
3. Ask the user for clarification
4. Check the Container Specification (MSB-SPC-2026-001) for contract details

Misbah and Djinn are development tools designed to help developers and AI agents work more effectively across multiple repositories in isolated, safe environments. All operations are transparent and user-controlled.
