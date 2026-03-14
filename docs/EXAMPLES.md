# Examples

## Basic Usage

### Create and Mount a Workspace

```bash
# Create a new workspace
jabal create -w myproject

# Edit the manifest
jabal edit -w myproject
```

Add sources to the manifest:

```yaml
name: myproject
description: My multi-repo project
sources:
  - path: ~/Projects/frontend
    mount: frontend
  - path: ~/Projects/backend
    mount: backend
  - path: ~/Projects/shared
    mount: shared
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
```

Mount and launch:

```bash
# Validate manifest
jabal validate -w myproject

# Mount with Claude Code
jabal mount -w myproject -a claude
```

Inside Claude, you'll have:

```
/tmp/jabal/myproject/
├── frontend/     # ~/Projects/frontend
├── backend/      # ~/Projects/backend
└── shared/       # ~/Projects/shared
```

### List Workspaces

```bash
# List all workspaces
jabal peaks

# Output:
# myproject (3 sources)
# demo-workspace (2 sources)
```

### Check Current Workspace

```bash
# From inside a mounted workspace
jabal summit

# Output:
# Workspace: myproject
# Provider: claude
# Mounted: 2026-03-13 10:30:00
# Sources: frontend, backend, shared
```

### Unmount Workspace

```bash
# Clean unmount (from outside the workspace)
jabal unmount -w myproject

# Force unmount (terminates provider)
jabal unmount -w myproject --force
```

## Advanced Usage

### Multi-Provider Configuration

Configure different providers in the same manifest:

```yaml
name: fullstack
sources:
  - path: ~/Projects/api
    mount: api
  - path: ~/Projects/web
    mount: web
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
      locus: http://localhost:8081
  aider:
    model: gpt-4
    auto-commits: true
```

Mount with different providers:

```bash
# Use Claude Code
jabal mount -w fullstack -a claude

# Or use Aider
jabal mount -w fullstack -a aider
```

### Workspace with Tags

Organize workspaces with tags:

```yaml
name: ecommerce
description: E-commerce platform
tags:
  - production
  - microservices
  - kubernetes
sources:
  - path: ~/Projects/checkout-service
    mount: checkout
  - path: ~/Projects/inventory-service
    mount: inventory
  - path: ~/Projects/payment-service
    mount: payment
```

### Nested Repositories

Handle nested project structures:

```yaml
name: monorepo
sources:
  - path: ~/Projects/monorepo/packages/core
    mount: core
  - path: ~/Projects/monorepo/packages/ui
    mount: ui
  - path: ~/Projects/monorepo/apps/web
    mount: web
```

**Note**: Sources cannot overlap. This would be invalid:

```yaml
# INVALID - nested paths
sources:
  - path: ~/Projects/monorepo
    mount: root
  - path: ~/Projects/monorepo/packages  # Error: nested under root
    mount: packages
```

## Closure Examples

### Hierarchical Directory Access

When working in a workspace with this structure:

```
workspace/
├── eco/
│   ├── sys/
│   └── com/
└── mod/
```

Use closure to restrict child agent access:

```python
# Inside Claude, working on full workspace
# Spawn child agent restricted to eco/ only

closure("eco", "analyze all dependencies in the ecosystem")
# Child can access: eco/sys/, eco/com/, etc.
# Child CANNOT access: mod/
```

### Nested Closures

```python
# Parent workspace
closure("eco", "review system layer")
  # Child 1 (eco/)
  closure("sys", "analyze communication layer")
    # Child 2 (eco/sys/)
    # Can only access eco/sys/
```

### Validation Errors

```python
# These will fail with hard errors:

# Absolute path (not allowed)
closure("/tmp/other", "...")  # Error: absolute paths not allowed

# Parent directory escape
closure("../other-mount", "...")  # Error: cannot escape mount

# Outside workspace
closure("eco/../../etc", "...")  # Error: path traversal blocked
```

## Real-World Scenarios

### Microservices Development

```yaml
name: payment-platform
description: Payment processing microservices
tags:
  - microservices
  - kubernetes
  - production
sources:
  - path: ~/Projects/payment-api
    mount: api
  - path: ~/Projects/payment-worker
    mount: worker
  - path: ~/Projects/payment-frontend
    mount: frontend
  - path: ~/Projects/shared-schemas
    mount: schemas
  - path: ~/Projects/infrastructure
    mount: infra
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
      locus: http://localhost:8081
      limes: http://localhost:8082
```

