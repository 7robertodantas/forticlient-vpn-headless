# Findings

FortiClient VPN 7.4.3.5411, arm64 deb, Docker Desktop (LinuxKit 6.10) on
Apple Silicon. Dates are 2026-10-24.

## Docker works for the control plane

- `fctsched` starts fine without systemd. It launches `confighandler`,
  `firewall`, and later `fortivpn` and `iked` on connect.
- None of those need GTK. The image installs the deb with
  `dpkg --force-depends` and only the libs those binaries link to (glib,
  libsecret, libudev and a few others). `fortitray` and the Electron GUI are
  present on disk but never run.

## Two container-specific requirements

1. **`SYS_PTRACE`**. `confighandler` (root) authorizes each local HTTP client by
   reading `/proc/<pid>/exe` and accepting only binaries under
   `/opt/forticlient/`. The CLI refuses to run as root, so the client is
   another uid. Reading another uid's `/proc/<pid>/exe` needs `CAP_SYS_PTRACE`,
   and Docker drops it. Without it every call is `403`, even from the vendor
   CLI.
2. **`XDG_RUNTIME_DIR`**. The CLI fails with "Failed to set up FortiClient
   directories" unless `/run/user/<uid>` exists and is owned by the user.

## Importing the XML profile

- `forticlient vpn edit` only creates SSL-VPN profiles.
- The GUI's import (Settings > Restore, and VPN > Import XML) is
  `POST /settings/config` with JSON
  `{"type":"restore","path":"<file>","password":""}`. `confighandler` reads the
  file itself. Found in the Electron source maps
  (`gui/resources/app.asar`, `system_settings.js` and `xml_vpn.js`).
- The caller must be a real binary under `/opt/forticlient`, so the image keeps
  a copy of curl at `/opt/forticlient/.headless-curl`.
- `forticlient vpn list` does not show "corporate" IPsec profiles.
  `GET /vpn` does (`fct-list`).

## SAML with an external browser

On `forticlient vpn connect`:

1. `fortivpn` writes `/tmp/.forticlient/iked.conf` and starts `iked`.
2. `iked` asks the gateway for a SAML URL and logs it to
   `/var/log/forticlient/ipsecvpn.log`:
   `Received SAML URL https://<gw>:<ike_saml_port>/saml?<id>`.
