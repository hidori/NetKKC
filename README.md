# NetKKC

Kana-Kanji conversion for embedded and edge devices over pluggable transports.

## Project Concept

- Provide kana-kanji conversion through an API server backed by `mozc_server`.
- Use HTTP/HTTPS for the initial release, while keeping transports replaceable
  for future extensions.
- Keep TLS termination flexible: either the API server or a load balancer/reverse
  proxy can handle HTTPS.

## Development Environment

- Run the API server and `mozc_server` together in a VS Code Dev Container.
- Isolate the runtime, build dependencies, and development tools from the host
  to keep the environment reproducible.
- Use `.devcontainer/devcontainer.json` and a single Dockerfile. No database or
  other separate services are planned initially, so Docker Compose is not needed.
- Consider packaging the API server and `mozc_server` in the same container for
  deployment as well.
