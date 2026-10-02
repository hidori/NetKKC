FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends mozc-server procps \
    && rm -rf /var/lib/apt/lists/*
