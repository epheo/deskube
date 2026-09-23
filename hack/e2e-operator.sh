#!/usr/bin/env bash
# Operator level proof: a VdiInstance becomes a desktop, a VdiPairing
# carries the PIN, and a headless Moonlight receives video.
# Needs the manager running (make run or a deployment) and the
# moonlight-cli image from hack/moonlight-cli.
set -euo pipefail
NS=${NS:-deskube-dev}
NAME=${NAME:-desk-op}
IMAGE=${IMAGE:-deskube-fedora-xfce}
OWNER=${OWNER:-$(oc whoami)}
KEY=${KEY:-$HOME/.ssh/deskube-dev.pub}
WORK=${WORK:-$(mktemp -d)}
CACHE=$WORK/moonlight-cache
mkdir -p "$CACHE"

ml() { podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i moonlight-cli "$@"; }

echo "== instance"
oc -n "$NS" apply -f - <<EOF
apiVersion: vdi.desku.be/v1alpha1
kind: VdiInstance
metadata:
  name: $NAME
spec:
  image:
    dataSource:
      name: $IMAGE
  owner: $OWNER
  sshAuthorizedKeys:
    - $(cat "$KEY")
EOF
for _ in $(seq 1 60); do
  phase=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  addr=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.address}' 2>/dev/null || true)
  echo "  phase=$phase address=$addr"
  [ "$phase" = Ready ] && break
  sleep 5
done
[ "$phase" = Ready ] || { oc -n "$NS" get vdiinstance "$NAME" -o yaml | sed -n '/status:/,$p'; exit 1; }
HOST=${addr%:*}; PORT=${addr##*:}

echo "== pair through a VdiPairing"
ml pair -port "$PORT" "$HOST" > "$WORK/pair.log" 2>&1 &
PAIR=$!
PIN=""
for _ in $(seq 1 30); do
  PIN=$(grep -o -E "PIN[^0-9]*[0-9]{4}" "$WORK/pair.log" | grep -o -E "[0-9]{4}$" | head -1 || true)
  [ -n "$PIN" ] && break
  sleep 1
done
[ -n "$PIN" ] || { cat "$WORK/pair.log"; echo "no PIN from client"; exit 1; }
oc -n "$NS" delete vdipairing "$NAME-e2e" --ignore-not-found > /dev/null
oc -n "$NS" apply -f - <<EOF
apiVersion: vdi.desku.be/v1alpha1
kind: VdiPairing
metadata:
  name: $NAME-e2e
spec:
  instanceRef: $NAME
  pin: "$PIN"
  clientName: e2e
EOF
for _ in $(seq 1 30); do
  pphase=$(oc -n "$NS" get vdipairing "$NAME-e2e" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  [ "$pphase" = Paired ] || [ "$pphase" = Failed ] && break
  sleep 2
done
oc -n "$NS" get vdipairing "$NAME-e2e"
wait $PAIR || true
grep -q "Succesfully paired" "$WORK/pair.log" || { cat "$WORK/pair.log"; exit 1; }

echo "== stream (25s)"
timeout 25 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i --entrypoint stdbuf moonlight-cli \
  -oL -eL moonlight stream -port "$PORT" -platform fake -viewonly -app Desktop \
  -width 1280 -height 720 -fps 30 -bitrate 3000 -verbose "$HOST" > "$WORK/stream.log" 2>&1 || true
timeout 20 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i moonlight-cli quit -port "$PORT" "$HOST" > /dev/null 2>&1 || true
grep -E "RTSP|Starting (control|video|audio) stream|first (video|audio) packet" "$WORK/stream.log"
echo "== session state seen by the operator"
sleep 3
oc -n "$NS" get vdiinstance "$NAME"
echo "logs in $WORK"
if grep -q "Received first video packet" "$WORK/stream.log"; then echo "RESULT: PASS"; else echo "RESULT: FAIL"; exit 1; fi
