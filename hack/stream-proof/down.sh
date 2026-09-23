#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
NS=${NS:-deskube-dev}
oc -n "$NS" delete -f vm.yaml --ignore-not-found
oc -n "$NS" delete secret desk-proof-cloudinit --ignore-not-found
