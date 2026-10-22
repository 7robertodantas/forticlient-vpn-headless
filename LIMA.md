# Lima VM Validation

Ephemeral x86_64 Linux VM for reproducing and validating the FortiClient VPN
headless workflow. No GUI, no X11, no display server — pure binary orchestration.

**Build on host, run in VM.** The Go binary is cross-compiled on macOS
(`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`) and copied into the VM.
The VM only needs: FortiClient .deb + minimal base libraries.

## Architecture

```
┌─────────────── macOS Host ────────────────┐
│                                           │
│  CGO_ENABLED=0 GOOS=linux GOARCH=amd64    │
│  go build -o fcvpn                        │
│  ┌─────────────────────────────┐          │
│  │  fcvpn import               │          │
│  │  fcvpn list                 │          │
│  │  fcvpn connect  ──► SAML    │          │
│  │  fcvpn disconnect           │          │
│  └──────────┬──────────────────┘          │
│             │ scp to VM                   │
│  ┌──────────▼──────────────────┐          │
│  │  Lima VM (x86_64, Ubuntu)   │          │
│  │                             │          │
│  │  FortiClient daemons:       │          │
│  │    fctsched  ── scheduler   │          │
│  │    confighandler ── HTTP DB │          │
│  │    fortivpn  ── IPsec ESP   │          │
│  │    iked      ── IKEv2 NRM   │          │
│  │                             │          │
│  │  SAML callback port 8020    │          │
│  │  ── forwarded to host :8020 │          │
│  └─────────────────────────────┘          │
│                                           │
└───────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

```bash
brew install lima
```

### One-command validation

```bash
# Stage artifacts → full VM lifecycle → teardown
make validate
```

### Step by step

```bash
# 1. Stage the FortiClient .deb and VPN XML profile
make provision

# 2. Create and start the VM (first run downloads Ubuntu image, ~2-3 min)
make validate

# 3. Inspect the VM while it's running
make validate-ssh

# 4. Clean up when done
make validate-cleanup
```

### Other targets

```bash
make validate-quick            # Reuse existing VM, skip creation
make validate-recreate         # Force destroy + recreate
make validate-ssh              # SSH into the running VM
make validate-cleanup          # Delete VM and shared artifacts
make list                      # List Lima VMs
make check-deps                # Verify prerequisites
```

## VM Specs

| Parameter    | Value                    |
|------------- |--------------------------|
| Arch         | x86_64 (emulated on ARM) |
| OS           | Ubuntu 24.04 LTS         |
| vCPUs        | 2                        |
| Memory       | 1 GiB                    |
| Disk         | 15 GiB                   |
| GUI          | None (server-only)       |
| Go runtime   | Not installed — binary built on host |
| SAML port    | 8020 (auto-forwarded to host 127.0.0.1:8020) |

## Validation Phases

The `scripts/validate.sh` runner executes these phases sequentially:

| #  | Phase          | What it validates                                              |
|----|----------------|----------------------------------------------------------------|
| 0  | Pre-flight     | limactl, .deb package, Go (host), XML profile                  |
| 1  | Copy artifacts | .deb, XML → VM                                                  |
| 2  | Build          | `CGO_ENABLED=0 go build linux/amd64` on host, copy binary in   |
| 3  | Import         | `fcvpn import` with the Sangoma XML profile                    |
| 4  | Services       | FortiClient daemons launch (fctsched, confighandler, fortitray)|
| 5  | SAML           | SAML callback HTTP server binds port 8020                       |
| 6  | Connectivity   | DNS + port reachability for sgvpn-north.netfortris.net          |

## Directory Layout

```
.
├── Makefile                          # Lifecycle orchestrator
├── lima/
│   ├── forticlient-vpn-headless.yaml # VM definition
│   ├── provision-system.sh           # Base deps only (no GUI, no Go)
│   └── provision-forticlient.sh      # .deb install + dirs
├── scripts/
│   └── validate.sh                   # 7-phase runner
└── LIMA.md                           # This file
```

## Shared Directory

Host `→` VM file bridge via Lima mount:

```
Host:  /tmp/lima-forticlient/
       ├─ forticlient_vpn_7.4.3.5411_amd64.deb
       └─ Sangoma_Corp_VPN_North_Linux_READY.xml

VM:    /tmp/lima-forticlient/   (same files, auto-mounted)
```

Set it up with: `make provision`

## X86_64 on Apple Silicon

QEMU on Apple Silicon can't emulate certain x86 CPU instructions (F16C,
VCORE) because the host ARM chips don't have them. The create command uses
`--set=hostCPUFeatures=[]` to strip host feature passthrough, forcing QEMU
to emulate everything in software.

This adds a few seconds to VM boot but prevents crashes. No other action needed.

## Troubleshooting

### VM won't start

```bash
# Check Lima logs
limactl shell forticlient-vpn-headless journalctl -b --no-pager | tail -50

# Delete and recreate
make validate-recreate
```

### FortiClient package missing

```bash
# Re-stage
make provision

