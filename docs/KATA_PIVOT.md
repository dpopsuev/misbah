# Kata Container Pivot - Design Decisions & Implementation Plan

**Date**: 2026-03-14
**Status**: APPROVED - Architectural pivot to Kata Containers
**Campaign**: AWT-CMP-2026-002 (Agent Runtime Stack v1)

---

## Design Decisions

### Core Requirements

1. **Zero-Trust Security Model**
   - **Decision**: Treat all LLM agents as potentially malicious
   - **Rationale**: Agents have full control inside jails (apt install, pip install, code execution)
   - **Implication**: User namespace isolation is INSUFFICIENT
   - **Solution**: VM-level isolation boundary (Kata Containers)

2. **Full Control for Agents**
   - **Decision**: Agents must be able to install packages, modify files, run arbitrary commands
   - **Rationale**: Real development workflows require package managers, build tools, debuggers
   - **Implication**: Agent needs root-like capabilities inside jail
   - **Solution**: Kata provides full root inside VM without host compromise

3. **Image-Based Reproducibility**
   - **Decision**: Base environments must be reproducible and shareable
   - **Rationale**: "Works on my machine" is unacceptable for multi-user/multi-environment deployment
   - **Implication**: Need OCI-compatible image system
   - **Solution**: Kata supports OCI images natively via CRI

4. **Platform Agnostic**
   - **Decision**: Misbah must work identically on local dev machines and K8s/OCP clusters
   - **Rationale**: Same code path = less bugs, easier testing, better DX
   - **Implication**: No "dual-mode" or environment detection
   - **Solution**: Kata runs locally AND in K8s with identical interface

5. **Red Hat OCP Production**
   - **Decision**: Must be supported in Red Hat OpenShift Container Platform
   - **Rationale**: Enterprise deployment target requires vendor support
   - **Implication**: Firecracker (not OCP-supported) is ruled out
   - **Solution**: Kata Containers officially supported by Red Hat

---

## Technology Selection

### Kata Containers: Why Chosen

| Requirement | Kata Solution | Alternatives |
|-------------|---------------|--------------|
| **Zero-Trust** | VM boundary (QEMU/KVM) | ❌ Namespaces (kernel shared) |
| **Full Control** | Root inside VM | ✅ gVisor (limited), ✅ Firecracker |
| **OCI Images** | Native CRI support | ❌ Namespaces (no images) |
| **OCP Support** | ✅ Official Red Hat | ❌ Firecracker, ❌ gVisor |
| **Local + K8s** | Same runtime everywhere | ❌ Firecracker (K8s complex) |
| **Security > Perf** | 200ms startup acceptable | ✅ Good isolation/perf balance |

### Rejected Alternatives

**User Namespaces (Current Implementation)**
- ❌ Kernel exploits can escape to host
- ❌ Does not meet Zero-Trust requirement
- ✅ Keep for Phase 1 validation only

**Firecracker (AWS microVMs)**
- ✅ Excellent isolation (~125ms startup)
- ❌ No OCP support
- ❌ No CRI integration (custom work)
- ❌ Optimized for serverless, not K8s

**gVisor (Google)**
- ✅ Good isolation (syscall filtering)
- ❌ No Red Hat support
- ❌ Some syscalls unimplemented
- ❌ Not as strong as VM boundary

---

## Architecture Update

### Previous Architecture (Namespace-based)

```
Misbah CLI
    ↓
Direct syscalls (unshare, mount, cgroups)
    ↓
Linux Kernel (shared)
    ↓
┌─────────────────────────┐
│ User Namespace          │ ⚠️ Kernel exploit = escape
│ - UID 0 → 100000        │
│ - Mount namespace       │
│ - PID namespace         │
└─────────────────────────┘
```

### New Architecture (Kata-based)

```
Misbah CLI (local) OR Djinn API (K8s)
    ↓
CRI Client (k8s.io/cri-api)
    ↓
Kata Runtime (containerd-shim-kata-v2)
    ↓
┌──────────────────────────────────┐
│ QEMU/KVM MicroVM                 │ ✅ VM boundary
│                                  │
│  ┌────────────────────────────┐  │
│  │ Guest Linux Kernel         │  │ ✅ Separate kernel
│  │                            │  │
│  │  Agent Jail Environment    │  │
│  │  - Root inside VM ✅       │  │
│  │  - apt install ✅          │  │
│  │  - pip install ✅          │  │
│  │  - git clone/commit ✅     │  │
│  │  - Kernel exploit → VM ❌  │  │
│  └────────────────────────────┘  │
└──────────────────────────────────┘
```

