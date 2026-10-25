# Plan: headless FortiClient IPsec + SAML gateway

## Goal

Run the vendor FortiClient VPN 7.4.3 (IPsec IKEv2 + SAML via external browser)
on a headless Linux box: a Docker container, a local Lima VM, or an external
VM reached over SSH. The VPN changes routes, DNS (`/etc/resolv.conf`) and
`/etc/hosts` inside that box only. A SOCKS5 proxy (`microsocks`) on the same box
lets the host send selected traffic through the tunnel.

```
 host (macOS)                              gateway (container or VM)
 ┌──────────────────────┐                  ┌─────────────────────────────────────┐
 │ browser ─────────────┼── :<redirect> ──►│ iked SAML callback (127.0.0.1)      │
 │   (SAML login)       │   port-forward   │ fctsched → confighandler, iked, ... │
 │                      │                  │ saml-helper: prints the login URL   │
 │ apps ── SOCKS5 ──────┼── :1080 ────────►│ microsocks ──► IPsec tunnel (XFRM)  │
 └──────────────────────┘                  └─────────────────────────────────────┘
```

## What we know about the package

The deb was inspected (arm64 and amd64 have the same layout).

- `fctsched` is the systemd service (`ExecStart=/opt/forticlient/fctsched`). It
  starts `confighandler` (Rust, HTTP and WebSocket hub on a dynamic port written
  to `/var/run/forticlient/service_port`) and the other daemons.
- `iked` does IKEv2 and installs kernel XFRM (IPsec) state. It also rewrites
  `/etc/resolv.conf` and `/etc/hosts`. It has no GUI dependency.
- `fortitray` and the Electron `gui/` are the only GTK/X11 parts. They are not
  needed to keep a tunnel up.
- SAML with `use_external_browser=1`: `iked` runs a small HTTP listener for the
  browser redirect. It sends `{"type":"rawData","bytes":"UID=…&REDIRECT_PORT=…"}`
  over the WebSocket hub to whichever UI is attached, and the UI opens the
  browser. This is why `forticlient-cli vpn connect` ends with "Login canceled"
  when no UI is attached.
- `confighandler` only accepts local clients whose real executable
  (`/proc/<pid>/exe`) is under `/opt/forticlient/`. Symlinks do not count.

## Approach

Use the vendor binaries unchanged. Do not reimplement the IPC (that was the
earlier Go attempt, now removed). Add only:

1. **Install without GUI.** Install the deb on a minimal Debian 13 and run
   `fctsched` as PID-1 child (container) or systemd service (VM).
2. **Import the profile.** Use the vendor CLI or GUI-less config path to load
   the XML.
3. **SAML helper.** Watch the event that would make the GUI open a browser,
   build the login URL, print it, and leave the `iked` redirect listener
   reachable from the host through a port forward.
4. **SOCKS5.** Run `microsocks` bound to the gateway's interface and publish
   port 1080 to the host.

## Phases

| # | Phase | Pass criterion |
|---|-------|----------------|
| 0 | Clean repo, write this plan | Old Go code gone |
| 1 | Docker image: install deb, start `fctsched` | `forticlient vpn list` works in the container |
| 2 | Import profile | the profile is listed |
| 3 | `vpn connect` and find the SAML hand-off | Login URL and redirect port captured |
| 4 | Browser on host completes SAML through the port forward | Tunnel up, XFRM state present, DNS rewritten in the container |
| 5 | microsocks | `curl --socks5-hostname localhost:1080 <internal host>` works from the host |
| 6 | Same scripts in a Lima VM and in an external VM over SSH | Same criteria as 1–5 |

## Docker feasibility notes

- IPsec XFRM state and policy are per network namespace. The container needs
  `NET_ADMIN` (and probably `SYS_MODULE` off, with modules already in the host
  kernel). Docker Desktop's LinuxKit kernel ships XFRM. `/dev/net/tun` is passed
  in case the `vpn` binary uses it.
- `/etc/resolv.conf` and `/etc/hosts` are bind-mounted by Docker but writable,
  so `iked` can rewrite them. The effect stays inside the container.
- No systemd: the entrypoint starts `fctsched` directly. D-Bus is started only
  if a component insists on it. The "org.freedesktop.secrets" warning is
  harmless.
- Port forwarding: on Docker Desktop, `-p 127.0.0.1:P:P` to a service listening
  on the container's `127.0.0.1` does not work. A `socat` relay inside the
  container forwards from `0.0.0.0` to the `127.0.0.1` listener.

If Docker fails on something fundamental (kernel XFRM, systemd-only
behaviour), the same install script runs in a Lima VM with `vz` and arm64 on
the Mac, with Lima port forwards for the redirect port and 1080.

## Layout

```
docker/                 Dockerfile, entrypoint
scripts/                install + helpers shared by Docker and VM
lima/                   Lima VM template
vendor/                 (git-ignored) the .deb files; the Docker build
                        downloads them from Fortinet when absent
secrets/                (git-ignored) the profile XML
docs/PLAN.md            this file
docs/FINDINGS.md        what each attempt showed
```

Everything is ephemeral: `make docker-up` and `make docker-down`, `make vm-up`
and `make vm-down`.
