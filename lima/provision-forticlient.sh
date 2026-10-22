#!/bin/bash
# Lima provisioning — installs FortiClient VPN package
# No GUI dependencies needed; FortiClient .deb pulls what it needs via dpkg.

set -euo pipefail

echo "==> [forticlient] Checking for FortiClient package..."

FCT_PKG="/tmp/lima-forticlient/forticlient_vpn_7.4.3.5411_amd64.deb"

if [ ! -f "$FCT_PKG" ]; then
    echo "FATAL: FortiClient package not found at $FCT_PKG"
    echo "Place forticlient_vpn_7.4.3.5411_amd64.deb in /tmp/lima-forticlient/ on the host."
    exit 1
fi

echo "==> [forticlient] Installing FortiClient VPN $FCT_PKG..."
dpkg -i "$FCT_PKG" || {
    echo "==> [forticlient] Resolving dependency issues..."
    apt-get install -f -y --fix-broken
    dpkg -i "$FCT_PKG"
}

echo "==> [forticlient] Verifying binaries..."
for bin in fctsched confighandler fortitray fortivpn iked; do
    path=$(which "$bin" 2>/dev/null || echo "NOT FOUND")
    if [ "$path" = "NOT FOUND" ]; then
        echo "  ✗ $bin not found in PATH"
    else
        echo "  ✓ $bin → $path"
    fi
done

echo "==> [forticlient] Creating required directories..."
mkdir -p /var/run/forticlient
chmod 755 /var/run/forticlient

# Create NSS DB (needed by FortiClient for TLS cert verification)
if [ ! -d "$HOME/.pki/nssdb" ]; then
    mkdir -p "$HOME/.pki/nssdb"
    certutil -N -d sql:"$HOME/.pki/nssdb" --empty-password 2>/dev/null || true
fi

echo "==> [forticlient] FortiClient provisioning complete."
