# forticlient-vpn-headless

Headless CLI for FortiClient IPsec VPN with SAML authentication. Works on **Linux** and **macOS**.

## Why this exists

The official FortiClient VPN requires a GUI (Electron app) for SAML authentication.
There is **no open-source client** that handles FortiGate IPsec+SAML headless:

- `openfortivpn` only supports SSL VPN (PPP+TLS), not IPsec
- `strongSwan`/`libreswan` handle FortiGate IPsec but not FortiClient's proprietary SAML-over-IKEv2

## The approach

This wraps FortiClient's own binaries — we provide only the headless SAML flow
(external browser + HTTP callback) and CLI interface. The IPsec engine comes from their
own binaries.

### Platform differences

| Linux | macOS |
|---|---|
| `/opt/forticlient/` | `/Library/Application Support/Fortinet/FortiClient/bin/` |
| `/var/run/forticlient/service_port` (dynamic) | `:9080` (bridge service) |
| `fortitray` (fctservice HTTP API) | `fctctld` (Unix socket IPC) |
| `fctsched`, `confighandler` | LaunchDaemons (auto-managed) |
| `xdg-open` | `open -a Safari` |
| `pgrep` for service detection | `lsof`/`pgrep` + bridge port check |

On Linux, fcvpn starts the FortiClient services directly. On macOS, services run as
LaunchDaemons and a bridge service (`fctbridge`) translates WebSocket calls to the
fctctld Unix socket protocol.

### What we provide vs. what FortiClient binaries provide

| We provide | They provide |
|---|---|
| CLI + SAML browser flow | IPsec tunnel (IKEv2 + ESP) |
| SAML HTTP callback server | VPN tunnel controller |
| Service orchestration (Linux) | IKE daemon (`iked`) |
| Config API client | Config DB + auth |
| WebSocket client | Signal/status handling |

## Installation

### Prerequisites

```bash
# 1. Install FortiClient VPN (required — contains the IPsec engine)
# macOS: Download from Fortinet or your org's portal
# Linux: dpkg -i forticlient_vpn_X.X.X_amd64.deb

# 2. Build fcvpn
go build -o /usr/local/bin/fcvpn ./cmd/fcvpn/

# 3. (macOS only) Build the bridge service
go build -o /usr/local/bin/fctbridge ./cmd/fctbridge/
```

### Build from source

```bash
go build -o fcvpn ./cmd/fcvpn/
```

Cross-compile for either platform:

```bash
# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o fcvpn ./cmd/fcvpn/

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o fcvpn ./cmd/fcvpn/

# Linux (x86_64)
GOOS=linux GOARCH=amd64 go build -o fcvpn ./cmd/fcvpn/
```

## Usage

```bash
# Import XML config
fcvpn import ~/MyCorp_Corp_VPN_North_Linux_READY.xml

# List configured tunnels
fcvpn list

# Connect (opens browser for SAML, establishes IPsec tunnel)
fcvpn connect "MyCorp Corp VPN North"

# Disconnect
fcvpn disconnect
```

## Commands

| Command | Description |
|---------|-------------|
| `import <file.xml>` | Parse XML → config DB |
| `list` | List configured tunnels |
| `connect [name]` | Start services → SAML → IPsec tunnel |
| `disconnect [name]` | Terminate tunnel |
| `status` | Service + config status |

## macOS Bridge Service

On macOS, FortiClient doesn't expose an HTTP API. Instead, a bridge service
translates the CLI's HTTP/WebSocket protocol to the fctctld Unix socket protocol.

```bash
# The bridge starts automatically when running fcvpn on macOS.
# To run it manually (useful for debugging):
fctbridge                         # Default port :9080
fctbridge --port 9080             # Custom port
fctbridge --verbose               # Enable debug logging

# Health check
curl http://127.0.0.1:9080/health
```

**Important**: Do NOT run the bridge while the FortiClient GUI app is running —
fctctld can only accept one non-GUI client at a time.

## Architecture

### Linux

```
fcvpn (this CLI)
  ├── Import XML ──► confighandler (SQLite DB)
  ├── Start services ──► fctsched, confighandler, fortitray
  ├── SAML auth ──► HTTP callback server + xdg-open
  ├── Connect ──► WebSocket → fctservice → fortivpn
  └── Tunnel ──► fortivpn + iked (IPsec)
```

### macOS

```
fcvpn (this CLI)
  ├── Import XML ──► fcconfig (data directory)
  ├── SAML auth ──► HTTP callback server + open -a Safari
  ├── Connect ──► WebSocket → fctbridge → fctctld → fct_tunnel_ctl
  └── Tunnel ──► fctctld + racoon/iked (IPsec)

fctbridge (bridge service)
  ├── HTTP API (config management)
  ├── WebSocket API (VPN control)
  └── Translates to → fctctld Unix socket (/var/run/fctvpnctrl.sock)
```

### No GUI Dependencies

No Electron, GTK, Qt, X11, Wayland, or display server. SAML browser delegation
uses platform-native methods (`xdg-open` on Linux, `open` on macOS). Runs fully
headless.

## Project Structure

```
.
├── cmd/fcvpn/main.go     # Shared CLI entry point
├── cmd/fctbridge/main.go # macOS bridge: HTTP/WS → Unix socket
├── fcvpn                 # Build output
├── platforms/            # Platform factory (init_linux.go, init_darwin.go)
├── pkg/
│   ├── platform/         # Platform interface definition
│   │   ├── linux/        # Linux paths & binaries
│   │   └── darwin/       # macOS paths & binaries
│   ├── service/          # Shared service lifecycle
│   │   ├── linux/        # Linux: starts fctsched/confighandler/fortitray
│   │   └── darwin/       # macOS: detects LaunchDaemons
│   ├── config/           # Shared config parsing
│   │   ├── linux/        # Linux: HTTP API to confighandler
│   │   └── darwin/       # macOS: fcconfig binary invocation
│   ├── ws/               # Shared WebSocket client
│   │   ├── linux/        # Linux: dynamic port from file
│   │   └── darwin/       # macOS: fixed bridge port
│   ├── saml/             # SAML HTTP callback server (shared)
│   └── xml/              # XML config parser (shared)
├── constants.go          # Event types & error types (shared)
├── go.mod
└── README.md
```

## Related

- [FortiClient IPsec Headless VPN Docs](docs/forticlient-ipsec-headless-vpn.md)
- [Binary REA Report](docs/forticlient-ipsec-re-engineering.md)
