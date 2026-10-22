#!/bin/bash
# Lima provisioning — minimal base (no GUI, no X11, no Go)
# Go is cross-compiled on the host, binary is copied in.

set -euo pipefail

echo "==> [provision] Updating package lists..."
apt-get update -qq

echo "==> [provision] Installing base dependencies..."
apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    openssl \
    libssl3t64 \
    libstdc++6 \
    libgcc-s1 \
    iproute2 \
    iputils-ping \
    net-tools \
    procps \
    pgrep \
    dnsutils \
    jq \
    xz-utils

echo "==> [provision] Base provisioning complete."
