#!/bin/bash
# Create (or recreate) the ephemeral Lima gateway VM.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
vm=forticlient-vpn-headless

limactl delete -f "$vm" 2>/dev/null || true
sed "s|\"{{.Dir}}/../..\"|\"$repo\"|" "$repo/lima/$vm.yaml" > "/tmp/$vm.yaml"
limactl start --tty=false --name "$vm" "/tmp/$vm.yaml"
rm -f "/tmp/$vm.yaml"
limactl shell --workdir / "$vm" sudo fct-list
