# forticlient-vpn-headless

Run the vendor FortiClient VPN (IPsec IKEv2 + SAML) headless in a Docker
container or a small VM. Do the SAML login in the host's browser, and use the
tunnel from the host through a SOCKS5 proxy. DNS and `/etc/hosts` changes stay
inside the gateway.

See [docs/PLAN.md](docs/PLAN.md) for the design and
[docs/FINDINGS.md](docs/FINDINGS.md) for what was learned about FortiClient
internals.

## Install

```bash
git clone https://github.com/7robertodantas/forticlient-vpn-headless.git
ln -s "$PWD/forticlient-vpn-headless/bin/fcvpn" ~/.local/bin/fcvpn   # any dir on your PATH
fcvpn init      # writes ~/.config/fcvpn/config from config.example
```

Then:

1. Export your VPN profile from a FortiClient GUI (Settings > Backup), and put
   the XML in `~/.config/fcvpn/profiles/`.
2. Edit `~/.config/fcvpn/config`: one section per profile, each naming the
   connection inside the XML and the gateway that runs it.

```ini
default = work

[work]
connection = My Corp VPN    # the name inside the XML
gateway = docker            # docker, lima or ssh

[work-vm]
connection = My Corp VPN
gateway = ssh
host = root@192.0.2.10
ssh_opts = -i ~/.ssh/id_ed25519
```

Nothing in the repo is specific to you. The config, the XML files, and the
debs in `vendor/` are git-ignored or live outside the repo.
`FCVPN_CONFIG_DIR` points `fcvpn` at another config directory.

## Use

```bash
fcvpn connect           # the default profile: starts the gateway if needed,
                        # opens the SAML login in your browser, waits for the tunnel
fcvpn connect work-vm   # or name one
fcvpn status            # STATE=connected/disconnected, and the SOCKS5 address
curl --socks5-hostname "$(fcvpn proxy)" http://some.internal.host/
fcvpn disconnect        # close the tunnel; the gateway keeps running
fcvpn down              # remove the container (or delete the VM)
fcvpn list              # profiles; * marks the default
```

| Command | What it does |
|---|---|
| `connect [profile]` | start the gateway if needed, log in, wait for the tunnel |
| `disconnect [profile]` | close the tunnel, keep the gateway |
| `status [profile]` | tunnel state, profile, SOCKS5 address; exit 0 when connected |
| `proxy [profile]` | just the SOCKS5 `host:port` |
| `up` / `down [profile]` | start or remove the gateway (container or VM) |
| `import [profile]` | import `~/.config/fcvpn/profiles/*.xml` into a running gateway |
| `setup [profile]` | `gateway = ssh`: install the gateway on that VM |
| `shell [profile]` | root shell on the gateway |
| `list`, `init` | profiles in the config; write a sample config |

Without a profile argument, `fcvpn` uses `$FCVPN_PROFILE`, then `default` in
the config, then the only profile there is.

## Gateways

### Docker (`gateway = docker`)

`fcvpn up` (or the first `connect`) builds the image and starts the
`fct-headless` container. On start, the container imports the XML files and
serves SOCKS5 on `127.0.0.1:1080`, or on `socks` from the profile. Set
`docker = podman` in the config to use Podman.

The build uses `vendor/forticlient_vpn_<version>_<arch>.deb` when it is
there, and otherwise downloads it from Fortinet and checks its SHA-256:

```bash
# Optional: use local debs (no download on every rebuild).
cp ~/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_{arm64,amd64}.deb vendor/
```

Build arguments: `FCT_VERSION` (default `7.4.3.5411`), `FCT_URL` (default
`https://filestore.fortinet.com/forticlient/downloads/forticlient_vpn_${FCT_VERSION}_${TARGETARCH}.deb`)
and `FCT_SHA256_amd64` / `FCT_SHA256_arm64` (pinned for the default version;
set to empty to skip the check for another version). The Lima and external VM
installers still need the deb in `vendor/`.

The image is not published to a registry: Fortinet's EULA (§2) does not
allow redistributing FortiClient, so everyone builds their own and gets the
deb from Fortinet directly.

`fcvpn disconnect` keeps the container running, so you can `connect` again.
`fcvpn down` removes the container, its network and the image. The next
`up` rebuilds from scratch. If the container is gone, nothing on the host
needs cleaning up: the VPN never touched the host's DNS or routes.

The container needs `NET_ADMIN`, `SYS_PTRACE` and `SYS_ADMIN`, plus
`seccomp=unconfined` (FortiClient uses the kernel keyring). It also needs a
network subnet that the VPN does not push (`FCT_SUBNET`, default
`198.18.0.0/24`). [docs/FINDINGS.md](docs/FINDINGS.md) explains why.

### Lima VM (`gateway = lima`, macOS)

A local, ephemeral Debian 13 VM (arm64, `vz`, 1 CPU, 768 MiB, ~0.8 GB disk
used), created from `lima/forticlient-vpn-headless.yaml`. The repo is
mounted read-only, and `scripts/install-gateway.sh` runs on first boot.
`fcvpn up` creates (or recreates) it and imports your XML files. SOCKS5 is
forwarded to `127.0.0.1:1080`. `fcvpn down` deletes the VM.

