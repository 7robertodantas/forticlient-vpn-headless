# Handoff: FortiClient IPC Reverse Engineering

**Date:** 2026-10-22
**Target:** FortiClient VPN 7.4.3.5411 (Ubuntu 24.04, x86_64)
**Method:** strace on `forticlient-cli vpn connect` (non-root user)

## Overview

This document details the complete IPC flow between the FortiClient CLI and the FortiClient services (confighandler, fortitray/fctservice, fortivpn, iked). The key finding is that `fortitray` is the only binary requiring GUI libraries (GTK/GDK/notify), and all other FortiClient components work headlessly.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  fctsched (scheduler)                           │
│  - Spawns confighandler, fortitray, firewall    │
│  - No GUI dependency                            │
└─────────────────────────────────────────────────┘
          │
          │ (forks, writes service_port)
          ▼
┌─────────────────────────────────────────────────┐
│  confighandler (HTTP)                           │
│  - Port: dynamic, written to                    │
│    /var/run/forticlient/service_port            │
│  - Endpoints: /config?request=12,               │
│    /config?request=7, /vpn, /vpn/state          │
│  - Response: protobuf (content-type:            │
│    application/octet-stream)                    │
│  - No GUI dependency                            │
└─────────────────────────────────────────────────┘
          │
          │ (same dynamic port, e.g. 36343)
          ▼
┌─────────────────────────────────────────────────┐
│  fortitray (fctservice) (WebSocket)             │
│  - Endpoint: ws://127.0.0.1:{port}/websocket    │
│  - Messages: binary WebSocket frames (JSON)     │
│  - REQUIRES: libgtk, libgdk, libnotify          │
│  - Headless stub: replaces this binary          │
└─────────────────────────────────────────────────┘
          │
          │ (IPC to fortivpn/iked for actual tunnel)
          ▼
┌─────────────────────────────────────────────────┐
│  fortivpn + iked                                │
│  - IPsec tunnel (IKEv2 + ESP)                   │
│  - No GUI dependency                            │
└─────────────────────────────────────────────────┘
```

## IPC Flow: `forticlient-cli vpn connect`

### Phase 1: Library Loading (skipped for brevity)
- Loads: libresolv, libpthread, libc, libsecret-1, libglib, libgio, libgobject, libgcrypt, libm, libpcre2, libgmodule, libz, libmount, libselinux, libffi, libgpg-error, libblkid

### Phase 2: Service Discovery
```
openat(AT_FDCWD, "/var/run/fctc.s", O_RDONLY) = 3
openat(AT_FDCWD, "/var/run/forticlient/service_port", O_RDONLY) = 3
read(3, "36343", 8191) = 5       # Reads the dynamic service port
```

The CLI reads the dynamic service port from `/var/run/forticlient/service_port`. This file is written by `confighandler` when it starts.

### Phase 3: Config Queries (HTTP to confighandler)
```
socket(AF_INET, SOCK_STREAM, IPPROTO_IP) = 3
connect(3, {sa_family=AF_INET, sin_port=htons(36343), sin_addr=inet_addr("127.0.0.1")}, 16) = -1 EINPROGRESS
sendto(3, "GET /config?request=12 HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n", 33, MSG_NOSIGNAL) = 33
recvfrom(3, "HTTP/1.0 200 OK\r\ncontent-type: application/octet-stream\r\n...", 16384) = 180

