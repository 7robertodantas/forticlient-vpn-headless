# Shared helpers for the fct-* gateway scripts. Sourced, not executed.
# Runs as root on the gateway (container or VM).

FCT_DIR=/opt/forticlient
FCT_USER=${FCT_USER:-fct}
PORT_FILE=/var/run/forticlient/service_port
IPSEC_LOG=/var/log/forticlient/ipsecvpn.log

die() { echo "error: $*" >&2; exit 1; }

# forticlient-cli refuses root and needs a per-user runtime dir.
as_user() {
    local uid
    uid=$(id -u "$FCT_USER") || die "user $FCT_USER missing"
    install -d -m 0700 -o "$FCT_USER" -g "$FCT_USER" "/run/user/$uid"
    runuser -u "$FCT_USER" -- env HOME="/home/$FCT_USER" XDG_RUNTIME_DIR="/run/user/$uid" "$@"
}

service_port() {
    local i
    for i in $(seq 1 60); do
        [ -s "$PORT_FILE" ] && { cat "$PORT_FILE"; return 0; }
        sleep 0.5
    done
    die "confighandler did not write $PORT_FILE"
}

# HTTP call to confighandler. Must come from a binary under /opt/forticlient.
api() {
    local method=$1 path=$2; shift 2
    as_user "$FCT_DIR/.headless-curl" -sS --fail-with-body -X "$method" \
        -H 'Content-Type: application/json' "$@" "http://127.0.0.1:$(service_port)$path"
}

# FortiClient logs every state change; the last one is authoritative.
is_connected() {
    [ "$(grep -o 'State: [A-Za-z]*' "$IPSEC_LOG" 2>/dev/null | tail -1)" = "State: Connected" ]
}
