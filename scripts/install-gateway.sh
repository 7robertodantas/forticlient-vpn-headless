#!/bin/bash
# Install the headless gateway on a plain Debian/Ubuntu VM (Lima or external).
# Tested on Ubuntu 24.04 (arm64) and Debian 13 (amd64). Safe to re-run.
#
# Usage (as root): install-gateway.sh <dir>
#   <dir> is a copy of this repo on the VM; only these are used:
#     <dir>/vendor/forticlient_vpn_*_<arch>.deb
#     <dir>/secrets/*.xml          profiles to import
#     <dir>/scripts/gateway/fct-*  helpers, installed to /usr/local/bin
set -euo pipefail
repo=${1:?usage: install-gateway.sh <dir>}
arch=$(dpkg --print-architecture)
deb=$(ls "$repo"/vendor/forticlient_vpn_*_"$arch".deb)
SOCKS_PORT=${SOCKS_PORT:-1080}
export DEBIAN_FRONTEND=noninteractive

# Libraries the non-GUI binaries actually link to, plus helper tools.
runtime=(ca-certificates gzip iptables iproute2 libnss3-tools libglib2.0-0t64
         libsecret-1-0 libudev1 libcap2 libuuid1 libgcrypt20 libstdc++6 zlib1g
         curl jq socat microsocks)

# The package also depends on GTK/X11/Electron libs that only the GUI uses.
# An empty package that "Provides" them keeps apt consistent (upgrades keep
# working) without installing a desktop stack. Installed first, so a previous
# half-finished install cannot leave apt broken.
provides=$(dpkg-deb -f "$deb" Depends | tr ',' '\n' | sed 's/|.*//; s/(.*)//; s/ //g' \
    | grep -vxF -f <(printf '%s\n' "${runtime[@]}" libc6 libglib2.0-0 libgcc1) \
    | sed 's/$/ (= 99:0)/' | paste -sd, - | sed 's/,/, /g')
dummy=$(mktemp -d)
mkdir -p "$dummy/pkg/DEBIAN"
cat > "$dummy/pkg/DEBIAN/control" <<CONTROL
Package: forticlient-headless-deps
Version: 1.0
Architecture: all
Maintainer: forticlient-vpn-headless
Provides: $provides
Description: Satisfies FortiClient GUI-only dependencies on a headless gateway
CONTROL
dpkg-deb -b "$dummy/pkg" "$dummy/forticlient-headless-deps.deb" >/dev/null
dpkg -i "$dummy/forticlient-headless-deps.deb"
rm -rf "$dummy"

apt-get update
apt-get install -y --no-install-recommends "${runtime[@]}"
apt-get install -y --no-install-recommends "$deb"
apt-get check

install -m 0755 /usr/bin/curl /opt/forticlient/.headless-curl
id fct >/dev/null 2>&1 || useradd -m -s /bin/bash fct
# The VPN replaces resolv.conf (systemd-resolved stub) with its own DNS, which
# cannot resolve this host's name; keep it in /etc/hosts (sudo needs it).
grep -qE "^[^#]*[[:space:]]$(hostname)([[:space:]]|$)" /etc/hosts \
    || echo "127.0.1.1 $(hostname)" >> /etc/hosts
install -m 0755 "$repo"/scripts/gateway/fct-* /usr/local/bin/
chmod 0644 /usr/local/bin/fct-lib.sh

# SOCKS5 proxy, unless something (e.g. an existing microsocks) already serves
# the port.
if systemctl is-active -q fct-microsocks.service \
   || ! ss -Hltn "sport = :$SOCKS_PORT" | grep -q .; then
    cat > /etc/systemd/system/fct-microsocks.service <<UNIT
[Unit]
Description=SOCKS5 proxy through the FortiClient VPN
After=network-online.target
[Service]
ExecStart=/usr/bin/microsocks -i 0.0.0.0 -p $SOCKS_PORT
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable --now fct-microsocks.service
else
    echo "port $SOCKS_PORT already in use; not installing fct-microsocks.service"
fi

systemctl enable --now forticlient.service
fct-start
shopt -s nullglob
profiles=("$repo"/secrets/*.xml)
[ ${#profiles[@]} -eq 0 ] || fct-import "${profiles[@]}"
