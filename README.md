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

The API contract is defined in `openapi.yaml` using OpenAPI 3.1. See
`doc/SPEC.md` for design policy. Models and Echo server interfaces are
generated with oapi-codegen v2.8.0. Use Go 1.27.1 as pinned in `mise.toml`.

Open the repository in the VS Code Dev Container. Docker Compose (`compose.yaml`)
starts three services that share one network namespace:

- `dev`: the development container.
- `api`: the API server with hot reload (air), published on `127.0.0.1:8080`.
- `mozc`: a dedicated `mozc_server`. Its profile is `.local/mozc`.

To start or stop the services without the Dev Container, run `make up` or
`make down` on the host.

In the `dev` container:

```sh
make generate          # Generate Go models and Echo server interfaces.
make build             # Build bin/kkc-server.
make lint              # Run golangci-lint.
make lint-fix          # Run golangci-lint and apply automatic fixes.
make test              # Run all tests, including real Mozc and HTTP round trips.
```

`POST /convert` converts a reading, and `POST /reconvert` converts again with
specified segments. For example:

```sh
curl -sS -H 'Content-Type: application/json' \
  --data '{"reading":"こんにちは、きょうはいいお天気ですね"}' \
  http://127.0.0.1:8080/convert
```

See `AGENTS.md` for detailed development notes and the current implementation
status.

## Planned

- Consider packaging the API server and `mozc_server` in the same container for
  deployment.
