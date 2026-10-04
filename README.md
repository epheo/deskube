# DeskuBe

Virtual desktops on KubeVirt, streamed with Sunshine and Moonlight.
No GPU needed, no agent in the client beyond Moonlight itself.

DeskuBe is an operator for OpenShift Virtualization and KubeVirt.
It boots a desktop VM from a golden image, exposes the seven Sunshine
ports, and approves Moonlight pairing on the owner's behalf.

## How it works

    VdiInstance ──> VirtualMachine (snapshot clone of the golden image)
                ──> Service (7 ports, NodePort or LoadBalancer)
                ──> Secrets (Sunshine admin password, cloud-init)
                ──> Role + RoleBinding for the owner

    VdiPairing  ──> POST /api/pin on the desktop's Sunshine web UI

Sunshine runs inside the guest on a base port the operator allocates.
Every other port is derived from it, and RTSP tells Moonlight the UDP
ports, so the guest and the Service use the same numbers.

Base port 31989 gives: 31984 https, 31989 http, 31990 web UI,
31998 video, 31999 control, 32000 audio, 32010 rtsp.

## Quick start

1. Build a golden image once. `hack/stream-proof` provisions a Fedora
   VM with Xorg dummy, XFCE, and Sunshine; `hack/golden` snapshots it
   into a DataSource.
2. Install and run the operator:

       make install
       make run   # or: go run ./cmd/main.go --probe-external

   Or in the cluster:

       make docker-build docker-push CONTAINER_TOOL=podman IMG=<registry>/deskube:dev
       make deploy IMG=<registry>/deskube:dev

   The manager probes desktops through their ClusterIP in the cluster,
   and through the client address with `--probe-external` outside it.

3. Create a desktop:

       apiVersion: vdi.desku.be/v1alpha1
       kind: VdiInstance
       metadata:
         name: alice-desk
       spec:
         image:
           dataSource:
             name: deskube-fedora-xfce
         owner: alice

4. Wait for `Ready` and read the address:

       $ oc get vdiinstance
       NAME         PHASE   ADDRESS              SESSION   OWNER
       alice-desk   Ready   203.0.113.10:30016    free      alice

5. In Moonlight, add the host `203.0.113.10:30016`. It shows a PIN.
   Hand the PIN to the operator:

       apiVersion: vdi.desku.be/v1alpha1
       kind: VdiPairing
       metadata:
         name: alice-laptop
       spec:
         instanceRef: alice-desk
         pin: "4693"

6. Moonlight lists `Desktop`. Connect.

The desktop follows the resolution Moonlight asks for. A Sunshine prep
command runs `deskube-resize` in the guest, which adds the mode to the
dummy display with cvt and xrandr, and resets to 1920x1080 afterwards.

`spec.running: false` stops the VM and keeps the disk.
`spec.idleTimeout: 30m` stops it after half an hour without a session;
the operator flips `running` to false, set it back to true to resume.

## Console plugin

`console/` is an OpenShift console dynamic plugin. It adds a
**Virtual Desktops** page under Virtualization: list, create, start,
stop, delete, and pair Moonlight through a VdiPairing. Owners see
their namespace; admins see all.

    make console-build console-push CONTAINER_TOOL=podman CONSOLE_IMG=<registry>/console-plugin:dev
    make console-deploy CONSOLE_IMG=<registry>/console-plugin:dev

The deploy target registers the ConsolePlugin and enables it in the
console operator. The console pods restart once, then the item appears.

## Status

`status.phase` is Provisioning, Starting, Ready, Stopping, Stopped, or Failed.
Conditions: VMReady, NetworkReady, StreamingReady, Ready.
`status.sessionState` is `free` or `busy`, read from Sunshine.
`status.lastActive` is the last time a session was seen.

## Exposure

- `NodePort` (default): one 27 port window per desktop on every node IP.
  86 desktops fit one cluster. The node firewall must allow 30000-32767
  TCP and UDP.
- `LoadBalancer`: one IP per desktop, Moonlight's default port 47989.

## Development

    make test        # envtest with stand-in KubeVirt CRDs
    make lint
    hack/stream-proof/e2e.sh   # headless Moonlight against one VM
    hack/e2e-operator.sh       # VdiInstance + VdiPairing on a cluster

Design notes: `docs/design.md`.

## Known gaps

- Software encoding only was tested. VAAPI and NVENC need a GPU.
- A VdiPairing with the right PIN approves whatever client is waiting.

## License

Apache 2.0, see LICENSE.
