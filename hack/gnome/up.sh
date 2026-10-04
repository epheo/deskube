#!/usr/bin/env bash
# Boots a GNOME desktop from a DataSource: ./up.sh <name> <base-port> [datasource]
set -euo pipefail
cd "$(dirname "$0")"
NAME=${1:?name}; PORT=${2:?base port}; IMAGE=${3:-desktop-gnome-rawhide}
NS=${NS:-deskube-dev}
KEY=${KEY:-$HOME/.ssh/deskube-dev.pub}
mkdir -p .rendered
PASS_FILE=.rendered/$NAME-sunshine-admin-password
[ -f "$PASS_FILE" ] || openssl rand -hex 12 > "$PASS_FILE"
render() {
  sed -e "s|__NAME__|$NAME|g" -e "s|__PORT__|$PORT|g" -e "s|__IMAGE__|$IMAGE|g" \
      -e "s|__P_HTTPS__|$((PORT-5))|g" -e "s|__P_WEB__|$((PORT+1))|g" \
      -e "s|__P_VIDEO__|$((PORT+9))|g" -e "s|__P_CONTROL__|$((PORT+10))|g" \
      -e "s|__P_AUDIO__|$((PORT+11))|g" -e "s|__P_RTSP__|$((PORT+21))|g" \
      -e "s|__SSH_PUBKEY__|$(cat "$KEY")|" -e "s|__SUNSHINE_PASS__|$(cat "$PASS_FILE")|" "$1"
}
render cloud-init.yaml.tpl > ".rendered/$NAME-cloud-init.yaml"
render vm.yaml.tpl > ".rendered/$NAME-vm.yaml"
oc -n "$NS" create secret generic "$NAME-cloudinit" \
  --from-file=userdata=".rendered/$NAME-cloud-init.yaml" --dry-run=client -o yaml | oc -n "$NS" apply -f -
oc -n "$NS" apply -f ".rendered/$NAME-vm.yaml"