**Workflow**:

```bash
# Mount workspace
jabal mount -w payment-platform -a claude

# Inside Claude:
# - Review API changes in api/
# - Update worker to handle new events
# - Update frontend to display new data
# - Modify shared schemas for new fields
# - Update Kubernetes manifests in infra/
# - All in one unified workspace!
```

### Library Development with Examples

```yaml
name: ui-library
sources:
  - path: ~/Projects/component-library
    mount: lib
  - path: ~/Projects/example-app
    mount: example
  - path: ~/Projects/documentation
    mount: docs
providers:
  claude:
    mcp_servers:
      limes: http://localhost:8082  # Test runner
```

**Workflow**:

```bash
jabal mount -w ui-library -a claude

# Inside Claude:
# - Modify component in lib/
# - Test in example/
# - Update docs/
# - Run tests across all three repos
```

### Full-Stack Application

```yaml
name: blog-platform
sources:
  - path: ~/Projects/blog-api
    mount: api
  - path: ~/Projects/blog-web
    mount: web
  - path: ~/Projects/blog-mobile
    mount: mobile
  - path: ~/Projects/blog-admin
    mount: admin
  - path: ~/Projects/blog-common
    mount: common
```

**Workflow**:

```bash
jabal mount -w blog-platform -a claude

# Inside Claude:
# - Add new API endpoint in api/
# - Add route in web/
# - Add screen in mobile/
# - Add admin UI in admin/
# - Share types in common/
# - Consistent changes across all repos
```

## Provider-Specific Features

### Claude Code Configuration

```yaml
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
      locus: http://localhost:8081
      limes: http://localhost:8082
    settings:
      auto_memory: true
      max_context_tokens: 200000
```

Generates `~/.config/jabal/workspaces/{workspace}/.claude/settings.local.json`:

```json
{
  "mcpServers": {
    "scribe": {
      "url": "http://localhost:8080"
    },
    "locus": {
      "url": "http://localhost:8081"
    },
    "limes": {
      "url": "http://localhost:8082"
    }
  }
}
```

### Aider Configuration

```yaml
providers:
  aider:
    model: gpt-4
    auto-commits: true
    edit-format: diff
```

Generates `~/.config/jabal/workspaces/{workspace}/.aider/.aider.conf.yml`:

```yaml
model: gpt-4
auto-commits: true
edit-format: diff
```

## Troubleshooting Examples

### Workspace Locked

```bash
# Check who has the lock
jabal summit -w myproject
# Output: Locked by PID 12345 (user: dpopsuev)

# Force unlock (terminates provider)
jabal unmount -w myproject --force
```

### Validation Errors

```bash
# Validate before mounting
jabal validate -w myproject

# Example errors:
# Error: Source path does not exist: ~/Projects/missing
# Error: Duplicate mount name: 'frontend'
# Error: Invalid mount name: 'my mount' (spaces not allowed)
```

### Debug Mode

```bash
# Enable verbose logging
jabal mount -w myproject -a claude --log-level debug

# Output:
# [DEBUG] Loading manifest: ~/.config/jabal/workspaces/myproject/manifest.yaml
# [DEBUG] Validating 3 sources
# [DEBUG] Acquiring lock: /tmp/jabal/.locks/myproject.lock
# [DEBUG] Creating namespace
# [DEBUG] Mounting: ~/Projects/frontend -> /tmp/jabal/myproject/frontend
# ...
```

## Tips and Best Practices

### Workspace Organization

- Use descriptive workspace names (`payment-platform` not `proj1`)
- Add tags for categorization (`production`, `experimental`, etc.)
- Keep manifests in version control (minus provider configs)

### Source Management

- Use `~` for home directory paths (portable across users)
- Avoid nested sources (causes validation errors)
- Use descriptive mount names (`api` not `a`)

### Provider Configuration

- Keep provider-specific settings minimal
- Let jabal generate configs (don't edit manually)
- Use same manifest for multiple providers

### Performance

- Limit sources to 5-10 per workspace (faster mount)
- Use SSDs for source repositories (faster bind mounts)
- Clean up unused workspaces (`jabal peaks` to review)

### Safety

- Always validate before mounting (`jabal validate`)
- Use `--force` sparingly (can lose unsaved work)
- Check `jabal summit` before force unmounting
