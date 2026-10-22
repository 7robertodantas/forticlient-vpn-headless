# Handoff: confighandler 403 and import

Date: 2026-10-22. Lima VM `forticlient-vpn-headless` was running during this work (Ubuntu 24.04, x86_64, FortiClient VPN 7.4.3.5411).

## What is true now

`fcvpn list` works when the binary is a real file under `/opt/forticlient/`. An empty config prints:

```text
No VPN configurations found.
Import one with: fcvpn import <file.xml>
```

`fcvpn import` still does not import a profile. It gets past the 403 and then fails with HTTP 500.

## Why sudo did not fix the 403

`confighandler` does not check the Unix user. `curl` and `sudo curl` both get `403` with an empty body. The official CLI, run as the normal Lima user, is accepted:

```text
forticlient vpn list
VPNs:
  (No VPN profile found)
```

`fortitray` and the Electron GUI were not running. They are not required for this check. The note in `LIMA.md` ("Import/List return HTTP 403") is wrong on that point and still tells you to run `/tmp/fcvpn`, which `confighandler` rejects.

The check is the caller's real executable path. `confighandler` resolves `/proc/<pid>/exe` and allows a binary that actually lives under `/opt/forticlient/`. A symlink does not count, because that path resolves to the target. `/tmp/lima-forticlient/fcvpn` is outside the directory, so it is refused as root or as the user.

Verified inside the VM:

| Caller | `GET /vpn` and `GET /config?request=12` |
|---|---|
| `/usr/bin/curl`, including under sudo | 403 |
| symlink `/opt/forticlient/curl-link` → `/usr/bin/curl` | 403 |
| a real copy of `curl` at `/opt/forticlient/curl` | 200 |
| `/opt/forticlient/fcvpn` | 200 on `GET /vpn` |
| `/tmp/lima-forticlient/fcvpn` | 403 |

The curl copies used for that check were removed. `/opt/forticlient/fcvpn` was left in place.

## Code change

`pkg/config/handler.go` `ListConfigs` now accepts the JSON array that `GET /vpn` returns. An empty config is `[]`. The previous code only unmarshaled a JSON object, so the first successful call died with `cannot unmarshal array into Go value of type map[string]interface {}`. Object responses still go through `extractConnections`.

Not committed. `LIMA.md` also has unrelated uncommitted edits from before this investigation.

## How to retry

Rebuild on the host into the shared directory, then copy into the allowed path. Do not symlink.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/lima-forticlient/fcvpn ./cmd/fctsaml/

limactl shell forticlient-vpn-headless -- bash -lc '
  sudo cp /tmp/lima-forticlient/fcvpn /opt/forticlient/fcvpn
  sudo chmod 755 /opt/forticlient/fcvpn
  /opt/forticlient/fcvpn list
'
```

`fctsched` and `confighandler` were already running. The port is in `/var/run/forticlient/service_port` (it was `37737`). `fortitray` was not running.

## What still blocks import

`/opt/forticlient/fcvpn import /tmp/lima-forticlient/Sangoma_Corp_VPN_North_Linux_READY.xml` printed:

```text
/opt/forticlient/fortitray: error while loading shared libraries: libgtk-x11-2.0.so.0: cannot open shared object file: No such file or directory
  ✗ Sangoma Corp VPN North: import failed: config import failed (HTTP 500):  (may need to restart services)
Imported 0 of 1 connections
```

Two separate problems:

1. `import` starts `fortitray`, and that binary needs `libgtk-x11-2.0.so.0`. The missing library is only a startup warning today. `connect` does need `fortitray`, because that process carries VPN status. Install `libgtk2.0-0` in the VM before testing connect.

2. `ImportConfig` posts the raw XML to `POST /config`. Once the caller is allowed, that request returns HTTP 500 with an empty body. An empty POST to the same URL also returns 500. The official CLI does not use that shape. `strace` of `forticlient vpn list` shows:

```text
GET /config?request=12 HTTP/1.0
Host: 127.0.0.1

HTTP/1.0 200 OK
content-type: application/octet-stream
```

The body is protobuf, not JSON or XML. A second call uses `GET /config?request=7`. `forticlient vpn edit <name>` is the official way to save a profile. Its request format has not been traced yet. `fcvpn import` has to speak that API, or shell out to `forticlient vpn edit`, before a profile will land.

`GET /vpn` from an allowed caller returns JSON (`[]` when nothing is configured). Listing can stay on that endpoint. Import cannot stay on raw `POST /config`.

## Suggested next steps

1. Update the 403 section in `LIMA.md` so it says to copy `fcvpn` under `/opt/forticlient/` and drop the Electron/fortitray auth claim.
2. Install `libgtk2.0-0` in the VM so `fortitray` can start.
3. `strace` a successful `forticlient vpn edit` (it refuses to run as root: "Please run this command under non-root user") and match `ImportConfig` to the `/config?request=` protobuf calls it makes.
4. Re-test `import`, then `list`, with `/opt/forticlient/fcvpn`.
