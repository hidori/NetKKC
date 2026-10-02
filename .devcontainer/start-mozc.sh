#!/usr/bin/env bash
set -euo pipefail

if ! pgrep -u "$(id -u)" -x mozc_server > /dev/null; then
    nohup /usr/lib/mozc/mozc_server >> "/tmp/mozc_server-$(id -u).log" 2>&1 < /dev/null &
fi
