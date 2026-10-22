# FortiClient `fortitray` (fctservice) — WebSocket Protocol Handoff

## Overview

`fortitray` (also called **fctservice**) is the FortiClient system tray daemon that exposes a **WebSocket API** for VPN management. It is the only FortiClient binary that requires GUI libraries (`libgtk`, `libgdk`).

The Go-based `fcvpn` CLI replaces `fortitray` with a headless stub that implements the same WebSocket protocol, eliminating the GUI dependency.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  fctsched (scheduler)                           │
│  - Spawns confighandler, fortitray, firewall    │
│  - No GUI dependency                            │
└─────────────────────────────────────────────────┘
         │
         │ (forks)
         ▼
┌─────────────────────────────────────────────────┐
│  confighandler (HTTP)                           │
│  - Port: dynamic, written to                    │
│    /var/run/forticlient/service_port            │
│  - Endpoints: /config, /vpn, /vpn/state         │
│  - No GUI dependency                            │
└─────────────────────────────────────────────────┘
         │
         │ (same port)
         ▼
┌─────────────────────────────────────────────────┐
│  fortitray / fctservice (WebSocket)             │
│  - Endpoint: ws://127.0.0.1:{port}/websocket    │
│  - Messages: JSON with "type" field             │
│  - REQUIRES: libgtk, libgdk, libnotify          │
│  - Headless stub: replaces this binary          │
└─────────────────────────────────────────────────┘
```

## Service Discovery

The WebSocket server listens on the **same port** as `confighandler`. The port is written to:

```
/var/run/forticlient/service_port
```

The stub reads this file to determine the port:

```go
port, _ := os.ReadFile("/var/run/forticlient/service_port")
```

## WebSocket Protocol

### Connection

```
GET /websocket
Upgrade: websocket
Connection: Upgrade
```

The server accepts the WebSocket upgrade and establishes a bidirectional JSON communication channel.

### Message Format

All messages are **JSON** with a `type` field:

**Client → Server (requests):**
```json
{
  "type": "FCT_VPN_CONNECT",
  "msg": {
    "connection_name": "MyVPN",
    "success": true
  }
}
```

**Server → Client (responses):**
```json
{
  "type": "FCT_VPN_CONNECTED",
  "msg": { ... }
}
```

### Message Types (Constants)

| Constant | Value | Direction | Description |
|----------|-------|-----------|-------------|
| `FCT_VPN_CONNECT` | `"FCT_VPN_CONNECT"` | Client → Server | Request to connect to a VPN tunnel |
| `FCT_VPN_DISCONNECT` | `"FCT_VPN_DISCONNECT"` | Client → Server | Request to disconnect |
| `FCT_VPN_CANCEL` | `"FCT_VPN_CANCEL"` | Client → Server | Cancel current operation |
| `FCT_VPN_CONNECTED` | `"FCT_VPN_CONNECTED"` | Server → Client | VPN connected successfully |
| `FCT_VPN_CONNECTING` | `"FCT_VPN_CONNECTING"` | Server → Client | Connecting in progress |
| `FCT_VPN_RECONNECTING` | `"FCT_VPN_RECONNECTING"` | Server → Client | Reconnecting in progress |
| `FCT_VPN_DISCONNECTED` | `"FCT_VPN_DISCONNECTED"` | Server → Client | VPN disconnected |
| `FCT_VPN_SAML_RESPONSE` | `"FCT_VPN_SAML_RESPONSE"` | Client → Server | SAML authentication result |
| `FCT_VPN_XAUTH` | `"FCT_VPN_XAUTH"` | Server → Client | XAuth challenge |
| `FCT_VPN_XAUTH_TOKEN` | `"FCT_VPN_XAUTH_TOKEN"` | Server → Client | XAuth OTP token |
| `FCT_VPN_XAUTH_RSA` | `"FCT_VPN_XAUTH_RSA"` | Server → Client | XAuth RSA token |
| `FCT_VPN_XAUTH_NEW_PWD` | `"FCT_VPN_XAUTH_NEW_PWD"` | Server → Client | XAuth new password |
| `FCT_VPN_XAUTH_CREDENTIALS` | `"FCT_VPN_XAUTH_CREDENTIALS"` | Server → Client | XAuth credentials |
| `FCT_VPN_DISCLAIMER` | `"FCT_VPN_DISCLAIMER"` | Server → Client | Disclaimer message |
| `FCT_VPN_CONFIRMATION` | `"FCT_VPN_CONFIRMATION"` | Server → Client | Confirmation message |
| `FCT_VPN_INVALID_CERTIFICATE` | `"FCT_VPN_INVALID_CERTIFICATE"` | Server → Client | Invalid certificate |
| `FCT_VPN_LIMIT_USER` | `"FCT_VPN_LIMIT_USER"` | Server → Client | User limit reached |
| `FCT_DH_KEY_EXCHANGE` | `"FCT_DH_KEY_EXCHANGE"` | Server → Client | DH key exchange |

### SAML States

| Constant | Value |
|----------|-------|
| `SAMLStateUnknown` | `0` |
| `SAMLStateSuccess` | `1` |
| `SAMLStateTimeout` | `2` |
| `SAMLStateCancelled` | `3` |
| `SAMLStateClose` | `4` |
| `SAMLStateEmptyResponse` | `5` |
| `SAMLStateRedirectUnavailable` | `6` |

### VPN States

| Constant | Value |
|----------|-------|
| `VPNStateDisconnected` | `0` |
| `VPNStateConnecting` | `1` |
| `VPNStateConnected` | `2` |
| `VPNStateUnknown` | `3` |
| `VPNStateReconnecting` | `4` |
| `VPNStateXAuth` | `5` |

## Connect Flow

1. Client sends `FCT_VPN_CONNECT` with `connection_name`
2. Server responds with `FCT_VPN_CONNECTING`
3. Server responds with `FCT_VPN_CONNECTED` (success) or `FCT_VPN_DISCONNECTED` (failure)
4. Client sends `FCT_VPN_SAML_RESPONSE` with `success: true` for SAML authentication
5. Server responds with `FCT_VPN_CONNECTED`

## Disconnect Flow

1. Client sends `FCT_VPN_DISCONNECT`
2. Server responds with `FCT_VPN_DISCONNECTED`

## Cancel Flow

1. Client sends `FCT_VPN_CANCEL`
2. Server responds with `FCT_VPN_DISCONNECTED`

## Headless Stub Implementation

The Go-based `fortitray` stub:

1. **Reads the service port** from `/var/run/forticlient/service_port` (same as confighandler)
2. **Starts a WebSocket server** on the same port at `/websocket`
3. **Handles messages** by type:
   - `FCT_VPN_CONNECT` → responds with `FCT_VPN_CONNECTING` → `FCT_VPN_CONNECTED`
   - `FCT_VPN_SAML_RESPONSE` → acknowledges
   - `FCT_VPN_DISCONNECT` → responds with `FCT_VPN_DISCONNECTED`
   - `FCT_VPN_CANCEL` → responds with `FCT_VPN_DISCONNECTED`
4. **No GUI libraries required** — pure Go WebSocket server

### Files

| File | Purpose |
|------|---------|
| `pkg/fctservice/stub.go` | WebSocket server implementation |
| `pkg/fctservice/stub_test.go` | Unit tests |

## Comparison: Real fortitray vs. Headless Stub

| Feature | Real fortitray | Headless stub |
|---------|---------------|---------------|
| GUI libraries | GTK, GDK, notify | None |
| Display server | Required | Not needed |
| WebSocket API | Full (all message types) | Partial (connect/disconnect/saml/cancel) |
| IPsec tunnel | Actual IPsec (fortivpn + iked) | Delegates to confighandler |
| Process | /opt/forticlient/fortitray | /opt/forticlient/fortitray (stub binary) |
| Port | Dynamic (service_port file) | Same (reads service_port file) |

## Migration Notes

1. The stub binary replaces the real `fortitray` at `/opt/forticlient/fortitray`
2. `fctsched` will start the stub instead of the real `fortitray`
3. The stub reads the same `service_port` file that confighandler writes
4. All existing Go code that connects to the WebSocket continues to work unchanged
