#!/usr/bin/env bash
# Re-snapshots the golden image from the proof VM.
# ./rebuild.sh [customize.sh]  runs the given script inside the VM first.
set -euo pipefail
cd "$(dirname "$0")"
NS=${NS:-deskube-dev}
VM=${VM:-desk-proof}
KEY=${KEY:-$HOME/.ssh/deskube-dev}
CUSTOMIZE=${1:-}

vmssh() {
  ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    -o ConnectTimeout=10 -o ProxyCommand="virtctl port-forward --stdio=true vm/$VM/$NS 22" "desk@$VM" "$@"
}

virtctl start "$VM" -n "$NS" || true
oc -n "$NS" wait "vm/$VM" --for=condition=Ready --timeout=280s
for _ in $(seq 1 40); do vmssh true 2>/dev/null && break; sleep 5; done

if [ -n "$CUSTOMIZE" ]; then
  vmssh 'cat > /tmp/customize.sh' < "$CUSTOMIZE"
  vmssh 'bash /tmp/customize.sh'
fi

# Instance specific state must not land in the image.
vmssh 'systemctl --user stop sunshine xfce; sudo systemctl stop xorg-dummy;
  rm -f ~/.config/sunshine/sunshine_state.json ~/.config/sunshine/sunshine.log;
  sudo rm -f /var/lib/cloud/deskube-provisioned;
  sudo cloud-init clean --logs --machine-id; sudo rm -f /etc/ssh/ssh_host_*; sudo sync; echo cleaned'

virtctl stop "$VM" -n "$NS"
oc -n "$NS" wait "vmi/$VM" --for=delete --timeout=180s
oc -n "$NS" delete datavolume/deskube-fedora-xfce --ignore-not-found
oc -n "$NS" apply -f golden.yaml
oc -n "$NS" wait datavolume/deskube-fedora-xfce --for=condition=Ready --timeout=280s
echo "golden image rebuilt $(date +%T)"
