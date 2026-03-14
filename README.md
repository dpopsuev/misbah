# Misbah

A workspace manager for CLI AI agents that enables multi-repository development through Linux user namespaces and bind mounts.

## Overview

CLI AI agents (Claude Code, Aider, Cursor) are currently bound to a single working directory. Misbah solves this by creating unified workspaces from multiple source repositories using Linux namespaces, allowing agents to seamlessly work across project boundaries.

## Features

- **Multi-repository workspaces**: Mount multiple source directories into a unified namespace
- **Zero-privilege isolation**: Uses unprivileged user namespaces (no root required)
- **Provider integration**: Native support for Claude Code, Aider, and Cursor
- **MCP Server**: Model Context Protocol server for AI agent integration
- **Closure semantics**: Hierarchical namespace restrictions for child agents
- **Lock management**: Prevents concurrent access conflicts

## Requirements

- Linux kernel 3.8+ with unprivileged user namespaces enabled
- Go 1.22+ (for building from source)
- util-linux (mount command)
- Provider binaries in PATH (claude, aider, etc.)

## Quick Start

### CLI Usage

```bash
# Install
make install

# Create a workspace manifest
misbah create -w myworkspace

# Edit the manifest to add sources
misbah edit -w myworkspace

# Mount and launch Claude Code
misbah mount -w myworkspace -a claude

# List workspaces
misbah peaks

# Current workspace status
misbah summit
```

### MCP Server Usage

```bash
# Start MCP server
misbah serve --port 8080

# MCP server provides these tools for AI agents:
# - misbah_list_workspaces
# - misbah_create_workspace
# - misbah_get_workspace
# - misbah_update_manifest
# - misbah_validate_workspace
# - misbah_get_status
# - misbah_list_providers

# Test MCP server
curl -X POST http://localhost:8080 \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

## Documentation

- [Installation Guide](docs/INSTALL.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Examples](docs/EXAMPLES.md)

## Project Status

🚧 **v0.1.0 MVP in development** - First E2E test in progress

## License

MIT License - See LICENSE file for details
