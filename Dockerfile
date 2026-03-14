# Dockerfile for testing misbah in a clean Linux environment
FROM golang:1.22-alpine

# Install dependencies
RUN apk add --no-cache \
    bash \
    util-linux \
    git \
    make

# Enable user namespaces (requires privileged container)
# Note: This is for testing only. In production, the host must enable user namespaces.

WORKDIR /workspace

# Copy source
COPY . .

# Build
RUN make build

# Default command
CMD ["/bin/bash"]