### External VM (`gateway = ssh`)

Any Debian/Ubuntu VM you can reach over SSH as root, for example on a
hypervisor in your LAN. Tested on Debian 13 (amd64). Put the VM's deb in
`vendor/`, then:

```bash
fcvpn setup work-vm     # copies scripts, the deb and your XML files to /opt/fct-headless
                        # on the VM and runs the installer there; safe to re-run
fcvpn connect work-vm
```

`fcvpn` never starts or stops that VM, and `down` only disconnects. Point
SOCKS5 at `<vm>:1080`, the profile's default `socks`. The installer adds
`fct-microsocks.service` unless something already listens on 1080. It also
installs an empty `forticlient-headless-deps` package that stands in for the
GUI libraries, so apt stays consistent.

If the VPN pushes the gateway's own LAN (for example `192.168.1.0/24`),
`fct-protect-lan` keeps it local. It adds a policy rule per directly attached
subnet (table and priority 201), which takes precedence over the main table
where FortiClient puts its routes. The subnets are detected automatically.
`fct-connect` applies the rules before every connect, so a recreated VM needs
nothing else. Corporate hosts in that range are then unreachable through this
gateway.

## How it works

| Piece | Where | What |
|-------|-------|------|
| `fct-start` | gateway | starts `fctsched` (or the systemd unit) |
| `fct-import` | gateway | GUI-equivalent XML restore through `confighandler` |
| `fct-connect` | gateway | starts the connect, prints `SAML_URL` and `SAML_PORT` |
| `fct-status` / `fct-disconnect` | gateway | tunnel state (XFRM SA) / stop |
| `bin/fcvpn` | host | profiles, gateway lifecycle, the commands below on the right gateway |
| `scripts/connect.sh` | host | opens the URL, forwards the callback port, waits |
| `scripts/saml-forward.py` | host | `localhost:PORT` → `GW_EXEC socat` → gateway `127.0.0.1:PORT` |

### The SAML callback

With a normal desktop FortiClient, a SAML login works like this. FortiClient
starts the IPsec connection, gets a login URL from the VPN server and opens
it in your browser. You log in with your identity provider, and the browser
is then redirected to `http://localhost:<port>/…`. FortiClient's `iked` is
listening on that port, picks up the login token from the request and
finishes the connection. The port is random and changes on every attempt.

Here, FortiClient runs somewhere else (a container, the Lima VM or a remote
VM), but the browser runs on your machine. When the browser follows that
redirect to `localhost:<port>`, it reaches your own machine, where nothing
is listening. So the redirect has to be carried over to the gateway:

1. `fct-connect` starts the connection on the gateway. It reads the login
   URL from FortiClient's log and finds the port `iked` is listening on.
2. `connect.sh` opens the URL in your browser. It also starts
   `saml-forward.py`, which listens on the same port on your machine.
3. When the redirect arrives, `saml-forward.py` runs `socat` on the gateway
   (through `docker exec`, `limactl shell` or `ssh`) and connects to `iked`'s
   port there. It passes the HTTP request through and sends `iked`'s reply
   back to the browser.
4. `iked` gets the token and brings up the tunnel. `connect.sh` checks
   `fct-status` until the tunnel is up, then stops the forwarder.

No port needs to be published on the gateway for this. The request goes
through the same command channel used to run the other commands there
(`GW_EXEC`). This is why the same `connect.sh` works for all three
gateways. A fixed
`docker -p` or Lima port forward could not do it, because the port changes
every time.

```mermaid
sequenceDiagram
    autonumber
    participant B as Browser
    participant C as connect.sh
    participant F as saml-forward.py
    participant G as fct-connect / fct-status
    participant I as iked (127.0.0.1:PORT)

    Note over B,F: host
    Note over G,I: gateway (reached via GW_EXEC)

    C->>G: GW_EXEC fct-connect "$PROFILE"
    G->>I: forticlient-cli vpn connect
    I-->>G: "Received SAML URL" in ipsecvpn.log, listener found with ss -ltnp
    G-->>C: SAML_URL=..., SAML_PORT=PORT
    C->>F: start, listen on localhost:PORT
    C->>B: open SAML_URL
    B->>B: IdP login
    B->>F: redirect to localhost:PORT with the token
    F->>I: GW_EXEC socat STDIO TCP:127.0.0.1:PORT
    I->>I: finish IKEv2 / EAP
    loop every 2s until connected or LOGIN_TIMEOUT (300s)
        C->>G: GW_EXEC fct-status (XFRM SA?)
    end
    C->>F: stop on exit
```

- `saml-forward.py` connects through only after the browser sends data.
  Browsers pre-open idle sockets, and `iked`'s callback server handles one
  connection at a time, so an idle socket would block the real request.
- Running `connect` again is safe. If an attempt is already waiting for SAML,
  `fct-connect` reuses its URL. If the tunnel is up, it prints
  `STATE=connected`.
