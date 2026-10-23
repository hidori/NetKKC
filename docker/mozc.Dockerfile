FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

RUN apt-get update --error-on=any \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends mozc-server=2.29.5160.102+dfsg-1.4 \
    && useradd --uid 1000 --create-home netkkc \
    && rm -rf /var/lib/apt/lists/*

ENV XDG_CONFIG_HOME=/var/lib/netkkc
USER netkkc
ENTRYPOINT ["/usr/lib/mozc/mozc_server", "--timeout=-1"]
