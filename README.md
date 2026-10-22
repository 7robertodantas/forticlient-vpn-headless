# forticlient-vpn-headless

Headless CLI for FortiClient IPsec VPN with SAML authentication.

## Why this exists

The official FortiClient VPN for Linux requires a GUI (Electron app) for SAML authentication.
There is **no open-source Linux client** that handles FortiGate IPsec+SAML:

- `openfortivpn` only supports SSL VPN (PPP+TLS), not IPsec
- `strongSwan`/`libreswan` handle FortiGate IPsec but not FortiClient's proprietary SAML-over-IKEv2

## The approach

This wraps FortiClient's own binaries — we provide only the headless SAML flow
(external browser + HTTP callback) and CLI interface. The IPsec engine comes from their
own `fortivpn` + `iked` binaries.

### What we provide vs. what FortiClient binaries provide

| We provide | They provide |
|---|---|
| CLI + SAML browser flow | IPsec tunnel (IKEv2 + ESP) |
| SAML HTTP callback server | VPN daemon (`fortivpn`) |
| Service orchestration | IKE daemon (`iked`) |
| Config API client | Config DB + auth |
| WebSocket client | Signal/status handling |

## Installation

### Prerequisites (on Linux gateway)

```bash
# 1. Install FortiClient VPN (required — contains the IPsec engine)
dpkg -i forticlient_vpn_7.4.3.5411_amd64.deb

# 2. Build fcvpn
go build -o /usr/local/bin/fcvpn ./cmd/fctsaml/
```

### Build from source

```bash
go build -o fcvpn ./cmd/fctsaml/
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
| `import <file.xml>` | Parse XML → confighandler SQLite |
| `list` | List configured tunnels |
| `connect [name]` | Start services → SAML → IPsec tunnel |
| `disconnect [name]` | Terminate tunnel |
| `status` | Service + config status |

## Architecture

```
fcvpn (this CLI)
  ├── Import XML ──► confighandler (SQLite DB)
  ├── Start services ──► fctsched, confighandler, fortitray
  ├── SAML auth ──► HTTP callback server + xdg-open
  ├── Connect ──► WebSocket → fctservice → fortivpn
  └── Tunnel ──► fortivpn + iked (IPsec)
```

### No GUI Dependencies

No Electron, GTK, Qt, X11, Wayland, or display server. SAML browser delegation
uses `xdg-open` (with chromium/firefox fallback). Runs fully headless.

## Project Structure

```
.
├── cmd/fctsaml/main.go    # CLI entry point
├── fcvpn                  # Build output
├── pkg/
│   ├── config/handler.go  # Confighandler HTTP API
│   ├── saml/server.go     # SAML HTTP callback server
│   ├── service/manager.go # Service lifecycle
│   ├── ws/client.go       # WebSocket client for fctservice
│   └── xml/config.go      # XML config parser
├── constants.go           # Constants, port reader
├── go.mod
├── go.sum
└── README.md
```

## Related

- [FortiClient IPsec Headless VPN Docs](docs/forticlient-ipsec-headless-vpn.md)
- [Binary REA Report](docs/forticlient-ipsec-re-engineering.md)
