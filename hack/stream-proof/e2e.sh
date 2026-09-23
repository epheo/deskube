#!/usr/bin/env bash
# End to end proof without a human: tunnel, pair through the Sunshine admin
# API, then start a stream with a headless Moonlight and look for the RTSP
# and video handshake in its log.
set -euo pipefail
cd "$(dirname "$0")"
NS=${NS:-deskube-dev}
HOST=127.0.0.1
PORT=31989
WEB=31990
PASS=$(cat .rendered/sunshine-admin-password)
WORK=${WORK:-$(mktemp -d)}
CACHE=$WORK/moonlight-cache
mkdir -p "$CACHE"

ml() { podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i moonlight-cli "$@"; }
api() { curl -sk -u "admin:$PASS" -H 'Content-Type: application/json' "$@"; }

virtctl port-forward "vm/desk-proof/$NS" 31984 31989 31990 32010 udp/31998 udp/31999 udp/32000 \
  > "$WORK/tunnel.log" 2>&1 &
TUNNEL=$!
trap 'kill $TUNNEL 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do api "https://$HOST:$WEB/api/config" -o /dev/null && break; sleep 1; done

echo "== serverinfo"
curl -s "http://$HOST:$PORT/serverinfo" | tr '<>' '\n\n' | grep -E -A1 "^(hostname|HttpsPort|ExternalPort|PairStatus|state)$" | grep -v -E "^(-|hostname|HttpsPort|ExternalPort|PairStatus|state)$" | paste - - - - -

echo "== pair"
ml pair -port $PORT $HOST > "$WORK/pair.log" 2>&1 &
PAIR=$!
PIN=""
for _ in $(seq 1 30); do
  PIN=$(grep -o -E "PIN[^0-9]*[0-9]{4}" "$WORK/pair.log" | grep -o -E "[0-9]{4}$" | head -1 || true)
  [ -n "$PIN" ] && break
  sleep 1
done
[ -n "$PIN" ] || { cat "$WORK/pair.log"; echo "no PIN from client"; exit 1; }
PENDING=$(api "https://$HOST:$WEB/api/pin")
echo "pending: $PENDING"
ID=$(echo "$PENDING" | python3 -c 'import sys,json; d=json.load(sys.stdin); p=d.get("pending") or d.get("pairings") or d; print((p[0] if isinstance(p,list) and p else {}).get("id",""))')
if [ -n "$ID" ]; then BODY="{\"pairing_id\":\"$ID\",\"pin\":\"$PIN\",\"name\":\"e2e\"}"; else BODY="{\"pin\":\"$PIN\",\"name\":\"e2e\"}"; fi
api -X POST "https://$HOST:$WEB/api/pin" -d "$BODY"; echo
wait $PAIR || true
tail -3 "$WORK/pair.log"

echo "== apps"
ml list -port $PORT $HOST 2>&1 | tail -5

echo "== stream (25s, view only: the headless client hangs on input setup without /dev/input)"
timeout 25 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i --entrypoint stdbuf moonlight-cli \
  -oL -eL moonlight stream -port $PORT -platform fake -viewonly -app Desktop \
  -width 1280 -height 720 -fps 30 -bitrate 3000 -verbose $HOST > "$WORK/stream.log" 2>&1 || true
timeout 20 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i moonlight-cli quit -port $PORT $HOST > /dev/null 2>&1 || true
grep -E "RTSP|port:|Starting (control|video|audio) stream|first (video|audio) packet" "$WORK/stream.log"
echo "logs in $WORK"
if grep -q "Received first video packet" "$WORK/stream.log"; then echo "RESULT: PASS, video packets received"; else echo "RESULT: FAIL"; exit 1; fi
