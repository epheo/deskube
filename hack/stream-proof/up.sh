#!/usr/bin/env bash
# Renders the cloud-init with a local SSH key and a random Sunshine admin
# password, then applies the VM and its NodePort Service.
set -euo pipefail
cd "$(dirname "$0")"
NS=${NS:-deskube-dev}
KEY=${KEY:-$HOME/.ssh/deskube-dev.pub}
SUNSHINE_VERSION=${SUNSHINE_VERSION:-2026.914.233613}

mkdir -p .rendered
PASS_FILE=.rendered/sunshine-admin-password
[ -f "$PASS_FILE" ] || openssl rand -hex 12 > "$PASS_FILE"

sed -e "s|__SSH_PUBKEY__|$(cat "$KEY")|" \
    -e "s|__SUNSHINE_PASS__|$(cat "$PASS_FILE")|" \
    -e "s|__SUNSHINE_VERSION__|$SUNSHINE_VERSION|g" \
    cloud-init.yaml.tpl > .rendered/cloud-init.yaml

oc -n "$NS" create secret generic desk-proof-cloudinit \
  --from-file=userdata=.rendered/cloud-init.yaml \
  --dry-run=client -o yaml | oc -n "$NS" apply -f -
oc -n "$NS" apply -f vm.yaml
