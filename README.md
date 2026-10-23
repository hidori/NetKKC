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
make test     # Run unit tests (live Mozc tests require the profile below).
make run      # Build/start Docker Mozc, check readiness, then start the API on 8080.
```

`POST /convert` is implemented against the generated Echo interface. For example:

```sh
curl -sS -H 'Content-Type: application/json' \
  --data '{"reading":"こんにちは、きょうはいいお天気ですね"}' \
  http://127.0.0.1:8080/convert
```

Mozc runs in Docker while the Go API runs on the Linux host. Host networking and
a shared profile directory expose Mozc's abstract Unix socket and discovery file;
there is no TCP bridge. This workflow requires a Linux Docker Engine whose host
network namespace and bind-mount paths are shared with the Go process. The cloud
environment has been verified with this configuration; Docker Desktop on other
platforms is not covered by this setup.

```sh
make mozc-up           # Build/start the dedicated Mozc container and verify IPC/config.
make test-integration  # Run all tests, including real Mozc and HTTP round-trip tests.
make mozc-down         # Stop this workspace's container; retain its profile.
```

`make mozc-up` is repeatable. An image update recreates only the container owned
by this workspace. `make clean` removes the API binary and does not stop Mozc.
The default profile is `/workspace/.netkkc/docker-mozc/mozc` in the cloud, or
`.local/mozc/mozc` in another checkout. The profile is dedicated to NetKKC and is
configured with incognito mode and history disabled. Do not point it at a desktop
IME profile: startup updates the Mozc settings for this profile.

Override `MOZC_STATE_DIR` (absolute path), `MOZC_DOCKER_IMAGE`, or
`MOZC_DOCKER_CONTAINER` as Make variables when needed. The binary also accepts
`-mozc-profile /path/to/profile` and `-listen :8080`. If using an externally started
dedicated Mozc server, run the binary directly after `make build`.

The Docker build reuses existing proxy settings, including proxy hostname
resolution needed in this cloud image. Proxy credentials are not saved in the
Dockerfile or image configuration. The Docker CLI uses writable configuration
under `/workspace/.netkkc/docker-config` in the cloud.

`GET /healsh` returns `OK`. The server waits for a usable Mozc connection before
listening. If Mozc becomes unavailable later, conversion returns HTTP 503;
health checks continue to report HTTP server availability.

- Run the API server and `mozc_server` together in a VS Code Dev Container.
- Isolate the runtime, build dependencies, and development tools from the host
  to keep the environment reproducible.
- Use `.devcontainer/devcontainer.json` and a single Dockerfile. No database or
  other separate services are planned initially, so Docker Compose is not needed.
- Consider packaging the API server and `mozc_server` in the same container for
  deployment as well.
