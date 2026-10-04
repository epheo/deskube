#!/usr/bin/env bash
# Runs a shell command inside a VM through the qemu guest agent, for
# guests that have no network yet. ./gexec.sh <vm> '<command>'
set -euo pipefail
VM=${1:?vm}; CMD=${2:?command}
NS=${NS:-deskube-dev}
POD=$(oc -n "$NS" get pod -l "vm.kubevirt.io/name=$VM" -o jsonpath='{.items[0].metadata.name}')
VIRSH="virsh -c qemu+unix:///session?socket=/var/run/libvirt/virtqemud-sock"
DOM="${NS}_${VM}"
REQ=$(jq -cn --arg c "$CMD" '{execute:"guest-exec",arguments:{path:"/bin/sh",arg:["-c",$c],"capture-output":true}}')
PID=$(oc -n "$NS" exec "$POD" -c compute -- $VIRSH qemu-agent-command "$DOM" "$REQ" | jq -r '.return.pid')
for _ in $(seq 1 60); do
  OUT=$(oc -n "$NS" exec "$POD" -c compute -- $VIRSH qemu-agent-command "$DOM" "{\"execute\":\"guest-exec-status\",\"arguments\":{\"pid\":$PID}}")
  echo "$OUT" | jq -e '.return.exited' >/dev/null && break
  sleep 1
done
echo "$OUT" | jq -r '.return["out-data"] // "" | @base64d'
echo "$OUT" | jq -r '.return["err-data"] // "" | @base64d' | sed 's/^/ERR: /' | grep -v '^ERR: $' || true
