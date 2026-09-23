#!/usr/bin/env bash
# Forwards every Sunshine port to localhost through the API server.
# Point Moonlight at 127.0.0.1:31989. Slow, but it needs no firewall change.
set -euo pipefail
NS=${NS:-deskube-dev}
exec virtctl port-forward "vm/desk-proof/$NS" \
  31984 31989 31990 32010 udp/31998 udp/31999 udp/32000
