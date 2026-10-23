# NetKKC

Kana-Kanji conversion for embedded and edge devices over pluggable transports.

## Project Concept

- Provide kana-kanji conversion through an API server backed by `mozc_server`.
- Use HTTP/HTTPS for the initial release, while keeping transports replaceable
  for future extensions.
- Keep TLS termination flexible: either the API server or a load balancer/reverse
  proxy can handle HTTPS.

## Initial Implementation Scope

- Start with a single kana-kanji conversion API.
- Accept a single string as input and segment it into bunsetsu (phrases) on the
  server side.
- Return an array of the segmented bunsetsu, with an array of conversion
  candidates for each bunsetsu.
- Provide `GET /healsh` for health checks, returning HTTP 200 with a `text/plain`
  body of `OK`.

## Development Environment

The API contract is defined in `openapi.yaml` using OpenAPI 3.1. See `SPEC.md`
for conversion behavior. Models and Echo server interfaces are generated with
oapi-codegen v2.8.0. Use Go 1.27.1 as pinned in `mise.toml`.

```sh
make generate          # Generate Go models and Echo server interfaces.
make build             # Build bin/kkc-server.
make test              # Run unit tests.
make run               # Build/start Docker Mozc, check readiness, then start the API on 8080.
make test-integration  # Run all tests, including real Mozc and HTTP round-trip tests.
make mozc-down         # Stop the Mozc container; retain its profile.
```

`POST /convert` converts a reading. For example:

```sh
curl -sS -H 'Content-Type: application/json' \
  --data '{"reading":"こんにちは、きょうはいいお天気ですね"}' \
  http://127.0.0.1:8080/convert
```

Mozc runs in Docker while the Go API runs on the Linux host, sharing the host
network and a dedicated Mozc profile directory. This requires a Linux Docker
Engine; Docker Desktop on other platforms is not covered. The binary accepts
`-mozc-profile /path/to/profile` and `-listen :8080`.

See `AGENTS.md` for detailed development notes and the current implementation
status.

## Planned

- Run the API server and `mozc_server` together in a VS Code Dev Container.
- Isolate the runtime, build dependencies, and development tools from the host
  to keep the environment reproducible.
- Use `.devcontainer/devcontainer.json` and a single Dockerfile. No database or
  other separate services are planned initially, so Docker Compose is not needed.
- Consider packaging the API server and `mozc_server` in the same container for
  deployment as well.