---

## Specification Impact

### MSB-SPC-2026-001: Jail Specification v1.0

**UPDATED**: Add `runtime` field

```yaml
version: "1.0"
runtime: kata  # NEW FIELD (namespace|kata|gvisor)
image: ghcr.io/myorg/agent-base:v1  # NEW FIELD (OCI image reference)
metadata:
  name: agent-session-123
  labels:
    tenant: customer-xyz
process:
  command: ["/usr/bin/djinn-agent"]
  env: ["DJINN_SESSION=123"]
  cwd: /workspace
namespaces:
  user: true
  mount: true
  pid: true
  network: true  # Required for apt/pip/git
mounts:
  - type: git-clone  # NEW TYPE
    repo: https://github.com/customer/backend.git
    branch: main
    credentials: github-token-secret
    destination: /workspace/backend
    options: [rw]
  - type: tmpfs
    destination: /tmp
    options: [rw, size=1G]
resources:
  memory: 4GB
  cpu: 2
  timeout: 3600  # 1 hour max session
```

**New Fields**:
- `runtime`: Runtime selection (kata, namespace, gvisor)
- `image`: OCI image reference (required for kata/gvisor)

**New Mount Type**:
- `git-clone`: Clone git repository into jail during creation

### MSB-SPC-2026-002: Misbah CLI Specification

**UPDATED**: Add image management commands

```bash
# Image management
misbah image pull <image-ref>
misbah image list
misbah image inspect <image-ref>
misbah image prune

# Jail commands (existing, unchanged)
misbah jail create --spec jail.yaml
misbah jail validate --spec jail.yaml
misbah jail start --spec jail.yaml
misbah jail stop --name <jail-name>
misbah jail list
misbah jail inspect --name <jail-name>
misbah jail destroy --name <jail-name>
```

### DJN-SPC-2026-004: Djinnfile Format Specification

**UPDATED**: Add image field to agent section

```yaml
version: "1.0"
workspace:
  name: my-fullstack-app
sources:
  - name: backend
    repo: https://github.com/customer/backend.git
    branch: main
  - name: frontend
    repo: https://github.com/customer/frontend.git
    branch: main
agent:
  driver: claude
  model: claude-sonnet-4
  image: ghcr.io/myorg/agent-base:v1  # NEW FIELD
  runtime: kata  # NEW FIELD (optional, default: kata)
  capabilities:
    - code_execution
    - file_modification
    - package_installation
  credentials:
    anthropic_api_key_env: ANTHROPIC_API_KEY
    github_token_env: GITHUB_TOKEN
resources:
  memory: 4GB
  cpu: 2
  timeout: 3600
git:
  auto_commit: true
  branches_allowed: [main, develop, feature/*]
```

---

## Implementation Pivot Plan

### Phase 1 (COMPLETED) ✅
**Status**: Namespace-based prototype validated
**Deliverables**:
- ✅ JailSpec model (model/jail.go)
- ✅ Namespace isolation (mount/namespace.go)
- ✅ Cgroup limits (mount/cgroup.go)
- ✅ Lifecycle management (mount/lifecycle.go)
- ✅ CLI commands (cli/jail.go)
- ✅ E2E tests (test/e2e/jail_test.go)

**Value**: Proved workflow, identified architectural needs

---

### Phase 2 (PIVOT TO KATA) 🎯

**Goal**: Replace namespace backend with Kata runtime via CRI

#### MSB-TSK-2026-008: CRI Integration (NEW)
**Priority**: CRITICAL
**Dependencies**: None
**Deliverable**: `runtime/cri/client.go`

```go
package cri

import (
    "context"
    "google.golang.org/grpc"
    criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type CRIClient struct {
    runtimeClient criapi.RuntimeServiceClient
    imageClient   criapi.ImageServiceClient
}

func NewCRIClient(endpoint string) (*CRIClient, error) {
    conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
    if err != nil {
        return nil, err
    }

    return &CRIClient{
        runtimeClient: criapi.NewRuntimeServiceClient(conn),
        imageClient:   criapi.NewImageServiceClient(conn),
    }, nil
}

func (c *CRIClient) PullImage(ctx context.Context, imageRef string) error
func (c *CRIClient) CreateContainer(ctx context.Context, spec *JailSpec) (string, error)
func (c *CRIClient) StartContainer(ctx context.Context, containerID string) error
func (c *CRIClient) StopContainer(ctx context.Context, containerID string, timeout int64) error
func (c *CRIClient) RemoveContainer(ctx context.Context, containerID string) error
```

