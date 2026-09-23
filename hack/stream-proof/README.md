# Stream proof

One Fedora VM with Sunshine, no operator involved.
Goal: a Moonlight client streams the desktop through the cluster.

    ./up.sh          # VM + NodePort Service in deskube-dev
    ./tunnel.sh      # local ports via the API server, then Moonlight -> 127.0.0.1:31989
    ./down.sh

Sunshine admin password: `.rendered/sunshine-admin-password`.
Web UI: https://127.0.0.1:31990 through the tunnel, or https://<node-ip>:31990
once the node firewall allows the NodePort range.

Ports are pinned to base 31989 so the Service maps them 1:1:

    31984 tcp https   31989 tcp http   31990 tcp web ui   32010 tcp rtsp
    31998 udp video   31999 udp control   32000 udp audio

## Result 2026-09-23

`./e2e.sh` passes through the tunnel: pairing via `GET/POST /api/pin`,
RTSP handshake, control, video, and audio streams, first video packet
after 100 ms. Sunshine encodes 1080p capture to 720p30 with libx264 at
about 15 ms per frame on 4 vCPU, no GPU.

Known gaps:
- No audio. Sunshine cannot set its null sink as default on PipeWire.
- The guest Sunshine port must equal the external port. RTSP advertises
  the UDP ports, so a Service cannot remap them.
- The node firewall drops NodePorts, so the direct path is untested.
- Provisioning takes about five minutes. A golden image fixes that.
