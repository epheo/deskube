#!/usr/bin/env bash
# Operator level proof: a VdiInstance becomes a desktop pod, its
# spec.pairing carries the PIN, and a headless Moonlight receives video.
# Needs the manager running (make run or a deployment) and the
# moonlight-cli image from the desku.be-images repository.
set -euo pipefail
NS=${NS:-deskube-dev}
NAME=${NAME:-desk-op}
IMAGE=${IMAGE:-ghcr.io/epheo/deskube-desktop:main}
OWNER=${OWNER:-$(oc whoami)}
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
  image: $IMAGE
  imagePullPolicy: Always
  owner: $OWNER
  resources:
    requests: {cpu: "2", memory: 4Gi}
    limits: {memory: 4Gi}
EOF
for _ in $(seq 1 60); do
  phase=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  addr=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.address}' 2>/dev/null || true)
  echo "  phase=$phase address=$addr"
  [ "$phase" = Ready ] && break
  sleep 5
done
[ "$phase" = Ready ] || { oc -n "$NS" get vdiinstance "$NAME" -o yaml | sed -n '/^status:/,$p'; exit 1; }
HOST=${addr%:*}; PORT=${addr##*:}

echo "== pair through spec.pairing"
ml pair -port "$PORT" "$HOST" > "$WORK/pair.log" 2>&1 &
PAIR=$!
PIN=""
for _ in $(seq 1 30); do
  PIN=$(grep -o -E "PIN[^0-9]*[0-9]{4}" "$WORK/pair.log" | grep -o -E "[0-9]{4}$" | head -1 || true)
  [ -n "$PIN" ] && break
  sleep 1
done
[ -n "$PIN" ] || { cat "$WORK/pair.log"; echo "no PIN from client"; exit 1; }
sleep 2
if grep -q "Already paired" "$WORK/pair.log"; then
  # The pairing lives in the home volume and outlives the pod.
  echo "  client already paired, nothing to approve"
else
  oc -n "$NS" patch vdiinstance "$NAME" --type=merge \
    -p "{\"spec\":{\"pairing\":{\"pin\":\"$PIN\",\"clientName\":\"e2e\"}}}"
  # status.pairing may still report an earlier PIN.
  for _ in $(seq 1 30); do
    ppin=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.pairing.pin}' 2>/dev/null || true)
    pphase=$(oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.pairing.phase}' 2>/dev/null || true)
    [ "$ppin" = "$PIN" ] && { [ "$pphase" = Paired ] || [ "$pphase" = Failed ]; } && break
    sleep 2
  done
  oc -n "$NS" get vdiinstance "$NAME" -o jsonpath='{.status.pairing}{"\n"}'
  wait $PAIR || true
  grep -q "Succesfully paired" "$WORK/pair.log" || { cat "$WORK/pair.log"; exit 1; }
fi

echo "== stream (25s)"
timeout 25 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i --entrypoint stdbuf moonlight-cli \
  -oL -eL moonlight stream -port "$PORT" -platform fake -viewonly -app Desktop \
  -width 1920 -height 1080 -fps 60 -bitrate 10000 -verbose "$HOST" > "$WORK/stream.log" 2>&1 || true
timeout 20 podman run --rm --network host -v "$CACHE:/root/.cache/moonlight:Z" -i moonlight-cli quit -port "$PORT" "$HOST" > /dev/null 2>&1 || true
grep -E "first audio packet|Video frames" "$WORK/stream.log"
echo "== session state seen by the operator"
sleep 3
oc -n "$NS" get vdiinstance "$NAME"
echo "logs in $WORK"
# The first packet proves little; count what arrived. A still desktop sends few frames.
GOT=$(sed -n 's/.*Video frames received: .* s, \([0-9]*\)\..* fps.*/\1/p' "$WORK/stream.log" | head -1)
if [ "${GOT:-0}" -ge "${MIN_FPS:-5}" ]; then echo "RESULT: PASS"; else echo "RESULT: FAIL, ${GOT:-no} fps received"; exit 1; fi