**Testing**:
- Unit tests with mock CRI server
- Integration tests with local Kata runtime

---

#### MSB-TSK-2026-009: Runtime Abstraction Layer (NEW)
**Priority**: CRITICAL
**Dependencies**: MSB-TSK-2026-008
**Deliverable**: `runtime/runtime.go`

```go
package runtime

import "github.com/dpopsuev/misbah/model"

// Runtime interface abstracts jail creation
type Runtime interface {
    Create(spec *model.JailSpec) (*Jail, error)
    Start(jail *Jail) error
    Stop(jail *Jail) error
    Destroy(jail *Jail) error
    List() ([]*Jail, error)
    Inspect(jailName string) (*JailInfo, error)
}

// Factory creates runtime based on spec
func NewRuntime(spec *model.JailSpec) (Runtime, error) {
    switch spec.Runtime {
    case "kata":
        return NewKataRuntime()
    case "namespace":
        return NewNamespaceRuntime()
    case "gvisor":
        return NewGVisorRuntime()
    default:
        return nil, fmt.Errorf("unsupported runtime: %s", spec.Runtime)
    }
}
```

**Runtimes**:
- `runtime/kata/runtime.go` - Kata via CRI
- `runtime/namespace/runtime.go` - Legacy namespace (Phase 1 code)
- `runtime/gvisor/runtime.go` - Future (optional)

---

#### MSB-TSK-2026-010: Kata Runtime Implementation (NEW)
**Priority**: CRITICAL
**Dependencies**: MSB-TSK-2026-008, MSB-TSK-2026-009
**Deliverable**: `runtime/kata/runtime.go`

```go
package kata

import (
    "github.com/dpopsuev/misbah/model"
    "github.com/dpopsuev/misbah/runtime/cri"
)

type KataRuntime struct {
    criClient *cri.CRIClient
    logger    *metrics.Logger
}

func NewKataRuntime() (*KataRuntime, error) {
    // Connect to Kata CRI endpoint
    client, err := cri.NewCRIClient("unix:///run/containerd/containerd.sock")
    if err != nil {
        return nil, err
    }

    return &KataRuntime{
        criClient: client,
        logger:    metrics.GetDefaultLogger(),
    }, nil
}

func (kr *KataRuntime) Create(spec *model.JailSpec) (*Jail, error) {
    ctx := context.Background()

    // 1. Pull image if not present
    if err := kr.criClient.PullImage(ctx, spec.Image); err != nil {
        return nil, fmt.Errorf("failed to pull image: %w", err)
    }

    // 2. Build CRI container config
    config := kr.buildContainerConfig(spec)

    // 3. Create container via CRI (Kata creates VM)
    containerID, err := kr.criClient.CreateContainer(ctx, config)
    if err != nil {
        return nil, fmt.Errorf("failed to create container: %w", err)
    }

    return &Jail{
        Name:        spec.Metadata.Name,
        ContainerID: containerID,
        Runtime:     "kata",
        Spec:        spec,
    }, nil
}

func (kr *KataRuntime) buildContainerConfig(spec *model.JailSpec) *criapi.ContainerConfig {
    // Convert JailSpec → CRI ContainerConfig
    // Handle mounts, env vars, resources
}
```

---

#### MSB-TSK-2026-011: Git-Clone Mount Type (NEW)
**Priority**: HIGH
**Dependencies**: MSB-TSK-2026-010
**Deliverable**: `mount/gitclone.go`

```go
package mount

import (
    "os/exec"
    "github.com/dpopsuev/misbah/model"
)

type GitCloner struct {
    logger *metrics.Logger
}

func (gc *GitCloner) Clone(mount *model.MountSpec, destPath string) error {
    // 1. Parse git-clone mount spec
    repo := mount.Repo
    branch := mount.Branch
    credentials := mount.Credentials

    // 2. Setup credentials (SSH key, token)
    if err := gc.setupCredentials(credentials); err != nil {
        return err
    }

    // 3. Execute git clone
    cmd := exec.Command("git", "clone",
        "--depth", "1",
        "--branch", branch,
        repo, destPath)

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("git clone failed: %w", err)
    }

    gc.logger.Infof("Cloned %s to %s", repo, destPath)
    return nil
}
```

