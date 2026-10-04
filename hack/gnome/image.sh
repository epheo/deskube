#!/usr/bin/env bash
# Turns a bootc OCI image into a DataSource without any privileged build:
# a builder VM boots Fedora CoreOS, switches to the image, powers off,
# and its disk is snapshotted. ./image.sh <datasource-name> <oci-ref>
set -euo pipefail
cd "$(dirname "$0")"
NAME=${1:?datasource name}; REF=${2:?oci image ref}
NS=${NS:-deskube-dev}
BASE=${BASE:-fcos-base}
KEY=${KEY:-$HOME/.ssh/deskube-dev.pub}
AUTH=${AUTH:-$HOME/.config/containers/auth.json}
BUILDER="$NAME-builder"
mkdir -p .rendered

# The registry is private, the guest needs the pull credentials.
REGISTRY=${REF%%/*}
AUTH_B64=$(jq -c --arg r "$REGISTRY" '{auths: {($r): .auths[$r]}}' "$AUTH" | base64 -w0)
UNIT=$(cat <<UNIT
[Unit]
Description=Switch to the DeskuBe desktop image and power off
After=network-online.target
Wants=network-online.target
ConditionPathExists=!/var/lib/deskube-switched

[Service]
Type=oneshot
# A multi gigabyte pull over a flaky link deserves a few attempts.
ExecStart=/bin/sh -c 'for i in 1 2 3 4 5; do bootc switch $REF && exit 0; sleep 30; done; exit 1'
# Nothing instance specific may ship in the image: the CoreOS admin user,
# the builder's hostname and machine id, and the SSH host keys.
ExecStart=/usr/sbin/userdel -r core
ExecStart=/bin/sh -c 'rm -f /etc/hostname /etc/ssh/ssh_host_*; truncate -s 0 /etc/machine-id'
ExecStart=/usr/bin/touch /var/lib/deskube-switched
ExecStart=/usr/bin/systemctl poweroff

[Install]
WantedBy=multi-user.target
UNIT
)
jq -n --arg key "$(cat "$KEY")" --arg auth "$AUTH_B64" --arg unit "$UNIT" '{
  ignition: {version: "3.4.0"},
  passwd: {users: [{name: "core", sshAuthorizedKeys: [$key]}]},
  storage: {files: [{path: "/etc/ostree/auth.json", mode: 384, overwrite: true,
                     contents: {source: ("data:text/plain;base64," + $auth)}}]},
  systemd: {units: [{name: "deskube-switch.service", enabled: true, contents: $unit}]}
}' > ".rendered/$BUILDER-ignition.json"

oc -n "$NS" create secret generic "$BUILDER-ignition" \
  --from-file=userdata=".rendered/$BUILDER-ignition.json" --dry-run=client -o yaml | oc -n "$NS" apply -f -

oc -n "$NS" apply -f - <<VM
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: $BUILDER
spec:
  # A guest poweroff must leave the VM stopped.
  runStrategy: RerunOnFailure
  instancetype:
    kind: VirtualMachineClusterInstancetype
    name: u1.large
  dataVolumeTemplates:
    - metadata:
        name: $BUILDER-root
        annotations:
          cdi.kubevirt.io/cloneType: snapshot
      spec:
        source:
          pvc:
            name: $BASE
            namespace: $NS
        storage:
          resources:
            requests:
              storage: 30Gi
  template:
    spec:
      domain:
        devices:
          disks:
            - name: rootdisk
              disk:
                bus: virtio
            - name: ignition
              disk:
                bus: virtio
          interfaces:
            - name: default
              masquerade: {}
      networks:
        - name: default
          pod: {}
      volumes:
        - name: rootdisk
          dataVolume:
            name: $BUILDER-root
        - name: ignition
          cloudInitConfigDrive:
            secretRef:
              name: $BUILDER-ignition
VM

echo "waiting for the builder to switch and power off"
for _ in $(seq 1 120); do
  status=$(oc -n "$NS" get vm "$BUILDER" -o jsonpath='{.status.printableStatus}')
  echo "  $(date +%T) $status"
  [ "$status" = Stopped ] && break
  sleep 10
done
[ "$status" = Stopped ] || { echo "builder did not stop"; exit 1; }

oc -n "$NS" delete datasource/"$NAME" datavolume/"$NAME" --ignore-not-found
oc -n "$NS" apply -f - <<DV
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataVolume
metadata:
  name: $NAME
  annotations:
    cdi.kubevirt.io/cloneType: snapshot
    cdi.kubevirt.io/storage.bind.immediate.requested: "true"
spec:
  source:
    pvc:
      name: $BUILDER-root
      namespace: $NS
  storage:
    resources:
      requests:
        storage: 30Gi
---
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataSource
metadata:
  name: $NAME
spec:
  source:
    pvc:
      name: $NAME
      namespace: $NS
DV
oc -n "$NS" wait "datavolume/$NAME" --for=condition=Ready --timeout=300s
oc -n "$NS" delete "vm/$BUILDER" "secret/$BUILDER-ignition" --ignore-not-found
echo "datasource $NAME ready from $REF"
