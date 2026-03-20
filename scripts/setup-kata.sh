#!/bin/bash
set -euo pipefail

# Misbah Kata Backend Setup
#
# This script configures the host for running Misbah with the Kata backend.
# Run as root or with sudo. Tested on Fedora 43.
#
# What it does:
#   1. Verifies prerequisites (containerd, kata, KVM)
#   2. Configures containerd with the Kata runtime handler
#   3. Configures CNI networking
#   4. Configures Kata for no-network mode (agents use proxy, not direct network)
#   5. Sets up the misbah group and daemon socket permissions
#   6. Installs the systemd unit and binaries
#
# Usage:
#   make build
#   sudo ./scripts/setup-kata.sh

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ok()   { echo -e "${GREEN}[OK]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; exit 1; }

# Must be root
[[ $EUID -eq 0 ]] || fail "Run as root: sudo $0"

echo "=== Misbah Kata Backend Setup ==="
echo

# --- Prerequisites ---

echo "Checking prerequisites..."

[[ -c /dev/kvm ]] || fail "/dev/kvm not found. Enable KVM in BIOS."
ok "KVM available"

command -v containerd >/dev/null || fail "containerd not installed. Install with: dnf install containerd"
systemctl is-active --quiet containerd || fail "containerd not running. Start with: systemctl start containerd"
ok "containerd running"

command -v kata-runtime >/dev/null || fail "kata-runtime not installed. Install with: dnf install kata-containers"
ok "Kata $(kata-runtime --version 2>&1 | head -1 | awk '{print $NF}')"

[[ -f bin/misbah ]] || fail "bin/misbah not found. Run: make build"
ok "Binaries built"

# --- CNI plugins ---

echo
echo "Configuring CNI..."

if [[ -d /usr/libexec/cni ]]; then
    # Fedora puts CNI plugins in /usr/libexec/cni, containerd expects /opt/cni/bin
    CONTAINERD_CNI_DIR=$(containerd config dump 2>/dev/null | grep -oP "bin_dirs = \['\K[^']+")
    if [[ "$CONTAINERD_CNI_DIR" == "/usr/libexec/cni" ]]; then
        ok "containerd CNI bin_dirs already set to /usr/libexec/cni"
    else
        warn "containerd may need bin_dirs configured for /usr/libexec/cni in config.toml"
    fi
else
    warn "CNI plugins not found at /usr/libexec/cni. Install with: dnf install containernetworking-plugins"
fi

if [[ ! -f /etc/cni/net.d/10-misbah.conflist ]]; then
    mkdir -p /etc/cni/net.d
    cat > /etc/cni/net.d/10-misbah.conflist << 'EOF'
{
  "cniVersion": "1.0.0",
  "name": "misbah",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "misbah0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16",
        "routes": [{ "dst": "0.0.0.0/0" }]
      }
    },
    { "type": "portmap", "capabilities": { "portMappings": true } },
    { "type": "loopback" }
  ]
}
EOF
    ok "CNI config created at /etc/cni/net.d/10-misbah.conflist"
else
    ok "CNI config already exists"
fi

# --- Kata configuration ---

echo
echo "Configuring Kata..."

KATA_CONFIG="/etc/kata-containers/configuration.toml"
if [[ -f "$KATA_CONFIG" ]]; then
    # Set internetworking_model to none (agents use proxy, not direct network)
    if grep -q 'internetworking_model = "none"' "$KATA_CONFIG"; then
        ok "Kata internetworking_model already set to none"
    else
        sed -i 's/^internetworking_model = .*/internetworking_model = "none"/' "$KATA_CONFIG"
        ok "Set Kata internetworking_model = none"
    fi

    # Set disable_new_netns to true (required for internetworking_model=none)
    if grep -q 'disable_new_netns = true' "$KATA_CONFIG"; then
        ok "Kata disable_new_netns already true"
    else
        sed -i 's/^disable_new_netns = .*/disable_new_netns = true/' "$KATA_CONFIG"
        ok "Set Kata disable_new_netns = true"
    fi
else
    warn "Kata config not found at $KATA_CONFIG"
fi

# --- containerd Kata runtime ---

echo
echo "Checking containerd Kata runtime..."

if containerd config dump 2>/dev/null | grep -q 'runtimes.kata'; then
    ok "Kata runtime registered in containerd"
else
    warn "Kata runtime not found in containerd config. Add to /etc/containerd/config.toml:"
    echo "  [plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.kata]"
    echo "    runtime_type = \"io.containerd.kata.v2\""
fi

# --- misbah group ---

echo
echo "Setting up misbah group..."

if getent group misbah >/dev/null 2>&1; then
    ok "Group 'misbah' exists"
else
    groupadd misbah
    ok "Created group 'misbah'"
fi

# Add invoking user to misbah group (the user who ran sudo)
REAL_USER="${SUDO_USER:-}"
if [[ -n "$REAL_USER" ]]; then
    if id -nG "$REAL_USER" | grep -qw misbah; then
        ok "$REAL_USER already in misbah group"
    else
        usermod -aG misbah "$REAL_USER"
        ok "Added $REAL_USER to misbah group (re-login or use 'sg misbah' to activate)"
    fi
fi

# --- Install binaries ---

echo
echo "Installing binaries..."

cp bin/misbah /usr/local/bin/misbah
ok "Installed /usr/local/bin/misbah"

# --- Systemd unit ---

echo
echo "Installing systemd unit..."

cp assets/misbah-daemon.service /etc/systemd/system/misbah-daemon.service
systemctl daemon-reload
ok "Installed misbah-daemon.service"

if systemctl is-active --quiet misbah-daemon; then
    systemctl restart misbah-daemon
    ok "Restarted misbah-daemon"
else
    systemctl start misbah-daemon
    ok "Started misbah-daemon"
fi

systemctl enable misbah-daemon 2>/dev/null
ok "Enabled misbah-daemon on boot"

# --- Verify ---

echo
echo "=== Verification ==="

sleep 1

if systemctl is-active --quiet misbah-daemon; then
    ok "Daemon running"
else
    fail "Daemon not running"
fi

SOCK="/run/misbah/permission.sock"
if [[ -S "$SOCK" ]]; then
    SOCK_GROUP=$(stat -c '%G' "$SOCK")
    SOCK_PERMS=$(stat -c '%a' "$SOCK")
    ok "Socket $SOCK ($SOCK_GROUP:$SOCK_PERMS)"
else
    warn "Socket not found at $SOCK"
fi

echo
echo "=== Setup complete ==="
echo
echo "Usage:"
echo "  sg misbah -c 'misbah container start --spec container.yaml --runtime kata'"
echo
echo "Or re-login to activate the misbah group, then:"
echo "  misbah container start --spec container.yaml --runtime kata"