**JailSpec Integration**:
```yaml
mounts:
  - type: git-clone
    repo: https://github.com/customer/backend.git
    branch: main
    credentials: github-token-secret
    destination: /workspace/backend
```

---

#### MSB-TSK-2026-012: Image Management CLI (NEW)
**Priority**: MEDIUM
**Dependencies**: MSB-TSK-2026-010
**Deliverable**: `cli/image.go`

```bash
# Pull OCI image from registry
misbah image pull ghcr.io/myorg/agent-base:v1

# List cached images
misbah image list

# Inspect image metadata
misbah image inspect ghcr.io/myorg/agent-base:v1

# Remove unused images
misbah image prune
```

**Implementation**:
```go
func runImagePull(cmd *cobra.Command, args []string) error {
    imageRef := args[0]

    // Use CRI client to pull image
    criClient, err := cri.NewCRIClient(criEndpoint)
    if err != nil {
        return err
    }

    logger.Infof("Pulling image: %s", imageRef)
    if err := criClient.PullImage(context.Background(), imageRef); err != nil {
        return fmt.Errorf("failed to pull image: %w", err)
    }

    logger.Infof("Image pulled successfully: %s", imageRef)
    return nil
}
```

---

#### MSB-TSK-2026-013: Update Lifecycle for Kata (MODIFIED)
**Priority**: HIGH
**Dependencies**: MSB-TSK-2026-009, MSB-TSK-2026-010
**Changes**: Refactor `mount/lifecycle.go` to use runtime abstraction

```go
// OLD (Phase 1)
func (lc *Lifecycle) MountJail(spec *model.JailSpec) error {
    // Direct namespace creation
    if err := lc.namespaceManager.CreateJail(spec, cgroupMgr); err != nil {
        return err
    }
}

// NEW (Phase 2)
func (lc *Lifecycle) MountJail(spec *model.JailSpec) error {
    // Runtime abstraction
    rt, err := runtime.NewRuntime(spec)
    if err != nil {
        return err
    }

    jail, err := rt.Create(spec)
    if err != nil {
        return err
    }

    if err := rt.Start(jail); err != nil {
        return err
    }

    // Wait for jail to complete or timeout
    return lc.waitForJail(jail, spec.Resources.Timeout)
}
```

---

#### MSB-TSK-2026-014: Kata E2E Tests (NEW)
**Priority**: HIGH
**Dependencies**: MSB-TSK-2026-013
**Deliverable**: `test/e2e/kata_test.go`

```go
//go:build e2e

func TestKataLifecycle(t *testing.T) {
    if !kataAvailable() {
        t.Skip("Kata runtime not available")
    }

    spec := &model.JailSpec{
        Runtime: "kata",
        Image:   "docker.io/ubuntu:24.04",
        Metadata: model.JailMetadata{
            Name: "kata-test-" + randomID(),
        },
        Process: model.ProcessSpec{
            Command: []string{"/bin/bash", "-c", "apt-get update && apt-get install -y python3"},
            Cwd:     "/workspace",
        },
        Namespaces: model.NamespaceSpec{
            User: true, Mount: true, PID: true, Network: true,
        },
        Mounts: []model.MountSpec{
            {Type: "tmpfs", Destination: "/workspace"},
        },
    }

    // Create and start jail
    lifecycle := mount.NewLifecycle(logger, recorder)
    err := lifecycle.MountJail(spec)
    require.NoError(t, err)
}

func TestKataGitClone(t *testing.T) {
    // Test git-clone mount type
    spec := &model.JailSpec{
        Runtime: "kata",
        Image:   "ghcr.io/myorg/agent-base:v1",
        Mounts: []model.MountSpec{
            {
                Type:        "git-clone",
                Repo:        "https://github.com/dpopsuev/misbah.git",
                Branch:      "main",
                Destination: "/workspace/misbah",
            },
        },
    }

    // Verify repo cloned successfully
}

func TestKataPackageInstallation(t *testing.T) {
    // Verify agent can install packages inside VM
}

func TestKataIsolation(t *testing.T) {
    // Verify VM isolation (multiple jails can't see each other)
}
```

---

### Phase 3: Djinn Integration

**DJN-TSK-2026-008**: Update Djinn Runtime to use Misbah Kata
**DJN-TSK-2026-009**: Djinnfile → JailSpec conversion with image field
**DJN-TSK-2026-010**: Agent driver integration with Kata jails

---

### Phase 4: OCP Deployment

