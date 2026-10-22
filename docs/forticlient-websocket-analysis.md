# FortiClient 7.4.3.5411 - WebSocket Service Analysis

## Executive Summary

Reverse engineering of the FortiClient Linux 7.4.3.5411 package reveals the exact WebSocket connection protocol used internally. The service process (`fctservice`) embedded in `fortivpn` validates incoming connections based on multiple headers and identity parameters.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         FortiClient Process Tree                     │
├──────────────────┬──────────────────────────────────────────────────┤
│ fortitray (Go)   │  GUI/tray process - connects to service via WS   │
│ fortivpn (C++)   │  Contains fctservice module - WebSocket server   │
│ vpn (C++)        │  VPN connection management                       │
│ confighandler    │  (Rust) Config management via warp web framework │
│ fctsched (C++)   │  Scheduled tasks                                 │
│ fctdns (C++)     │  DNS management                                  │
└──────────────────┴──────────────────────────────────────────────────┘
```

---

## Service Process (`fctservice`)

Located in the `fortivpn` binary (C++ with libevent):

### Key Symbols
```
fctservice::FCTService::start_websocket()
fctservice::FCTService::http_request_send()
fctservice::InternalException
```

### Service Socket
- **NNG IPC path**: `/var/run/forticlient/service_port`
- **WebSocket endpoint**: `/websocket`
- **Dynamic IPC**: `ipc:///var/run/forticlient/<random_hash>.ipc`
  - Examples: `8668fd42ebc016367da6f8d16e455684.ipc`
  - Template: `ipc://%s/forticlient/%s`

---

## WebSocket Connection Protocol (from fortitray)

### Required Request Headers

| Header | Value Pattern | Notes |
|--------|--------------|-------|
| `Origin` | `%s` (from config) | **Must match server-expected value** |
| `User-Agent` | `FortiClientLinuxVPN/` | **Incomplete version string** - note trailing slash |
| `Sec-WebSocket-Key` | `%s` (base64 random) | Standard WS key |
| `Sec-WebSocket-Protocol` | `%s` (protocol string) | **Must match expected protocol** |
| `Sec-WebSocket-Version` | `13` | Standard |
| `Connection` | `Upgrade` | Standard |
| `Upgrade` | `websocket` | Standard |

### Connection URL
- Protocol: `ws://` or `wss://`
- Path: `/websocket`
- Template: `ipc://%s/forticlient/%s`

---

## Identity & Authentication Parameters

### 1. Machine Identity
- **File**: `/etc/forticlient/custom_machine_id.conf`
- Contains a UUID/machine identifier
- Used to identify the connecting client

### 2. Version Strings
- `forticlient_version` - FortiClient version
- `os_version` - Operating system version
- `FortiClientLinuxVPN/` - User-Agent (incomplete)

### 3. Token Types
| Token | Description |
|-------|-------------|
| `ZTNA_TOKEN` | Zero Trust Network Access token |
| `FCT_CLOUD_TOKEN` | FortiCloud authentication token |
| `ONPREM_TOKEN` | On-premises authentication token |
| `auth_token` | General authentication token |

### 4. URL Query Parameter
- `tokeninfo=` - Token information appended to WebSocket URL

---

## CORS Configuration

The service enforces CORS:
- `Access-Control-Allow-Origin: %s`
- `access_control_allow_headers`
- `access_control_allow_methods`
- `access_control_allow_origin`
- `access_control_list`

---

## WebSocket Protocol Messages (JSON)

### Connection State Messages
```json
{"type":"FCT_VPN_CONNECTED"}
{"type":"FCT_VPN_CONNECTING"}
{"type":"FCT_VPN_DISCONNECTED"}
{"type":"FCT_VPN_RECONNECTING"}
```

### Authentication Messages
```json
{"type":"FCT_VPN_XAUTH","msg":{"user_input":1}}
{"type":"FCT_VPN_XAUTH_TOKEN"}
{"type":"FCT_VPN_XAUTH_CREDENTIALS"}
{"type":"FCT_VPN_XAUTH_NEW_PWD"}
{"type":"FCT_VPN_XAUTH_RSA"}
{"type":"FCT_VPN_INVALID_CERTIFICATE"}
{"type":"FCT_VPN_LIMIT_USER"}
{"type":"FCT_VPN_DISCLAIMER"}
```

### ZTNA Messages
```json
{"type":"FCT_ZTNA_CANCEL"}
{"type":"FCT_ZTNA_CONFIRMATION"}
{"type":"FCT_ZTNA_DENY"}
{"type":"FCT_ZTNA_INVALID_CERTIFICATE"}
{"type":"FCT_ZTNA_TIMEOUT"}
```

---

## WebSocket Server Settings (from confighandler)

The confighandler (Rust, using `warp` web framework) validates:
- `origin` header
- `user-agent` header
- `sec-websocket-key`
- `sec-websocket-protocol`
- `sec-websocket-version`
- `sec-websocket-accept` (response)
- `sec-websocket-extensions` (response)

---

## What the Stub Must Send

### Minimum Required Headers
```http
GET /websocket HTTP/1.1
Host: localhost
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: <base64-random-24-char>
Sec-WebSocket-Version: 13
Sec-WebSocket-Protocol: <expected-protocol>
Origin: <expected-origin>
User-Agent: FortiClientLinuxVPN/
```

### Key Findings

1. **`Origin` header is REQUIRED** - The server validates this. The value likely comes from configuration or is hardcoded.

2. **`User-Agent: FortiClientLinuxVPN/`** - Note the **trailing slash with no version number**. This is intentional - the version is sent separately via `forticlient_version`.

3. **`Sec-WebSocket-Protocol`** - Must match the server-expected protocol string.

4. **Machine identity** from `/etc/forticlient/custom_machine_id.conf` is likely validated server-side.

5. **Token** (`ZTNA_TOKEN`, `FCT_CLOUD_TOKEN`, or `ONPREM_TOKEN`) must be included, likely as:
   - Query parameter: `?tokeninfo=<token_value>`
   - Or as a header

6. **CORS** - The `Origin` must match `Access-Control-Allow-Origin`.

7. **NNG Protocol** - The connection uses NNG (nanomsg) protocol over WebSocket, which adds additional framing.

---

## Common Failure Points for Stubs

| Issue | Symptom |
|-------|---------|
| Wrong `Origin` | Connection rejected immediately |
| Missing `User-Agent` | Connection rejected |
| Wrong `Sec-WebSocket-Protocol` | Protocol mismatch error |
| Missing machine_id | Identity validation failure |
| Missing token | Authentication failure |
| Wrong NNG protocol framing | "Peer protocol mismatch" |

---

## Error Messages (from binaries)

```
Websocket handshake failed
websocket out of memory; closing connection
Websocket pull failed; closing connection
Malformed websocket request
Failed to connect with the websocket server
Failed to connect with the websocket server: Invalid port
Failed to parse websocket reply
Peer protocol mismatch: %d != %d, rejected.
Peer pipe protocol %d is not BUS protocol, rejected.
Peer pipe protocol %d is already paired, rejected.
```

---

## Recommendations for Stub Implementation

1. **Set `User-Agent: FortiClientLinuxVPN/`** (with trailing slash, no version)
2. **Read `/etc/forticlient/custom_machine_id.conf`** and include machine ID
3. **Set `Origin`** to the expected value (likely from config or hardcoded)
4. **Include token** via `tokeninfo=` query parameter or header
5. **Use correct `Sec-WebSocket-Protocol`** value
6. **Use proper NNG WebSocket framing** (not raw WebSocket frames)
7. **Connect to `/websocket` endpoint** on the service port