sendto(3, "GET /config?request=7 HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n", 32, MSG_NOSIGNAL) = 32
recvfrom(3, "HTTP/1.0 200 OK\r\ncontent-type: application/octet-stream\r\n...", 16384) = 344
```

**Key findings:**
- CLI queries confighandler via HTTP on the dynamic service port
- Two requests: `GET /config?request=12` (180 bytes) and `GET /config?request=7` (344 bytes)
- Response content-type: `application/octet-stream` (protobuf, not JSON or XML)
- The `/config` endpoint uses protobuf encoding, not raw XML

### Phase 4: Check fortivpn Process
```
openat(AT_FDCWD, "/proc", O_RDONLY|O_NONBLOCK|O_CLOEXEC|O_DIRECTORY) = 3
openat(AT_FDCWD, "/proc/17326/cmdline", O_RDONLY) = 3
read(3, "fortivpn\0connect\0TestVPN\0", 8191) = 25
openat(AT_FDCWD, "/run/user/501/forticlient/fortivpn.pid", O_RDONLY) = 3
openat(AT_FDCWD, "/run/user/501/forticlient/fortivpn.pid", O_RDWR|O_CREAT|O_TRUNC, 0666) = 3
write(3, "17326\0", 6) = 6
```

**Key findings:**
- CLI checks if `fortivpn` is running by reading `/proc/<pid>/cmdline`
- Checks for `fortivpn\0connect\0<name>` in the command line
- Writes its own PID to `/run/user/<uid>/forticlient/fortivpn.pid`

### Phase 5: D-Bus Key Store Access
```
socket(AF_UNIX, SOCK_STREAM|SOCK_CLOEXEC|SOCK_NONBLOCK, 0) = 7
connect(7, {sa_family=AF_UNIX, sun_path="/run/user/501/bus"}, 110) = 0
sendto(7, "AUTH\r\n", 6, MSG_NOSIGNAL) = 6
recvfrom(7, "REJECTED EXTERNAL\r\n", 4096) = 19
sendto(7, "AUTH EXTERNAL\r\n", 15, MSG_NOSIGNAL) = 15
recvfrom(7, "DATA\r\n", 4096) = 6
sendto(7, "DATA\r\n", 6, MSG_NOSIGNAL) = 6
recvfrom(7, "OK 38b74c27f1f88485776d2efc6ada0...", 4096) = 37
sendto(7, "NEGOTIATE_UNIX_FD\r\n", 19, MSG_NOSIGNAL) = 19
recvfrom(7, "AGREE_UNIX_FD\r\n", 4096) = 15
sendto(7, "BEGIN\r\n", 7, MSG_NOSIGNAL) = 7
```

**Key findings:**
- CLI connects to D-Bus session bus at `/run/user/<uid>/bus`
- Uses D-Bus EXTERNAL authentication with user's UID (501)
- Requests Unix file descriptor negotiation
- This is for accessing the system key store (fails in VM: "The name org.freedesktop.secrets was not provided by any .service files")

### Phase 6: WebSocket to FortiClient Service
```
# First connection (CLI → Real FortiClient)
socket(AF_INET, SOCK_STREAM, IPPROTO_IP) = 12
connect(12, {sa_family=AF_INET, sin_port=htons(36343), sin_addr=inet_addr("127.0.0.1")}, 16) = -1 EINPROGRESS
sendto(12, "GET /websocket HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", 157, MSG_NOSIGNAL) = 157
recvfrom(12, "HTTP/1.1 101 Switching Protocols\r\n...", 16384) = 166

# WebSocket frame 1: Connect request (98 bytes payload)
# Frame: \201\376\0\250e\221!\t + \36\263Up\25\364\0033G\327b]:\307qG:\322nG+...
# Payload: {"msg":{"connection_name":"TestVPN","success":true}}

sendto(12, "\201\376\0\250e\221!\t", 8, MSG_NOSIGNAL) = 8       # WebSocket handshake (FIN+opcode, mask, length, key)
sendto(12, "\36\263Up\25\364\0033G\327b]:\307qG:\322nG+...", 168, MSG_NOSIGNAL) = 168  # Masked payload (98 bytes)

# Response: XAuth challenge (35 bytes)
recvfrom(12, "\201!{\"msg\":{},\"type\":\"FCT_VPN_XAUTH\"}", 16218) = 35
write(11, "{\"msg\":{},\"type\":\"FCT_VPN_XAUTH\"}", 33) = 33

# Response: Connecting status (31 bytes)
recvfrom(12, "\201\35{\"type\":\"FCT_VPN_CONNECTING\"}", 16218) = 31
write(11, "{\"type\":\"FCT_VPN_CONNECTING\"}", 29) = 29

# Response: Error (91 bytes)
recvfrom(12, "\201Y{\"msg\":{\"message\":\"Error: Can not connect to VPN server.\"}}", 16384) = 91
write(11, "{\"msg\":{\"message\":\"Error: Can not connect to VPN server.\"}}", 89) = 89
```

**Key findings:**
- CLI establishes WebSocket to `ws://127.0.0.1:{service_port}/websocket`
- Sends WebSocket text frame with connection name: `{"msg":{"connection_name":"TestVPN","success":true}}`
- Real FortiClient responds with:
  1. `FCT_VPN_XAUTH` (XAuth challenge with empty msg)
  2. `FCT_VPN_CONNECTING` (status update)
  3. `FCT_VPN_DISCONNECTED` with error message (in VM, no actual VPN server)