**MSB-TSK-2026-015**: OCP RuntimeClass configuration (NEW)
**MSB-TSK-2026-016**: Djinn deployment manifests for OCP (NEW)
**MSB-TSK-2026-017**: Multi-tenant isolation testing (NEW)

---

## Migration Path

### Backward Compatibility

**Namespace runtime remains available**:
```yaml
# Old specs still work (default runtime: namespace)
version: "1.0"
metadata:
  name: legacy-jail
# No runtime field = defaults to namespace
```

**Explicit namespace runtime**:
```yaml
runtime: namespace  # Explicitly use Phase 1 implementation
```

### Deprecation Timeline

- **2026-Q2**: Kata becomes default, namespace deprecated
- **2026-Q3**: Namespace runtime marked for removal
- **2026-Q4**: Namespace runtime removed (Kata only)

---

## Testing Strategy

### Unit Tests
- CRI client mocking
- Runtime factory selection
- JailSpec validation with new fields

### Integration Tests
- Local Kata runtime (requires Kata installation)
- Image pull/cache
- Git-clone mounts
- Package installation (apt/pip)

### E2E Tests
- Full jail lifecycle with Kata
- Multi-jail isolation
- Resource limits enforcement
- Network isolation
- OCP deployment (separate test cluster)

### Security Tests
- VM escape attempts (negative testing)
- Kernel exploit simulation
- Multi-tenant isolation verification

---

## Dependencies & Prerequisites

### Local Development
```bash
# Install Kata Containers
sudo apt-get update
sudo apt-get install -y kata-containers

# Verify installation
kata-runtime --version
containerd --version

# Configure containerd for Kata
sudo mkdir -p /etc/containerd/
sudo containerd config default > /etc/containerd/config.toml
# Add Kata runtime to config.toml
```

### OCP Cluster
```bash
# Install Kata Operator
oc create -f https://raw.githubusercontent.com/kata-containers/kata-containers/main/tools/packaging/kata-deploy/kata-deploy/base/kata-deploy.yaml

# Create RuntimeClass
oc apply -f - <<EOF
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: kata
handler: kata
EOF

# Verify Kata available
oc get runtimeclass
```

---

## Success Metrics

### Security
- ✅ Zero successful VM escapes in penetration testing
- ✅ Multi-tenant isolation verified (tenants cannot access each other)
- ✅ Kernel exploits contained within VM

### Performance
- ✅ Jail startup < 300ms (cold start)
- ✅ Jail startup < 100ms (warm start with VM templates)
- ✅ Agent throughput ≥ 80% of native (network/disk)

### Compatibility
- ✅ Same JailSpec works locally and in OCP
- ✅ Namespace runtime backward compatible (Phase 1 specs work)
- ✅ OCI image ecosystem integration (Docker Hub, GHCR, etc.)

### Functionality
- ✅ Agent can install packages (apt, pip, npm)
- ✅ Git clone/commit/push workflows functional
- ✅ Multi-repository support (multiple git-clone mounts)
- ✅ Resource limits enforced (memory, CPU, timeout)

---

## Risks & Mitigations

### Risk 1: Kata Performance Overhead
**Impact**: 200-300ms startup may be too slow
**Mitigation**: VM templates (pre-booted VMs), warm pools

### Risk 2: OCP Kata Support Issues
**Impact**: Kata may have bugs or limitations in OCP
**Mitigation**: Early testing in OCP dev cluster, Red Hat support contract

### Risk 3: CRI API Complexity
**Impact**: CRI integration more complex than direct syscalls
**Mitigation**: Use well-tested CRI libraries, extensive integration tests

### Risk 4: Image Storage/Bandwidth
**Impact**: Large base images consume storage and bandwidth
**Mitigation**: Local image caching, multi-stage builds, layer sharing

---

## Next Steps

1. **Immediate**: Review and approve this pivot plan
2. **Week 1**: Implement MSB-TSK-2026-008 (CRI integration)
3. **Week 2**: Implement MSB-TSK-2026-009/010 (Runtime abstraction + Kata)
4. **Week 3**: Implement MSB-TSK-2026-011/012 (Git-clone + Image CLI)
5. **Week 4**: E2E testing and OCP deployment validation

**Target**: Kata-based Misbah PoC complete by end of month

---

## Approvals

- [ ] Architecture Review
- [ ] Security Review
- [ ] OCP Platform Team
- [ ] Djinn Integration Team

**Document Owner**: dpopsuev
**Last Updated**: 2026-03-14
