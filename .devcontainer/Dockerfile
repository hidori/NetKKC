FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN export GOBIN=/usr/local/bin GOMODCACHE=/tmp/gomod GOCACHE=/tmp/gocache \
    && go install github.com/air-verse/air@v1.67.4 \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 \
    && rm -rf /tmp/gomod /tmp/gocache /go/pkg
