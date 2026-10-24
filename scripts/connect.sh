#!/bin/bash
# Connect the gateway's VPN, doing the SAML login in this host's browser.
#
#   scripts/connect.sh ["<profile name>"]
#
# GW_EXEC is how to run a command on the gateway as root, e.g.
#   docker exec -i fct-headless                (default)
#   limactl shell --workdir / forticlient-vpn-headless sudo
#   ssh root@gateway-vm                        (external VM)
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
PROFILE=${1:-${FCT_PROFILE:-}}
[ -n "$PROFILE" ] || { echo "usage: connect.sh \"<profile name>\" (or set FCT_PROFILE)" >&2; exit 2; }
GW_EXEC=${GW_EXEC:-docker exec -i fct-headless}
LOGIN_TIMEOUT=${LOGIN_TIMEOUT:-300}
read -r -a gw <<<"$GW_EXEC"

# Run a command on the gateway. ssh joins its arguments into one remote shell
# command, so quote them there; docker exec / limactl shell pass argv as is.
gw_run() {
    if [ "$(basename "${gw[0]}")" = ssh ]; then
        "${gw[@]}" "$(printf '%q ' "$@")"
    else
        "${gw[@]}" "$@"
    fi
}

out=$(gw_run fct-connect "$PROFILE")
if grep -q '^STATE=connected' <<<"$out"; then
    echo "Already connected."; exit 0
fi
url=$(sed -n 's/^SAML_URL=//p' <<<"$out")
port=$(sed -n 's/^SAML_PORT=//p' <<<"$out")
[ -n "$url" ] && [ -n "$port" ] || { echo "$out" >&2; exit 1; }

python3 "$here/saml-forward.py" "$port" -- "${gw[@]}" socat STDIO "TCP:127.0.0.1:$port" &
fwd=$!
trap 'kill $fwd 2>/dev/null || true' EXIT

echo "SAML login: $url"
echo "(callback localhost:$port is forwarded to the gateway)"
if [ -z "${NO_BROWSER:-}" ]; then
    command -v open >/dev/null && open "$url" || xdg-open "$url" 2>/dev/null || true
fi

deadline=$((SECONDS + LOGIN_TIMEOUT))
until gw_run fct-status >/dev/null 2>&1; do
    if [ $SECONDS -ge $deadline ]; then
        echo "Timed out waiting for the tunnel." >&2
        gw_run fct-status >&2 || true
        exit 1
    fi
    sleep 2
done
gw_run fct-status
