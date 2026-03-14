# Installation Guide

## Prerequisites

### System Requirements

- **Operating System**: Linux kernel 3.8 or later
- **Architecture**: x86_64, arm64
- **Unprivileged user namespaces**: Must be enabled

### Check Prerequisites

```bash
# Check kernel version (must be >= 3.8)
uname -r

# Check if user namespaces are enabled
unshare --user --pid echo "User namespaces: OK"

# Verify mount command is available
which mount
```

### Enable User Namespaces (if disabled)

On some distributions, unprivileged user namespaces are disabled by default:

```bash
# Debian/Ubuntu
sudo sysctl -w kernel.unprivileged_userns_clone=1

# Arch/Fedora
sudo sysctl -w user.max_user_namespaces=15000

# Make permanent
echo "kernel.unprivileged_userns_clone = 1" | sudo tee -a /etc/sysctl.conf
```

## Installation Methods

### From Source (Recommended)

```bash
# Clone repository
git clone https://github.com/jabal/jabal.git
cd jabal

# Build and install
make install

# Verify installation
jabal --version
```

### Pre-built Binary

```bash
# Download latest release
curl -L https://github.com/jabal/jabal/releases/latest/download/jabal-linux-amd64 -o jabal

# Make executable
chmod +x jabal

# Move to PATH
sudo mv jabal /usr/local/bin/
```

## Post-Installation

### Shell Completion

```bash
# Bash
jabal completion bash | sudo tee /etc/bash_completion.d/jabal

# Zsh
jabal completion zsh > "${fpath[1]}/_jabal"

# Fish
jabal completion fish > ~/.config/fish/completions/jabal.fish
```

### Configuration Directory

Jabal stores configuration in `~/.config/jabal/`:

```bash
mkdir -p ~/.config/jabal/workspaces
```

## Verification

Test your installation:

```bash
# Create test workspace
jabal create -w test

# Verify manifest was created
ls ~/.config/jabal/workspaces/test/manifest.yaml

# Validate manifest
jabal validate -w test
```

## Troubleshooting

### Error: "user namespaces not available"

User namespaces are disabled. Follow the "Enable User Namespaces" section above.

### Error: "mount: permission denied"

You may need to enable unprivileged bind mounts:

```bash
sudo sysctl -w fs.protected_regular=0
```

### Build Errors

Ensure Go 1.22+ is installed:

```bash
go version  # Should be >= 1.22
```

## Next Steps

See [EXAMPLES.md](EXAMPLES.md) for usage examples.
