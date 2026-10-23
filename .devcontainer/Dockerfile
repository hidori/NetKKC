FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN GOBIN=/usr/local/bin GOMODCACHE=/tmp/gomod GOCACHE=/tmp/gocache \
        go install github.com/air-verse/air@v1.67.4 \
    && rm -rf /tmp/gomod /tmp/gocache /go/pkg
