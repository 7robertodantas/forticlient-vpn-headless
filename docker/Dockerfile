# Headless FortiClient VPN gateway.
# Build context is the repo root (see Makefile): needs vendor/*.deb.
FROM debian:trixie-slim

ARG TARGETARCH
ARG FCT_VERSION=7.4.3.5411
ENV DEBIAN_FRONTEND=noninteractive

# Runtime libs of the non-GUI binaries (confighandler, fctsched, iked, vpn,
# fortivpn, forticlient-cli, firewall, fctdns) plus what postinst calls.
# GTK/X11/Electron deps of the package are deliberately not installed.
# The Electron GUI (~300 MB), docs, man pages and locales are not unpacked.
RUN printf 'path-exclude=%s\n' '/opt/forticlient/gui/*' '/usr/share/doc/*' \
      '/usr/share/man/*' '/usr/share/locale/*' > /etc/dpkg/dpkg.cfg.d/headless \
    && apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates gzip iptables iproute2 libnss3-tools \
      libglib2.0-0t64 libsecret-1-0 libudev1 libcap2 libuuid1 libgcrypt20 \
      libstdc++6 zlib1g \
      curl jq socat microsocks procps \
    && rm -rf /var/lib/apt/lists/*

# Bind-mounted, not COPYed, so the deb does not stay in an image layer.
RUN --mount=type=bind,source=vendor/forticlient_vpn_${FCT_VERSION}_${TARGETARCH}.deb,target=/tmp/forticlient.deb \
    dpkg --force-depends -i /tmp/forticlient.deb \
    # confighandler only answers clients whose real executable lives under
    # /opt/forticlient, so the import helper uses its own copy of curl.
    && install -m 0755 /usr/bin/curl /opt/forticlient/.headless-curl \
    # forticlient-cli refuses to run as root.
    && useradd -m -s /bin/bash fct

COPY scripts/gateway/ /usr/local/bin/
RUN chmod 0755 /usr/local/bin/fct-*

ENV FCT_USER=fct
EXPOSE 1080
ENTRYPOINT ["/usr/local/bin/fct-entrypoint"]
