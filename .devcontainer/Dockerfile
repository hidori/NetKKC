FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends mozc-server=2.29.5160.102+dfsg-1.4 procps \
    && rm -rf /var/lib/apt/lists/*
