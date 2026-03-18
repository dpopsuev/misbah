# Misbah

A container manager for CLI AI agents that creates isolated execution environments with progressive trust permission brokering.

## What It Does

Misbah runs AI agents (Claude Code, Aider, etc.) inside isolated containers. All outbound network access, MCP tool calls, and package installations are blocked by default. When the agent needs access, the user is prompted with **[Once] / [Always] / [Deny]**.

```
Agent: curl api.github.com
  |
  v
Network Proxy (inside container)
  |
  v
Permission Daemon (on host)
  |
  v
User: [O]nce  [A]lways  [D]eny: _
```

## Architecture

Two backends, one security model:

- **Namespace backend** (local dev): Daemonless, rootless. Uses Linux user namespaces and bind mounts. Works out of the box.
- **Kata backend** (VM isolation): Runs agents in hardware-isolated Kata VMs via a privileged daemon. Requires containerd + Kata Containers.

```
User (unprivileged)
  |
  +-- misbah container start --spec agent.yaml          (namespace: direct)
  +-- misbah container start --spec agent.yaml --runtime kata  (kata: via daemon)
        |
        v
  Misbah Daemon (privileged, systemd)
    +-- containerd/CRI --> Kata VM (QEMU/KVM)
    +-- Permission broker --> user prompts
    +-- Audit log
```

## Quick Start

### Build

```bash
make build
# Produces bin/misbah and bin/misbah-proxy
```

### Namespace Container (no setup required)

```bash
# Create a container spec
bin/misbah container create --spec agent.yaml --name my-agent

# Run it
bin/misbah container start --spec agent.yaml

# With permission daemon (enables progressive trust)
bin/misbah daemon start &
bin/misbah container start --spec agent.yaml
# Agent traffic now routed through proxy -> daemon -> user prompt
```

### Kata Container (requires infrastructure)

```bash
# Install: containerd, kata-containers, CNI plugins
# See docs/INSTALL.md

# Install and start daemon
sudo cp bin/misbah bin/misbah-proxy /usr/local/bin/
sudo cp assets/misbah-daemon.service /etc/systemd/system/
sudo groupadd misbah && sudo usermod -aG misbah $USER
sudo systemctl enable --now misbah-daemon

# Run (as unprivileged user in misbah group)
bin/misbah container start --spec agent.yaml --runtime kata
```

### Container Spec Example

```yaml
version: "1.0"
metadata:
  name: coding-agent
process:
  command: ["/bin/bash"]
  cwd: /workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: bind
    source: /home/user/project
    destination: /workspace
    options: [rw]
permissions:
  default_policy: prompt
  network_whitelist:
    - api.github.com
  mcp_whitelist:
    - scribe_list
```

## Progressive Trust

Every resource access goes through the permission daemon:

| Resource | Proxy | Example |
|----------|-------|---------|
| Network | HTTP/HTTPS forward proxy | `curl api.github.com` |
| MCP tools | JSON-RPC reverse proxy | `scribe artifact create` |
| Packages | CLI wrapper | `pip install numpy` |

Decisions persist: **Always** is saved to whitelist, **Once** is ephemeral, **Deny** blocks permanently.

## Security Model

```
Host:
  Daemon (root:containerd) --> containerd socket
  Socket (/run/misbah/permission.sock, root:misbah 660)

Container:
  Agent --> proxy --> permission.sock --> daemon --> user
  Cannot: access containerd, host filesystem, daemon API socket
```

- Namespace backend: unprivileged user namespaces (no root)
- Kata backend: separate kernel per agent (QEMU/KVM)
- Daemon socket: `root:misbah 660` (Docker model)
- Zero-trust: deny-by-default, explicit user approval

## Project Structure

```
cmd/misbah/          Entry point
cmd/misbah-proxy/    Network proxy binary (runs inside containers)
cli/                 Cobra CLI commands
daemon/              Permission daemon + container lifecycle server
proxy/               Network, MCP, and package proxies
cri/                 CRI client for containerd/Kata
runtime/             Namespace backend, lifecycle, locks, cgroups
model/               Container spec, lock model
config/              Configuration and path defaults
tier/                Tier namespace mount generation (Eco/Sys/Com/Mod)
mcp/                 MCP protocol server
metrics/             Structured logging
test/harness/        E2E test harness (Lab isolation, probes)
test/e2e/            End-to-end tests
test/integration/    Integration tests
assets/              Systemd unit file
```

## Requirements

- Linux kernel 5.10+ with unprivileged user namespaces
- Go 1.23+ (building from source)
- For Kata: containerd, kata-containers, KVM (`/dev/kvm`), CNI plugins

## Status

**Namespace backend**: Operational. Containers run with full isolation, progressive trust wired end-to-end.

**Kata backend**: Code complete. Blocked by upstream Kata networking issue on Fedora 43 / kernel 6.18 (MSB-MIR-4).

## License

MIT
