#!/bin/bash
# Lima provisioning (copy for Lima's provision directory)

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
