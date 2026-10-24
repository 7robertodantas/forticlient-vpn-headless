# forticlient-vpn-headless

Run the vendor FortiClient VPN (IPsec IKEv2 + SAML) headless in a Docker
container or a small VM. Do the SAML login in the host's browser, and use the
tunnel from the host through a SOCKS5 proxy. DNS and `/etc/hosts` changes stay
inside the gateway.

See [docs/PLAN.md](docs/PLAN.md) for the design and
[docs/FINDINGS.md](docs/FINDINGS.md) for what was learned about FortiClient
internals.

## Setup

```bash
cp ~/Downloads/forticlient_vpn/forticlient_vpn_7.4.3.5411_{arm64,amd64}.deb vendor/
cp /path/to/your-profile.xml secrets/   # git-ignored
cp .env.example .env                    # git-ignored; set FCT_PROFILE (and GW/SSH_OPTS for a VM)
```

## Docker

```bash
make docker-up          # build, start, import secrets/*.xml, SOCKS5 on 127.0.0.1:1080
make docker-connect     # opens SAML login in your browser, waits for the tunnel
make docker-status
curl --socks5-hostname 127.0.0.1:1080 http://some.internal.host/
```

Change the profile name with `make docker-connect PROFILE="Other VPN"`.

### Shut down

```bash
make docker-disconnect  # close the tunnel; DNS and routes inside the container are restored
make docker-status      # should print STATE=disconnected
make docker-down        # stop and remove the container, its network and the image
```

`make docker-disconnect` keeps the container running, so you can run
`make docker-connect` again. `make docker-down` removes everything. The next
`make docker-up` rebuilds from scratch. If the container is gone, nothing on
the host needs cleaning up: the VPN never touched the host's DNS or routes.

The container needs `NET_ADMIN`, `SYS_PTRACE` and `SYS_ADMIN`, plus
`seccomp=unconfined` (FortiClient uses the kernel keyring). It also needs a
network subnet that the VPN does not push (`FCT_SUBNET`, default
`198.18.0.0/24`). [docs/FINDINGS.md](docs/FINDINGS.md) explains why.

## Lima VM

A local, ephemeral Ubuntu 24.04 VM (arm64, `vz`) on macOS, created from
`lima/forticlient-vpn-headless.yaml`. The repo is mounted read-only and
`scripts/install-gateway.sh` runs on first boot. SOCKS5 is forwarded to
`127.0.0.1:1080`.

```bash
make vm-up              # create (or recreate) the VM and install the gateway
make vm-connect         # opens SAML login in your browser, waits for the tunnel
make vm-status
make vm-disconnect      # close the tunnel, keep the VM
make vm-down            # delete the VM
```

## External VM gateway

Any Debian/Ubuntu VM you can reach over SSH, for example on a hypervisor in
your LAN. Tested on Debian 13 (amd64). Copy the needed
parts of the repo to the VM, then run the installer there. It is safe to re-run.

```bash
GW=root@<gateway-ip>; KEY=~/.ssh/<key>
COPYFILE_DISABLE=1 tar --no-xattrs -czf - scripts secrets/*.xml vendor/*_amd64.deb \
  | ssh -i $KEY $GW 'mkdir -p /opt/fct-headless && tar -xzf - -C /opt/fct-headless --no-same-owner'
ssh -i $KEY $GW bash /opt/fct-headless/scripts/install-gateway.sh /opt/fct-headless
```

Then, from your machine:

```bash
make ssh-connect      # GW and SSH_OPTS from .env (or pass GW=... SSH_OPTS=...)
make ssh-status
make ssh-disconnect
```

Point SOCKS5 at `<vm>:1080`. The installer adds `fct-microsocks.service`
unless something already listens on 1080. It also installs an empty
`forticlient-headless-deps` package that stands in for the GUI libraries, so
apt stays consistent.

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
| `scripts/connect.sh` | host | opens the URL, forwards the callback port, waits |
| `scripts/saml-forward.py` | host | `localhost:PORT` → `GW_EXEC socat` → gateway `127.0.0.1:PORT` |
