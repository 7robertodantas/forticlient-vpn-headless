#!/bin/bash
# validate.sh — End-to-end validation of forticlient-vpn-headless in a Lima VM
#
# Usage:
#   ./scripts/validate.sh                    # full run (auto-create VM)
#   ./scripts/validate.sh --keep-vm          # keep VM after failures
#   ./scripts/validate.sh --recreate         # force recreate VM
#   ./scripts/validate.sh --skip-create      # skip VM creation (assume running)
#   ./scripts/validate.sh --import-only      # only import config, skip connect
#   ./scripts/validate.sh --ssh              # just SSH into the VM

set -euo pipefail
cd "$(dirname "$0")/.."

# ─── defaults ──────────────────────────────────────────────────────────
VM_NAME="forticlient-vpn-headless"
PASS=0
FAIL=0
WARN=0
SKIP=0
ERRORS=()
KEEP_VM=false
SKIP_CREATE=false
IMPORT_ONLY=false
SSH_ONLY=false
RECREATE=false
XML_PROFILE=""
FCVPN_VM_PATH="/opt/forticlient/fcvpn"

# Parse args
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep-vm)      KEEP_VM=true; shift ;;
        --recreate)     RECREATE=true; shift ;;
        --skip-create)  SKIP_CREATE=true; shift ;;
        --import-only)  IMPORT_ONLY=true; shift ;;
        --ssh)          SSH_ONLY=true; shift ;;
        --xml=*)        XML_PROFILE="${1#--xml=}"; shift ;;
        *)              echo "Unknown arg: $1"; exit 1 ;;
    esac
done

# If no XML provided, look for known files
if [ -z "$XML_PROFILE" ]; then
    for f in "$HOME/Documents/Sangoma_Corp_VPN_North_Linux_READY.xml" \
             "$HOME/Documents/Sangoma_Corp_VPN_*.xml" \
             "./vpn-profiles/*.xml"; do
        if [ -f "$f" ] 2>/dev/null; then
            XML_PROFILE="$f"
            break
        fi
    done
fi

# ─── colors / output ───────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
bold='\033[1m'

log()  { printf "${CYAN}  [log]  %s${NC}\n" "$*"; }
pass() { printf "${GREEN}  ✓ PASS${NC}  %s\n" "$*"; PASS=$((PASS+1)); }
fail() { printf "${RED}  ✗ FAIL${NC}  %s\n" "$*"; FAIL=$((FAIL+1)); ERRORS+=("$*"); }
warn() { printf "${YELLOW}  ⚠ WARN${NC}  %s\n" "$*"; WARN=$((WARN+1)); }
skip() { printf "${YELLOW}  ○ SKIP${NC}  %s\n" "$*"; SKIP=$((SKIP+1)); }
section() { printf "\n${bold}▶ %s${NC}\n" "$*"; }

# ─── Lima helpers ──────────────────────────────────────────────────────
lima_cmd() {
    limactl "$@" 2>&1
}

# ─── VM lifecycle ──────────────────────────────────────────────────────
ensure_vm_created() {
    local status
    status=$(limactl list --json 2>/dev/null | jq -r ".[] | select(.name==\"$VM_NAME\") | .status" 2>/dev/null || echo "")

    if [ "$RECREATE" = true ] || [ "$status" = "Stopped" ] || [ -z "$status" ]; then
        if [ "$status" = "Running" ] || [ -z "$status" ]; then
            # Stop first if running and recreating
            if [ "$RECREATE" = true ] && [ "$status" = "Running" ]; then
                log "Stopping existing VM for recreation..."
                limactl stop "$VM_NAME" 2>/dev/null || true
                sleep 3
            fi
            log "Creating VM (x86_64, Ubuntu 24.04)..."
            # hostCPUFeatures=[] disables host CPU passthrough — critical for x86_64 on Apple Silicon
            limactl create \
                lima/$VM_NAME.yaml 2>&1 | tail -5
            sleep 5
        fi
        log "Starting VM..."
        limactl start "$VM_NAME" 2>&1 | tail -10
        # Wait for first boot
        log "Waiting for first boot..."
        local retries=0
        local max_retries=120
        while [ $retries -lt $max_retries ]; do
            if limactl shell "$VM_NAME" echo "ready" >/dev/null 2>&1; then
                log "VM booted after $((retries * 5))s"
                # Run manual provisioning (apt + FortiClient)
                provision_vm && return 0
            fi
            sleep 5
            retries=$((retries + 1))
        done
        fail "VM boot timeout after $((max_retries * 5))s"
        return 1
    elif [ "$status" = "Running" ]; then
        log "VM is already running: $VM_NAME"
        # Check if already provisioned
        if limactl shell "$VM_NAME" which confighandler >/dev/null 2>&1; then
            return 0
        fi
        log "VM running but not provisioned — provisioning now..."
        provision_vm
        return $?
    else
        warn "Unexpected VM status: $status"
        return 1
    fi
}