# Verify
ls -lh /tmp/lima-forticlient/
```

### Binary won't run inside VM

```bash
# SSH in and check missing libs
make validate-ssh
ldd /tmp/fcvpn | grep "not found"
```

### SAML callback port conflict

Lima auto-forwards guest port 8020 to the same port on the host, and the
FortiGate redirects the browser to `127.0.0.1:8020`, so both sides must use
the same port. If 8020 is already in use on the host, Lima can't forward it:

```bash
lsof -nP -iTCP:8020 -sTCP:LISTEN   # find and stop whatever holds it
```

### Import/List return HTTP 403

`fcvpn import` and `fcvpn list` require the FortiClient service stack to be
running with proper authentication. The confighandler enforces internal auth
that requires the full FortiClient electron app or at minimum the fortitray
tray daemon with its GUI dependencies.

For a minimal test, you can manually start the headless services:

```bash
make validate-ssh
# Inside VM:
sudo /opt/forticlient/fctsched &
sudo /opt/forticlient/confighandler &
cat /var/run/forticlient/service_port  # note the port
/tmp/fcvpn import ~/Sangoma_Corp_VPN_North_Linux_READY.xml
```

### Full SAML flow

`make validate` only checks that the SAML server binds. To run a real
connection with the browser on the host, see
[Testing the Full Connect Flow](#testing-the-full-connect-flow).

## Testing the Full Connect Flow

Run `fcvpn connect` inside the running VM and complete SAML login in your
macOS browser. The tunnel comes up inside the VM, not on the host.

```
VM:   fcvpn connect ──► callback server on 0.0.0.0:8020
                    └─► xdg-open shim writes URL to /tmp/lima-forticlient/saml-url
Host: watcher opens URL in macOS browser ──► IdP login
      FortiGate redirects to http://127.0.0.1:8020/remote/saml/auth_id?id=...
      Lima forwards host :8020 ──► VM :8020 ──► fcvpn ──► IPsec tunnel
```

### 1. Build the binary into the shared directory

The VM mounts `/tmp/lima-forticlient` at the same path, so there's no copy step.
`make validate` deletes `/tmp/fcvpn` during cleanup, so build it again:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/lima-forticlient/fcvpn ./cmd/fctsaml/
```

### 2. Install an `xdg-open` shim in the VM

The VM has no browser, and `fcvpn connect` aborts if it can't launch one. The
shim hands the URL to the host through the shared directory:

```bash
limactl shell forticlient-vpn-headless -- sudo tee /usr/local/bin/xdg-open >/dev/null <<'EOF'
#!/bin/sh
echo "$1" > /tmp/lima-forticlient/saml-url
echo "SAML URL handed to host: $1" >&2
EOF
limactl shell forticlient-vpn-headless -- sudo chmod +x /usr/local/bin/xdg-open
```

### 3. Start the URL watcher on the host (terminal A)

```bash
rm -f /tmp/lima-forticlient/saml-url
while :; do
  if [ -s /tmp/lima-forticlient/saml-url ]; then
    open "$(cat /tmp/lima-forticlient/saml-url)"
    rm -f /tmp/lima-forticlient/saml-url
  fi
  sleep 1
done
```

If you'd rather skip the watcher, copy the `Opening browser to: ...` URL that
`fcvpn connect` prints and paste it into your browser. You still need the shim.

### 4. Import and connect inside the VM (terminal B)

```bash
make validate-ssh

# Inside VM:
sudo /tmp/lima-forticlient/fcvpn import /tmp/lima-forticlient/Sangoma_Corp_VPN_North_Linux_READY.xml
sudo /tmp/lima-forticlient/fcvpn list
sudo /tmp/lima-forticlient/fcvpn connect "<tunnel name from list>"
```

`sudo` is required because `fortivpn` and `iked` create the tunnel interface and
routes. Ubuntu's sudo `secure_path` includes `/usr/local/bin`, so the shim
still gets picked up.

### 5. Log in from the host browser

The watcher opens the FortiGate SAML page. After you log in, the browser
redirects to `http://127.0.0.1:8020/...` and shows **VPN Connected**. In
terminal B you should see:

```
  ✓ SAML authentication successful!
Connection request sent. Waiting for status...
✓ VPN connected successfully!
```

You have 300 seconds to log in by default (`xauth_timeout` in the XML).

While the browser is waiting, you can check from the host that Lima is
forwarding the callback port:

```bash
lsof -nP -iTCP:8020 -sTCP:LISTEN   # should show a limactl/ssh process
```

### 6. Verify the tunnel inside the VM

```bash
pgrep -a 'fortivpn|iked'
ip addr          # look for the new tunnel interface / VPN address
ip route         # corporate subnets routed through the tunnel
ping -c3 <internal host>
```

### 7. Disconnect and clean up

```bash
sudo /tmp/lima-forticlient/fcvpn disconnect
sudo rm -f /usr/local/bin/xdg-open   # optional
```

Then stop the watcher with Ctrl-C in terminal A.

### Connect-flow troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `no browser found` | Shim missing or not executable — redo step 2 |
| Browser shows "connection refused" on `127.0.0.1:8020` | `fcvpn connect` already exited, or host port 8020 is taken (`lsof -nP -iTCP:8020`) |
| Browser opens nothing | Watcher not running, or `saml-url` was written before the watcher started — paste the printed URL by hand |
| `SAML authentication failed (state: 6)` | Callback arrived without an `id` — login didn't finish on the IdP side |
| SAML succeeds but the tunnel never connects | Check `journalctl` and `/var/log/forticlient/` inside the VM; ESP/UDP 500/4500 may be blocked by QEMU user networking |
