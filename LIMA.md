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
│  │  ── forwarded to :18020     │          │
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
| SAML port    | 8020 (forwarded to 18020) |

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

Port 8020 (guest) → 18020 (host) is configured in the YAML. If 18020 is
already in use on the host:

```bash
# Edit the YAML and change both ports
# Then recreate: make validate-recreate
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

Full SAML authentication requires user interaction via a browser. The validation
tests the SAML server binding; actual SAML testing requires a real SAML IdP or
a local mock IdP.