### Phase 7: Second WebSocket Connection (for SAML/SAML response)
```
# Second connection (CLI → Real FortiClient for SAML)
socket(AF_INET, SOCK_STREAM, IPPROTO_IP) = 13
connect(13, {sa_family=AF_INET, sin_port=htons(36343), sin_addr=inet_addr("127.0.0.1")}, 16) = -1 EINPROGRESS
sendto(13, "GET /websocket HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: ...\r\nSec-WebSocket-Version: 13\r\n\r\n", 157, MSG_NOSIGNAL) = 157

# WebSocket frame: SAML response (33 bytes payload)
sendto(13, "\201\241ZL\235y", 6, MSG_NOSIGNAL) = 6       # WebSocket handshake
sendto(13, "!n\351\0*)\277Cx\n\336-\5\32\3157\5\24\334,\16\4\277Ux!\356\36xv\346\4", 33, MSG_NOSIGNAL) = 33  # Masked payload
```

## WebSocket Protocol

### Frame Structure (from strace)
```
Byte 0: FIN (bit 7) + Opcode (bits 3-0)
  - 0x81 = FIN=1, Opcode=1 (text message)
  - 0x82 = FIN=1, Opcode=2 (binary message)
Byte 1: MASK (bit 7) + Payload Length (bits 6-0)
  - 0x7e = 126 bytes (2-byte length follows)
  - 0x7f = 127 bytes (8-byte length follows)
  - 0x78 = 120 bytes (direct, no masking key)
  - 0x74 = 116 bytes (direct, no masking key)
Bytes 2-5 (if MASK=1): Masking key (4 bytes)
Bytes 6+ (if MASK=1): Masked payload
```

### Message Types (Client → Server)
| Type | Description |
|------|-------------|
| `FCT_VPN_CONNECT` | Connect to VPN tunnel |
| `FCT_VPN_DISCONNECT` | Disconnect from VPN |
| `FCT_VPN_CANCEL` | Cancel current operation |
| `FCT_VPN_SAML_RESPONSE` | SAML authentication result |
| `FCT_VPN_XAUTH` | XAuth credentials response |

### Message Types (Server → Client)
| Type | Description |
|------|-------------|
| `FCT_VPN_CONNECTED` | VPN connected successfully |
| `FCT_VPN_CONNECTING` | Connecting in progress |
| `FCT_VPN_RECONNECTING` | Reconnecting in progress |
| `FCT_VPN_DISCONNECTED` | VPN disconnected |
| `FCT_VPN_XAUTH` | XAuth challenge (username/password/token) |
| `FCT_VPN_XAUTH_TOKEN` | XAuth OTP token prompt |
| `FCT_VPN_XAUTH_RSA` | XAuth RSA token prompt |
| `FCT_VPN_XAUTH_NEW_PWD` | XAuth new password prompt |
| `FCT_VPN_XAUTH_CREDENTIALS` | XAuth credentials prompt |
| `FCT_VPN_DISCLAIMER` | Disclaimer message |
| `FCT_VPN_CONFIRMATION` | Confirmation message |
| `FCT_VPN_INVALID_CERTIFICATE` | Invalid certificate |
| `FCT_VPN_LIMIT_USER` | User limit reached |

### Example: Connect Request Frame
```
0x81 0x78                    # FIN=1, opcode=1 (text), length=120, no mask
{"msg":{"connection_name":"TestVPN","success":true}}  # 120 bytes JSON payload
```

### Example: XAuth Challenge Response Frame
```
0x81 0x23                    # FIN=1, opcode=1 (text), length=35, no mask
{"msg":{},"type":"FCT_VPN_XAUTH"}  # 35 bytes JSON payload
```

### Example: Error Response Frame
```
0x81 0x59                    # FIN=1, opcode=1 (text), length=89, no mask
{"msg":{"message":"Error: Can not connect to VPN server."}}  # 89 bytes JSON payload
```

## File-Based IPC

### Service Port File
- **Path:** `/var/run/forticlient/service_port`
- **Content:** Dynamic port number (e.g., `36343`)
- **Written by:** `confighandler` when it starts
- **Read by:** CLI, stub, and other FortiClient components

