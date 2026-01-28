#!/usr/bin/env bash
# oc-commatrix - runs commatrix CLI in a container
# Works on Linux and macOS (ARM/AMD64)

set -e

# Default image (injected at build time by inject.sh)
img=""

# Find container engine
ENGINE=$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)
if [ -z "$ENGINE" ]; then
    echo "Error: podman or docker required" >&2
    exit 1
fi

# Priority: 1) COMMATRIX_IMAGE env var, 2) injected default, 3) local image search
if [ -n "$COMMATRIX_IMAGE" ]; then
    img="$COMMATRIX_IMAGE"
elif [ -z "$img" ]; then
    # No injected default, try to find local image
    LOCAL_IMG=$("$ENGINE" images --format "{{.Repository}}:{{.Tag}}" 2>/dev/null | grep -E "(^|/)commatrix:" | head -n1)
    if [ -n "$LOCAL_IMG" ]; then
        img="$LOCAL_IMG"
    fi
fi

if [ -z "$img" ]; then
    echo "Error: No commatrix image found." >&2
    echo "  Option 1: Build locally - podman build -t commatrix:4.21 ." >&2
    echo "  Option 2: Set COMMATRIX_IMAGE=<registry-url>" >&2
    exit 1
fi

# Run containerized CLI with kubeconfig mounted
exec "$ENGINE" run --rm -it \
    -v "${KUBECONFIG:-$HOME/.kube/config}:/kubeconfig:ro" \
    -e KUBECONFIG=/kubeconfig \
    "$img" "$@"
