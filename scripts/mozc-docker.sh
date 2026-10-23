#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"

# The cloud image has a read-only home; keep Docker CLI metadata writable.
if [[ -d /workspace/.netkkc ]]; then
    export DOCKER_CONFIG="${DOCKER_CONFIG:-/workspace/.netkkc/docker-config}"
    mkdir -p "$DOCKER_CONFIG"
fi

docker_command=${DOCKER:-docker}
image=${MOZC_DOCKER_IMAGE:-netkkc-mozc:dev}
container=${MOZC_DOCKER_CONTAINER:-netkkc-mozc-$(id -u)}
state_dir=$(realpath -m "${MOZC_STATE_DIR:-$repo_root/.local/mozc}")

check_owned_container() {
    local workspace profile
    workspace=$("$docker_command" inspect --format '{{index .Config.Labels "dev.netkkc.workspace"}}' "$container")
    profile=$("$docker_command" inspect --format '{{index .Config.Labels "dev.netkkc.state"}}' "$container")
    if [[ "$workspace" != "$repo_root" || "$profile" != "$state_dir" ]]; then
        echo "Container $container belongs to another workspace or profile; choose MOZC_DOCKER_CONTAINER." >&2
        exit 1
    fi
}

case "${1:-}" in
    build)
        network_args=()
        # A cloud proxy hostname may be defined only in the outer machine's
        # hosts file. Carry its resolved address into the build, not credentials.
        # Docker's predefined proxy build args are not stored in image history.
        if command -v python3 >/dev/null 2>&1; then
            proxy_hosts=$(python3 - <<'PY'
import os, socket, urllib.parse
seen = set()
for name in ('http_proxy', 'https_proxy', 'HTTP_PROXY', 'HTTPS_PROXY'):
    value = os.environ.get(name)
    if not value:
        continue
    hostname = urllib.parse.urlsplit(value).hostname
    if not hostname or hostname in seen:
        continue
    seen.add(hostname)
    address = socket.getaddrinfo(hostname, None, socket.AF_INET, socket.SOCK_STREAM)[0][4][0]
    print(hostname + ':' + address)
PY
)
            while IFS= read -r mapping; do
                [[ -z "$mapping" ]] || network_args+=(--add-host "$mapping")
            done <<< "$proxy_hosts"
        fi
        # apt uses lowercase proxy names. Reuse existing injected bindings.
        export http_proxy="${http_proxy:-${HTTP_PROXY:-}}"
        export https_proxy="${https_proxy:-${HTTPS_PROXY:-}}"
        export no_proxy="${no_proxy:-${NO_PROXY:-}}"
        "$docker_command" build --network host "${network_args[@]}" \
            --build-arg http_proxy --build-arg https_proxy --build-arg no_proxy \
            --file docker/mozc.Dockerfile --tag "$image" docker
        ;;
    start)
        mkdir -p "$state_dir/mozc"
        if "$docker_command" container inspect "$container" >/dev/null 2>&1; then
            check_owned_container
            current_image=$("$docker_command" inspect --format '{{.Image}}' "$container")
            desired_image=$("$docker_command" image inspect --format '{{.Id}}' "$image")
            if [[ "$current_image" != "$desired_image" ]]; then
                "$docker_command" rm --force "$container" >/dev/null
            fi
        fi
        if "$docker_command" container inspect "$container" >/dev/null 2>&1; then
            "$docker_command" start "$container"
        else
            # Linux abstract Unix sockets are scoped to the network namespace.
            # Host networking + shared discovery/profile files lets host Go
            # talk directly to Mozc; no HTTP/TCP bridge is introduced.
            "$docker_command" run --detach --name "$container" --network host \
                --user "$(id -u):$(id -g)" \
                --label "dev.netkkc.workspace=$repo_root" \
                --label "dev.netkkc.state=$state_dir" \
                --mount "type=bind,source=$state_dir,target=/var/lib/netkkc" \
                "$image"
        fi
        ;;
    stop)
        if "$docker_command" container inspect "$container" >/dev/null 2>&1; then
            check_owned_container
            "$docker_command" stop "$container"
        fi
        ;;
    *)
        echo "Usage: $0 {build|start|stop}" >&2
        exit 2
        ;;
esac
