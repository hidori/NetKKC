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
- Remove the Hello, World! endpoint before starting conversion API implementation.
- Provide `GET /healsh` for health checks, returning HTTP 200 with a `text/plain`
  body of `OK`.

## Development Environment

The API contract is defined in `openapi.yaml` using OpenAPI 3.1. See `SPEC.md`
for conversion behavior. Models and Echo server interfaces are generated with
oapi-codegen v2.8.0; edit the definition and regenerate instead of editing
`internal/api/openapi.gen.go` directly.

Use Go 1.27.1 as pinned in `mise.toml`. In the prepared cloud environment, Make
automatically uses the installed Go and caches under `/workspace/.netkkc` if
the `go` on `PATH` is not a working Go compiler; no shell activation is needed.
To select another Go executable explicitly, pass `GO=/path/to/go` to Make.

```sh
make generate # Generate Go models and Echo server interfaces.
make build    # Build bin/kkc-server.
make test     # Run all Go tests.
make run      # Build and start the server on port 8080.
```

The generated conversion interface is not yet wired into the application;
the running server currently provides only `GET /healsh`.

- Run the API server and `mozc_server` together in a VS Code Dev Container.
- Isolate the runtime, build dependencies, and development tools from the host
  to keep the environment reproducible.
- Use `.devcontainer/devcontainer.json` and a single Dockerfile. No database or
  other separate services are planned initially, so Docker Compose is not needed.
- Consider packaging the API server and `mozc_server` in the same container for
  deployment as well.
