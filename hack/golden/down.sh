#!/usr/bin/env bash
set -euo pipefail
NAME=${1:?name}; NS=${NS:-deskube-dev}
oc -n "$NS" delete vm,svc "$NAME" --ignore-not-found
oc -n "$NS" delete secret "$NAME-cloudinit" --ignore-not-found
