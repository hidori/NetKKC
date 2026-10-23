#!/bin/sh
# Run the command as the owner of the workspace (the working directory).
# Mozc's discovery file is readable only by its owner, so Mozc and its
# clients must share one UID.
set -eu
exec setpriv --reuid="$(stat -c %u .)" --regid="$(stat -c %g .)" --clear-groups "$@"