3. `iked` opens a callback listener on `127.0.0.1:<random port>`. It asks the
   UI to open the URL (`confighandler` then logs "Failed to send message to
   fortitray", which is expected and harmless).
4. After the IdP login, the gateway redirects the browser to that localhost
   port, `iked` gets the token and continues IKEv2 (EAP).

The CLI exits right away ("Unable to use system's key store"), but `fortivpn`
and `iked` keep waiting for the token. `iked` retries the policy every 60s
without a new URL.

Headless solution: `fct-connect` reads the URL from the log and the port from
`ss`. On the host, `scripts/connect.sh` opens the URL and runs
`scripts/saml-forward.py`. That listens on `localhost:<port>` (IPv4 and IPv6)
and pipes each connection through `docker exec -i … socat` (or
`limactl shell`, or `ssh`) to the gateway's `127.0.0.1:<port>`. A fixed
`docker -p` mapping cannot be used because the port changes on every attempt.

## The kernel keyring (why the first login failed)

The first browser login reached `iked` but failed with "IPsec VPN login
failed due to unrecognized SAML token" (`fct_eap_peer_init: failed to get
password`). Disassembling `iked` showed that the EAP password getter does
`keyctl(KEYCTL_SEARCH, KEY_SPEC_PROCESS_KEYRING, "user", "saml_token")`. The
same holds for `"user_password"`. FortiClient passes secrets between its own
processes through the kernel keyring. Docker's default seccomp profile blocks
`keyctl` and `add_key`, so the container runs with `seccomp=unconfined`. This
is not a problem on a VM.

`iked` also unmounts whatever is mounted on `/etc/hosts` and mounts its own
hosts file there, which needs `SYS_ADMIN`. Docker's `/etc/hosts` is a bind
mount over an empty file, so the entrypoint moves Docker's content into the
real file at startup. Otherwise `localhost` disappears once `iked` unmounts
it. `/etc/resolv.conf` is rewritten in place and is fine.

## Verified (2026-10-24, Docker)

- [x] SAML login in the Mac browser, then the callback is forwarded and the
      tunnel comes up: `State: Connected`, a VPN virtual IP (`<vip>`), split routes
      `10.0.0.0/8` and `172.16.0.0/16`.
- [x] `/etc/resolv.conf` is rewritten to the VPN DNS (`<vpn-dns>`) inside the
      container only.
- [x] Traffic goes through the tunnel (`ping <vpn-dns>`, about 110 ms).
- [x] SOCKS5 from the Mac works (`curl --socks5-hostname 127.0.0.1:1080 …`).
      Public sites leave directly because of the split tunnel.
- [x] An internal hostname through SOCKS5 end to end. An internal
      Jenkins host (public IP in a pushed subnet) answered after the
      missing route was added by hand. See the next point.
- [x] Container subnet pinned to `198.18.0.0/24` (compose). After a fresh
      `make docker-up` all 124 routes are installed and Jenkins answers through
      SOCKS5 with no manual route. The VPN pushes about 120 split subnets, including
      `172.20.0.0/16`, which is the subnet Compose had picked. FortiClient
      stopped adding routes at that conflict ("File exists"), so everything
      after it, including the Jenkins subnet, had XFRM policies but no route. The
      same rule applies to a VM: its LAN must not overlap a pushed subnet.
- [x] `/etc/hosts` keeps localhost after connect (entrypoint fix).
- [x] Disconnect (`fct-disconnect`) restores DNS and routes and removes all
      XFRM state and policy.
- [x] Browsers pre-open idle sockets. `iked`'s callback server handles one
      connection at a time and hung on the idle one, so `saml-forward.py`
      connects through only once a request arrives.
- [x] Lima VM (`make vm-up`, `make vm-connect`). Ubuntu 24.04 arm64 on vz runs with stock systemd and no special
      permissions. After the browser login: `State: Connected`, 124 routes,
      VPN DNS, and Jenkins through `127.0.0.1:1080`. Disconnect restores the
      systemd-resolved `resolv.conf` symlink and removes all VPN routes and
      XFRM policy.

## VM notes

- Lima auto-forwards every guest localhost port. That would grab `iked`'s
  callback port, and Lima's forwarder would hang on idle browser preconnects.
  The template forwards only 1080 and ignores everything else.
- While connected, `resolv.conf` points at the VPN DNS, which cannot resolve
  the VM's own hostname (`sudo: unable to resolve host`).
  `install-gateway.sh` adds `127.0.1.1 <hostname>` to `/etc/hosts`. It was
  applied by hand to the test VM but not yet re-verified with a connection.
- The VM's LAN must not overlap any pushed subnet. Lima's `192.168.5.0/24` is
  fine. Check this for an external VM's network.

Harmless noise: "Failed to add route … 172.20.0.0" (that route already exists
for the Docker network), and confighandler's "Failed to send message to
fortitray".

## External VM gateway (Debian 13, amd64), 2026-10-24

- [x] `install-gateway.sh` on Debian trixie. All runtime packages exist under
      the same names.
- [x] `make ssh-connect` gives `State: Connected` and 124 routes. Jenkins
      answers through the VM's SOCKS5 (`<gateway-ip>:1080`).
- [x] The VPN pushes a `/24` that is the VM's own LAN (`<lan>`). The kernel's
      connected route has metric 100 (DHCP), so FortiClient's metric-0 route is
      added (no "File exists" this time) and wins for new connections. It
      was tested with a pre-existing LAN policy rule on that VM stopped:
      `ip route get <lan-host>` went to `src <vip>`
      (hijacked). With `fct-protect-lan` it went to `table 201 src
      <gateway-ip>`. It is self-contained: `fct-connect` applies it before
      every connect, with no reliance on anything already on the VM and no
      extra service. A first version that deleted the route once after connect was
      dropped, because a FortiClient reconnect would re-add it.
- Pitfalls found and fixed:
  - `dpkg --force-depends` left apt broken ("Unmet dependencies"). It was
    replaced by an empty `forticlient-headless-deps` package that "Provides"
    the GUI-only dependencies, installed before any apt call.
  - The installer overwrote an existing microsocks setup. It now skips its
    SOCKS unit when the port is taken.
  - `ssh` joins argv into one remote command, so `connect.sh` shell-quotes
    the arguments for the ssh transport.