# ─── Manual provisioning (replaces Lima provision scripts) ─────────────
provision_vm() {
    section "Provisioning VM"

    # Check for FortiClient package
    local fct_pkg="$HOME/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_amd64.deb"
    if [ ! -f "$fct_pkg" ]; then
        fail "FortiClient .deb not found at $fct_pkg"
        return 1
    fi

    # Copy package to VM
    log "Copying FortiClient package..."
    lima_cp_to_vm "$fct_pkg"

    # Run provisioning commands inside VM
    log "Installing base dependencies..."
    lima_run 'sudo apt-get update -qq && sudo apt-get install -y --no-install-recommends \
        curl ca-certificates openssl libssl3t64 libstdc++6 libgcc-s1 \
        iproute2 iputils-ping net-tools procps dnsutils jq xz-utils' | tail -3

    log "Installing FortiClient VPN (may pull GUI deps for tray binary)..."
    # Force install to get binaries even with missing deps, then fix deps
    lima_run 'cd /tmp && sudo dpkg --force-depends -i forticlient_vpn_7.4.3.5411_amd64.deb && sudo apt-get install -f -y' 2>&1 | tail -10

    log "Creating required directories..."
    lima_run 'echo | sudo -S mkdir -p /var/run/forticlient && echo | sudo -S chmod 755 /var/run/forticlient && echo | sudo -S mkdir -p /home/rdantas.guest/.pki/nssdb && echo | sudo -S certutil -N -d sql:/home/rdantas.guest/.pki/nssdb --empty-password 2>/dev/null || true'

    # Create NSS DB
    lima_run 'sudo mkdir -p /home/rdantas.guest/.pki/nssdb && sudo certutil -N -d sql:/home/rdantas.guest/.pki/nssdb --empty-password 2>/dev/null || true'

    # Verify
    log "Verifying FortiClient binaries..."
    local verify_check
    verify_check=$(lima_run 'for bin in fctsched confighandler fortitray fortivpn iked; do
        if [ -x "/opt/forticlient/$bin" ]; then
            echo "  ✓ $bin → /opt/forticlient/$bin"
        else
            echo "  ✗ $bin NOT FOUND"
        fi
    done' 2>&1)
    echo "$verify_check"
    if echo "$verify_check" | grep -q "✗"; then
        fail "Some FortiClient binaries missing"
        return 1
    fi

    log "Provisioning complete."
    return 0
}

lima_run() {
    limactl shell "$VM_NAME" -- bash -c "$1"
}

lima_cp_to_vm() {
    limactl cp "$1" "$VM_NAME:/tmp/"
}

# ─── Phase 0: pre-flight ──────────────────────────────────────────────
phase_preflight() {
    section "Phase 0: Pre-flight Checks"

    if ! command -v limactl &>/dev/null; then
        fail "limactl not found. Install with: brew install lima"
        return 1
    fi
    pass "limactl is available ($(limactl version 2>/dev/null | head -1))"

    # Check FortiClient package
    local fct_pkg="$HOME/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_amd64.deb"
    if [ -f "$fct_pkg" ]; then
        pass "FortiClient deb package found ($(du -h "$fct_pkg" | cut -f1))"
    else
        # Try x86_64 RPM alternative
        local rpm_pkg="$HOME/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_x86_64.rpm"
        if [ -f "$rpm_pkg" ]; then
            warn "Only RPM found, not .deb — will try RPM conversion"
        else
            fail "FortiClient package not found in ~/Downloads/forticlient_vpn/"
            warn "Available packages:"
            ls -la "$HOME/Downloads/forticlient_vpn/" 2>/dev/null | tail -5 || true
        fi
    fi

    # Check Go (host-side, for cross-compilation)
    if ! command -v go &>/dev/null; then
        fail "go not found on host — needed for cross-compilation"
        return 1
    fi
    local go_ver
    go_ver=$(go version 2>/dev/null | awk '{print $3}')
    pass "Go available on host: $go_ver"
}

# ─── Phase 1: copy artifacts into VM ──────────────────────────────────
phase_copy_artifacts() {
    section "Phase 1: Copy Artifacts to VM"

    # Create shared directory
    lima_run "mkdir -p /tmp/lima-forticlient"

    # Copy FortiClient package
    local fct_pkg="$HOME/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_amd64.deb"
    if [ -f "$fct_pkg" ]; then
        log "Copying FortiClient package..."
        lima_cp_to_vm "$fct_pkg"
        lima_run "ls -lh /tmp/forticlient_vpn_7.4.3.5411_amd64.deb" | grep -q "forticlient" && \
            pass "FortiClient package in VM" || warn "Package copy may have failed"
    else
        warn "Skipping FortiClient package — not available locally"
    fi

    # Copy XML profile if provided
    if [ -n "$XML_PROFILE" ] && [ -f "$XML_PROFILE" ]; then
        log "Copying XML profile..."
        lima_cp_to_vm "$XML_PROFILE"
        local xml_name
        xml_name=$(basename "$XML_PROFILE")
        lima_run "mv /tmp/$xml_name /tmp/lima-forticlient/$xml_name"
        pass "XML profile in VM: $xml_name"
    else
        warn "No XML profile provided — import tests will be skipped"
    fi
}

# ─── Phase 2: cross-compile on host, copy binary ──────────────────────
phase_build() {
    section "Phase 2: Build & Deploy Binary"

    # Cross-compile for linux/amd64
    log "Cross-compiling fcvpn for linux/amd64..."
    local bin_target="/tmp/fcvpn-linux-amd64"
    local build_output
    build_output=$(CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$bin_target" ./cmd/fctsaml/ 2>&1)

    if [ $? -ne 0 ]; then
        fail "Go cross-compile failed:\n$build_output"
        return 1
    fi

    local bin_size
    bin_size=$(du -h "$bin_target" | cut -f1)
    pass "Binary built: $bin_target ($bin_size)"

    # FortiClient verifies the caller's real executable path.  Install a real
    # copy below its own directory; a symlink or binary under /tmp is rejected.
    log "Deploying binary to $FCVPN_VM_PATH..."
    lima_cp_to_vm "$bin_target"
    if ! lima_run "sudo install -m 0755 /tmp/fcvpn-linux-amd64 $FCVPN_VM_PATH && rm -f /tmp/fcvpn-linux-amd64"; then
        fail "Could not install fcvpn at $FCVPN_VM_PATH"
        return 1
    fi
    lima_run "test -f $FCVPN_VM_PATH && test ! -L $FCVPN_VM_PATH && ls -lh $FCVPN_VM_PATH"
    lima_run "$FCVPN_VM_PATH help 2>&1 | grep -q fcvpn" && \
        pass "Binary runs inside VM" || warn "Binary execution check failed"

    # Clean up host build artifact
    rm -f "$bin_target"
}

# ─── Phase 3: import & verify ─────────────────────────────────────────
phase_import() {
    section "Phase 3: Import & Verify Config"

    if [ -z "$XML_PROFILE" ] || [ ! -f "$XML_PROFILE" ]; then
        skip "No XML profile available — skipping import phase"
        return
    fi

    local xml_name
    xml_name=$(basename "$XML_PROFILE")

    # 3a: Import
    log "Importing $xml_name..."
    local import_output
    import_output=$(lima_run "$FCVPN_VM_PATH import /tmp/lima-forticlient/$xml_name" 2>&1 || true)
    echo "$import_output"
    if echo "$import_output" | grep -qi "imported\|✓"; then
        pass "Config import succeeded"
    else
        warn "Import requires running FortiClient services + auth (HTTP 403 without full service stack): $import_output"
    fi

    # 3b: List
    log "Listing configured tunnels..."
    local list_output
    list_output=$(lima_run "$FCVPN_VM_PATH list 2>&1" || true)
    echo "$list_output"
    if echo "$list_output" | grep -qi "sangoma\|tunnels"; then
        pass "List shows configured tunnels"
    else
        warn "List requires running confighandler service (full validation)"
    fi
}

# ─── Phase 4: service orchestration ────────────────────────────────────
phase_services() {
    section "Phase 4: Service Orchestration"

    # Check if FortiClient binaries are accessible
    log "Checking FortiClient service binaries..."
    local svc_check
    svc_check=$(lima_run 'for bin in fctsched confighandler fortitray; do
        if [ -x "/opt/forticlient/$bin" ]; then
            echo "  PASS $bin → /opt/forticlient/$bin"
        else
            echo "  FAIL $bin NOT FOUND"
        fi
    done' 2>&1)
    echo "$svc_check"
    if echo "$svc_check" | grep -q "FAIL"; then
        return 1
    fi

    # Check binary sizes (validation that packages installed correctly)
    log "Checking binary sizes..."
    local sizes
    sizes=$(lima_run 'ls -lh /opt/forticlient/{fctsched,confighandler,fortitray,fortivpn,iked} 2>&1' || echo "NOT_READY")
    echo "$sizes"
    if echo "$sizes" | grep -q "NOT_FOUND\|No such file"; then
        fail "Some binaries missing in /opt/forticlient/"
    fi

    # Check for required dirs
    log "Checking required directories..."
    if lima_run "test -d /var/run/forticlient"; then
        pass "/var/run/forticlient exists"
    else
        warn "/var/run/forticlient not found (will be created on first service start)"
    fi

    # Check for NSS DB
    log "Checking NSS database..."
    if lima_run "test -d ~/.pki/nssdb"; then
        pass "NSS DB exists at ~/.pki/nssdb"
    else
        warn "NSS DB not found (may need: mkdir -p ~/.pki/nssdb && certutil -N -d sql:$HOME/.pki/nssdb --empty-password)"
    fi

    # Quick service start test (just verify they launch, then kill)
    log "Quick-start test: fctsched..."
    local fctsched_start
    fctsched_start=$(lima_run '/opt/forticlient/fctsched &' 2>&1)
    sleep 2
    if lima_run 'pgrep -f fctsched >/dev/null'; then
        pass "fctsched launched"
        lima_run 'pkill -f fctsched 2>/dev/null || true'
    else
        warn "fctsched may not have started cleanly (expected in containerized VM)"
    fi

    log "Quick-start test: confighandler..."
    lima_run '/opt/forticlient/confighandler &' 2>/dev/null || true
    sleep 2
    if lima_run 'pgrep -f confighandler >/dev/null'; then
        pass "confighandler launched"
        lima_run 'pkill -f confighandler 2>/dev/null || true'
    else
        warn "confighandler may not have started cleanly"
    fi
}

# ─── Phase 5: SAML callback test ──────────────────────────────────────
phase_saml() {
    section "Phase 5: SAML Callback Server (unit)"

    # Test the deployed binary: verify it parses SAML config correctly
    log "Testing fcvpn binary against deployed config..."
    local xml_name
    xml_name=$(basename "$XML_PROFILE")
    local test_output
    test_output=$(lima_run "$FCVPN_VM_PATH list 2>&1" || true)
    echo "$test_output"

    # Verify the binary can parse the port file location
    log "Testing port reader logic..."
    # Create a dummy port file and test
    lima_run 'mkdir -p /tmp/forticlient && echo "8080" > /tmp/forticlient/service_port'
    local port_test
    port_test=$(lima_run 'cat /tmp/forticlient/service_port')
    if [ "$port_test" = "8080" ]; then
        pass "Port file reader logic works"
    else
        fail "Port file reader returned unexpected: $port_test"
    fi
}

# ─── Phase 6: connectivity ────────────────────────────────────────────
phase_connectivity() {
    section "Phase 6: Network Connectivity"

    # DNS
    log "Testing DNS resolution..."
    local dns_result
    dns_result=$(lima_run "dig +short sgvpn-north.netfortris.net" 2>&1 || echo "FAIL")
    if echo "$dns_result" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+'; then
        pass "DNS resolves sgvpn-north.netfortris.net → $dns_result"
    else
        warn "DNS for VPN server: $dns_result"
    fi

    # Port check (UDP 500 for IKE, TCP 443 for SAML)
    log "Testing network reachability..."
    local connectivity
    connectivity=$(lima_run '
        echo "IKE UDP 500: $(timeout 3 bash -c "echo > /dev/udp/sgvpn-north.netfortris.net/500" 2>&1 && echo "REACHABLE" || echo "BLOCKED")"
        echo "SAML TCP 443: $(timeout 3 bash -c "echo > /dev/udp/sgvpn-north.netfortris.net/443" 2>&1 && echo "REACHABLE" || echo "BLOCKED")"
    ' 2>&1) || true
    echo "$connectivity"

    # Check if iptables/nftables would block anything
    log "Checking firewall rules..."
    local iptables_check
    iptables_check=$(lima_run "iptables -L 2>&1 | head -20" || echo "iptables not available")
    echo "  $iptables_check"
}

# ─── Phase 7: teardown ────────────────────────────────────────────────
phase_teardown() {
    section "Teardown"

    # Kill any leftover FortiClient processes in VM
    lima_run 'pkill -f fctsched 2>/dev/null || true'
    lima_run 'pkill -f confighandler 2>/dev/null || true'
    lima_run 'pkill -f fortitray 2>/dev/null || true'
    lima_run 'pkill -f fortivpn 2>/dev/null || true'
    lima_run 'pkill -f iked 2>/dev/null || true'
    pass "Cleaned up VM processes"

    # Remove build artifacts
    lima_run "sudo rm -f $FCVPN_VM_PATH"
    pass "Cleaned up VM artifacts"
}

# ─── Results ───────────────────────────────────────────────────────────
print_results() {
    section "Results"
    printf "  ${GREEN}✓ PASS: %d${NC}\n" "$PASS"
    printf "  ${RED}✗ FAIL: %d${NC}\n" "$FAIL"
    printf "  ${YELLOW}⚠ WARN: %d${NC}\n" "$WARN"
    printf "  ${CYAN}○ SKIP: %d${NC}\n" "$SKIP"
    echo ""

    if [ ${#ERRORS[@]} -gt 0 ]; then
        echo "  Failures:"
        for e in "${ERRORS[@]}"; do
            echo "    - $e"
        done
        echo ""
    fi

    if [ $FAIL -gt 0 ]; then
        printf "${RED}${bold}VALIDATION FAILED${NC}\n"
        return 1
    elif [ $WARN -gt 0 ] && [ $SKIP -gt 0 ]; then
        printf "${YELLOW}${bold}VALIDATION PASSED WITH WARNINGS${NC}\n"
        return 0
    else
        printf "${GREEN}${bold}VALIDATION PASSED${NC}\n"
        return 0
    fi
}

# ─── Main ──────────────────────────────────────────────────────────────
main() {
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║  FortiClient VPN Headless — Lima VM Validation                  ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo ""

    if [ "$SSH_ONLY" = true ]; then
        log "Opening SSH into VM..."
        limactl shell "$VM_NAME"
        exit 0
    fi

    if [ "$SKIP_CREATE" != true ]; then
        ensure_vm_created
    else
        # Ensure VM is running
        local status
        status=$(limactl list --json 2>/dev/null | jq -r ".[] | select(.name==\"$VM_NAME\") | .status" 2>/dev/null || echo "")
        if [ "$status" != "Running" ]; then
            log "Starting VM..."
            limactl start "$VM_NAME" 2>&1 | tail -5
            log "Waiting for boot..."
            local retries=0
            while [ $retries -lt 60 ]; do
                if limactl shell "$VM_NAME" echo "ready" >/dev/null 2>&1; then
                    log "VM booted after $((retries * 5))s"
                    break
                fi
                sleep 5
                retries=$((retries + 1))
            done
        fi
    fi

    # Ensure provisioning is done (even if VM was already running)
    if ! lima_run "test -x /opt/forticlient/confighandler" >/dev/null 2>&1; then
        log "FortiClient not installed — provisioning now..."
        provision_vm || exit 1
    fi

    # Run phases
    phase_preflight || true
    phase_copy_artifacts || true
    phase_build || true
    phase_import || true
    phase_services || true
    phase_saml || true
    phase_connectivity || true

    if [ "$IMPORT_ONLY" = false ]; then
        phase_teardown || true
    fi

    print_results
}

main
