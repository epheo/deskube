#!/usr/bin/env bash
# SSH into the proof VM through the API server, no NodePort needed.
NS=${NS:-deskube-dev}
exec ssh -i "${KEY:-$HOME/.ssh/deskube-dev}" \
  -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
  -o ProxyCommand="virtctl port-forward --stdio=true vm/desk-proof/$NS 22" \
  desk@desk-proof "$@"