### PID Files
- **fctsched:** `/var/run/forticlient/fctsched.pid`
- **sslvpn:** `/var/run/forticlient/sslvpn.pid`
- **fortivpn:** `/run/user/<uid>/forticlient/fortivpn.pid`

### IPC Sockets
- **firewall.ipc:** `/var/run/forticlient/firewall.ipc` (Unix domain socket)
- **8e145d51ebfccd7ee77b3eed706792fe.ipc:** FortiClient internal socket
- **8668fd42ebc016367da6f8d16e455684.ipc:** FortiClient internal socket

These sockets return a binary protocol (`SP\x00\x01`) when probed with "ping".

### D-Bus Socket
- **Path:** `/run/user/<uid>/bus`
- **Protocol:** D-Bus EXTERNAL authentication
- **Used by:** CLI to access system key store (fails in VM without secret-service)

## Stub Implementation

The stub replaces `fortitray` (which requires GTK/GDK/notify) with a Go WebSocket proxy.

### How It Works
1. **Listens on port 8021** (fixed, separate from confighandler's dynamic port)
2. **Reads `service_port` file** to find the real FortiClient service port
3. **Accepts WebSocket connections** from the CLI (fcvpn)
4. **Establishes WebSocket connection** to the real FortiClient service on the dynamic service_port
5. **Proxies WebSocket frames bidirectionally** between CLI and real FortiClient service

### Files Modified
| File | Change |
|------|--------|
| `constants.go` | Removed `WebSocketStubPort = 8021`, added `DefaultStubPort = 8021` |
| `pkg/fctservice/stub.go` | Rewrote to proxy WebSocket frames instead of faking responses |
| `cmd/fortitray-stub/main.go` | Updated to use new `NewStub(stubPort)` constructor |
| `pkg/service/manager.go` | Updated `startFortitrayStub` to update `service_port` file with stub port (8021) |
| `pkg/ws/client.go` | Updated `NewStubClient` to use `DefaultStubPort` |

### Usage
```bash
# Build the stub
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/fortitray-stub ./cmd/fortitray-stub/

# Install in VM
sudo cp /tmp/fortitray-stub /opt/forticlient/fortitray-stub
sudo chmod +x /opt/forticlient/fortitray-stub

# Start services (fctsched → confighandler + fortitray, then fortitray-stub)
sudo /opt/forticlient/fctsched &
/opt/forticlient/fortitray-stub &

# The CLI connects to port 8021 (stub), which proxies to real FortiClient on 36343
/opt/forticlient/fcvpn connect "MyVPN"
```

## Summary of IPC Mechanisms

| Mechanism | Used By | Purpose |
|-----------|---------|---------|
| HTTP (REST) | CLI → confighandler | Config queries (`/config?request=12`, `/config?request=7`) |
| WebSocket | CLI → fortitray/fctservice | VPN connect/disconnect/SAML/cancel commands |
| D-Bus (EXTERNAL) | CLI → secret-service | Key store access (fails in headless VM) |
| Unix domain sockets | FortiClient internal | Firewall, internal IPC |
| Text files | CLI, services | Service port, PIDs, process status |
| `/proc/<pid>/cmdline` | CLI | Check if fortivpn is running |
| Protobuf | confighandler | Config data (content-type: application/octet-stream) |

## Key Takeaways

1. **Only `fortitray` requires GUI libraries** (GTK/GDK/notify). All other FortiClient components (fctsched, confighandler, fortivpn, iked, firewall, fctdns, update) work headlessly.

2. **The CLI communicates with FortiClient services via:**
   - HTTP to confighandler (dynamic port from `service_port` file)
   - WebSocket to fortitray/fctservice (same dynamic port)
   - D-Bus for key store access (fails without secret-service)

3. **The stub proxies WebSocket frames** between the CLI and the real FortiClient service, eliminating the GTK dependency while maintaining full functionality.

4. **The real FortiClient service handles:**
   - VPN connect/disconnect (via fortivpn/iked)
   - SAML authentication flow
   - XAuth challenges (username/password/token)
   - Connection status updates

5. **The stub does NOT implement VPN logic** — it simply proxies WebSocket frames to the real FortiClient service, which does all the actual work (fortivpn for IPsec tunnel, iked for IKEv2 key exchange).
