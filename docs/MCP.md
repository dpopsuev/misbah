# MCP Server Integration

Jabal provides a Model Context Protocol (MCP) server for seamless integration with AI agents and automation tools.

## Overview

The MCP server exposes jabal functionality via HTTP JSON-RPC, making it easy for AI agents to:
- Discover and manage workspaces
- Create and update manifests
- Validate configurations
- Query workspace status
- List available providers

## Starting the Server

```bash
# Default (localhost:8080)
jabal serve

# Custom address/port
jabal serve --addr 0.0.0.0 --port 9000

# With debug logging
jabal serve --verbose
```

## MCP Protocol

Jabal implements **MCP 2024-11-05** specification.

### Initialization

```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "my-client",
        "version": "1.0"
      }
    }
  }'
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {}
    },
    "serverInfo": {
      "name": "jabal",
      "version": "0.1.0"
    }
  }
}
```

### List Available Tools

```bash
curl -X POST http://localhost:8080 \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Returns all available MCP tools with their schemas.

## Available MCP Tools

### jabal_list_workspaces

List all available workspaces.

**Input:** (empty)

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_list_workspaces",
      "arguments": {}
    }
  }'
```

**Response:**
```json
{
  "content": [{
    "type": "text",
    "text": "{\"workspaces\":[{\"name\":\"myproject\",\"description\":\"My project\",\"sources\":3}],\"total\":1}"
  }]
}
```

### jabal_create_workspace

Create a new workspace.

**Input:**
- `name` (string, required): Workspace name
- `description` (string, optional): Workspace description

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_create_workspace",
      "arguments": {
        "name": "myproject",
        "description": "My awesome project"
      }
    }
  }'
```

**Response:**
```json
{
  "content": [{
    "type": "text",
    "text": "✓ Workspace 'myproject' created successfully at /home/user/.config/jabal/workspaces/myproject/manifest.yaml"
  }]
}
```

### jabal_get_workspace

Get workspace details including full manifest.

**Input:**
- `name` (string, required): Workspace name

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_get_workspace",
      "arguments": {
        "name": "myproject"
      }
    }
  }'
```

**Response:**
```json
{
  "content": [{
    "type": "text",
    "text": "{\"name\":\"myproject\",\"description\":\"My awesome project\",\"sources\":[...],\"providers\":{...}}"
  }]
}
```

### jabal_update_manifest

Update workspace manifest.

**Input:**
- `name` (string, required): Workspace name
- `manifest` (object, required): Complete manifest object

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_update_manifest",
      "arguments": {
        "name": "myproject",
        "manifest": {
          "name": "myproject",
          "description": "Updated description",
          "sources": [
            {"path": "/home/user/project-a", "mount": "project-a"},
            {"path": "/home/user/project-b", "mount": "project-b"}
          ],
          "providers": {
            "claude": {
              "mcp_servers": {
                "scribe": "http://localhost:8080"
              }
            }
          }
        }
      }
    }
  }'
```

### jabal_validate_workspace

Validate a workspace manifest.

**Input:**
- `name` (string, required): Workspace name

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_validate_workspace",
      "arguments": {
        "name": "myproject"
      }
    }
  }'
```

**Response:**
```json
{
  "content": [{
    "type": "text",
    "text": "✓ Workspace 'myproject' is valid"
  }],
  "isError": false
}
```

### jabal_get_status

Get workspace mount status.

**Input:**
- `name` (string, required): Workspace name

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_get_status",
      "arguments": {
        "name": "myproject"
      }
    }
  }'
```

**Response:**
```json
{
  "content": [{
    "type": "text",
    "text": "{\"Workspace\":\"myproject\",\"Mounted\":false}"
  }]
}
```

### jabal_list_providers

List available providers (claude, aider, cursor).

**Input:** (empty)

**Example:**
```bash
curl -X POST http://localhost:8080 \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "jabal_list_providers",
      "arguments": {}
    }
  }'
```

## Error Handling

Errors are returned in MCP error format:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": 500,
    "message": "Workspace 'nonexistent' does not exist"
  }
}
```

Tool-level errors (validation failures, etc.) return success with `isError: true`:

```json
{
  "content": [{
    "type": "text",
    "text": "Validation failed: duplicate mount name: 'source'"
  }],
  "isError": true
}
```

## AI Agent Integration

### Claude Code

Add to `.claude/settings.json`:

```json
{
  "mcpServers": {
    "jabal": {
      "url": "http://localhost:8080"
    }
  }
}
```

### Python Client (OpenAI-compatible)

```python
import requests

def call_jabal_tool(tool_name, arguments):
    response = requests.post("http://localhost:8080", json={
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    })
    return response.json()["result"]

# List workspaces
workspaces = call_jabal_tool("jabal_list_workspaces", {})
print(workspaces)

# Create workspace
result = call_jabal_tool("jabal_create_workspace", {
    "name": "my-project",
    "description": "My awesome project"
})
print(result)
```

### Go Client

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type MCPRequest struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      int         `json:"id"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params"`
}

func callJabalTool(tool string, args map[string]interface{}) (map[string]interface{}, error) {
    req := MCPRequest{
        JSONRPC: "2.0",
        ID:      1,
        Method:  "tools/call",
        Params: map[string]interface{}{
            "name":      tool,
            "arguments": args,
        },
    }

    body, _ := json.Marshal(req)
    resp, err := http.Post("http://localhost:8080", "application/json", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result["result"].(map[string]interface{}), nil
}
```

## Use Cases

### Automated Workspace Management

```bash
# AI agent creates workspace
jabal_create_workspace(name="feature-x")

# AI agent adds sources
jabal_update_manifest(
    name="feature-x",
    manifest={
        "sources": [
            {"path": "/home/user/frontend", "mount": "frontend"},
            {"path": "/home/user/backend", "mount": "backend"}
        ]
    }
)

# AI agent validates
jabal_validate_workspace(name="feature-x")
```

### Multi-Repository Analysis

AI agent discovers all workspaces, analyzes their structure, and provides insights.

### Workspace Templates

AI agent generates manifests based on project type detection.

## Security Considerations

1. **Localhost only by default**: MCP server binds to 127.0.0.1
2. **No authentication**: Intended for local development only
3. **Validation**: All inputs validated before execution
4. **Read-only operations**: Most tools are read-only
5. **Namespace isolation**: Workspace operations are isolated

For production use:
- Use `--addr 127.0.0.1` (never 0.0.0.0 on public networks)
- Add authentication layer if exposing externally
- Run behind reverse proxy with auth

## Debugging

Enable debug logging:

```bash
jabal serve --verbose
```

Test with verbose curl:

```bash
curl -v -X POST http://localhost:8080 \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

## Comparison with CLI

| Task | CLI | MCP |
|------|-----|-----|
| List workspaces | `jabal peaks` | `jabal_list_workspaces()` |
| Create workspace | `jabal create -w test` | `jabal_create_workspace(name="test")` |
| Validate | `jabal validate -w test` | `jabal_validate_workspace(name="test")` |
| Get status | `jabal summit -w test` | `jabal_get_status(name="test")` |

**Advantages of MCP:**
- Structured JSON responses (no text parsing)
- Type-safe via JSON Schema
- Auto-discoverable by AI agents
- Programmatic access
- Better error handling

**Advantages of CLI:**
- Interactive use
- Shell integration
- Simpler for humans
- No server process needed
