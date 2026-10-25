# Headless FortiClient VPN gateway.
# Build context is the repo root (see Makefile). Uses
# vendor/forticlient_vpn_<version>_<arch>.deb if present, otherwise downloads
# it from FCT_URL and checks it against FCT_SHA256_<arch> (empty skips it).
FROM debian:trixie-slim

ARG TARGETARCH
ARG FCT_VERSION=7.4.3.5411
ARG FCT_URL=https://filestore.fortinet.com/forticlient/downloads/forticlient_vpn_${FCT_VERSION}_${TARGETARCH}.deb
ARG FCT_SHA256_amd64=d2c0366decbf0b907fbf0d9306b8ac0b705d2d9cb4eb7b946ae06ce5388d4e5a
ARG FCT_SHA256_arm64=45f73f0402afef40b646425317deca09093de23ec0a00d1bbc212e0706764334
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

# vendor/ is bind-mounted, not COPYed, and a downloaded deb is deleted in the
# same step, so the deb never stays in an image layer.
RUN --mount=type=bind,source=vendor,target=/tmp/vendor \
    deb=/tmp/vendor/forticlient_vpn_${FCT_VERSION}_${TARGETARCH}.deb; \
    if [ ! -f "$deb" ]; then \
        deb=/tmp/forticlient.deb \
        && echo "downloading $FCT_URL" \
        && curl -fsSL --retry 3 -o "$deb" "$FCT_URL" \
        && eval "sha=\${FCT_SHA256_${TARGETARCH}:-}" \
        && { [ -z "$sha" ] || echo "$sha  $deb" | sha256sum -c -; } \
        || exit 1; \
    fi \
    && dpkg --force-depends -i "$deb" \
    && rm -f /tmp/forticlient.deb \
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
