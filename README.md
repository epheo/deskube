# DeskuBe

Virtual desktops as pods, streamed with Sunshine and Moonlight.
No agent on the client beyond Moonlight itself.

DeskuBe is a Kubernetes operator. A desktop is a container: a Wayland
session and Sunshine, on the node's GPU when it has one and on the CPU
when it has none. The operator gives it a home volume, exposes the
seven Sunshine ports, and approves Moonlight pairing on the owner's
behalf.

## How it works

    VdiInstance ──> Deployment (one desktop pod, or none when stopped)
                ──> PersistentVolumeClaim (the home)
                ──> Service (7 ports, NodePort or LoadBalancer)
                ──> Secret (Sunshine admin password)
                ──> Role + RoleBinding for the owner

    VdiPairing  ──> POST /api/pin on the desktop's Sunshine web UI

Sunshine runs in the pod on a base port the operator allocates.
Every other port is derived from it, and RTSP tells Moonlight the UDP
ports, so the pod and the Service use the same numbers.

Base port 31989 gives: 31984 https, 31989 http, 31990 web UI,
31998 video, 31999 control, 32000 audio, 32010 rtsp.

## Quick start

1. Build and push the operator image:

       make docker-build docker-push CONTAINER_TOOL=podman IMG=<registry>/deskube:dev

   The desktop images (sway, Sunshine and the input shim on the newest
   Fedora release, plus a Steam variant) are built in the
   [desku.be-images](../desku.be-images) repository; the operator only
   knows them by name.

2. Install the operator:

       make install
       make deploy IMG=<registry>/deskube:dev

   `make run`, or `go run ./cmd/main.go --probe-external`, runs it
   outside the cluster; it then probes desktops through the client
   address instead of the ClusterIP.

3. Create a desktop:

       apiVersion: vdi.desku.be/v1alpha1
       kind: VdiInstance
       metadata:
         name: alice-desk
       spec:
         image: registry.desku.be/deskube/desktop:dev
         owner: alice
         resources:
           requests: {cpu: "2", memory: 4Gi}
           limits: {memory: 4Gi}

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

The desktop takes the resolution and refresh rate Moonlight asks for,
applied when the app launches, and returns to 1920x1080 at 60 Hz when
the session ends.

`spec.running: false` removes the pod and keeps the home.
`spec.idleTimeout: 30m` does that after half an hour without a session;
the operator flips `running` to false, set it back to true to resume.

## Limits per desktop

- CPU and memory: `spec.resources`, enforced by the kubelet. Request
  CPU rather than cap it: a throttled desktop stutters.
- GPU memory and GPU time have no limit in Kubernetes. A frame cap and
  a resolution cap per desktop are what bound them.

## Desktops that run sandboxes

Steam, Flatpak, and container engines build sandboxes of their own,
which an ordinary pod may not. `spec.nestedSandboxes: true` allows it
with as little as it takes:

- the pod runs in its own user namespace, as UID 1000 inside it;
- `/proc` is unmasked, so a sandbox can mount its own;
- on OpenShift a service account made for this one desktop is bound to
  the stock `nested-container` SCC.

It stays unprivileged: no capability, no privilege escalation, no API
token. The runtime's default seccomp filter refuses nested namespaces,
so such a desktop needs another one. The manager picks the tightest
available and reports it in the `SeccompFiltered` condition:

1. `--sandbox-seccomp-profile=<path under the kubelet's seccomp
   directory>`, a profile an admin already put on the nodes.
2. Otherwise its built in profile, installed on the nodes by the
   [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
   when that operator is present. The desktop waits for it rather than
   start unfiltered. DeskuBe itself never touches a node.
3. Otherwise no filter, and the condition is False and says so.

The built in profile is podman's default on Fedora, resolved for a
container without capabilities by `hack/seccomp/flatten.py`.

## Console plugin

`console/` is an OpenShift console dynamic plugin. It adds a
**Virtual Desktops** page: list, create, start, stop, delete, and pair
Moonlight through a VdiPairing. Owners see their namespace; admins see
all.

    make console-build console-push CONTAINER_TOOL=podman CONSOLE_IMG=<registry>/console-plugin:dev
    make console-deploy CONSOLE_IMG=<registry>/console-plugin:dev

The deploy target registers the ConsolePlugin and enables it in the
console operator. The console pods restart once, then the item appears.

## Status

`status.phase` is Provisioning, Starting, Ready, Stopping, Stopped, or Failed.
Conditions: PodReady, NetworkReady, StreamingReady, Ready. PodReady
carries the kubelet's reason when a desktop does not start.
`status.sessionState` is `free` or `busy`, read from Sunshine.
`status.lastActive` is the last time a session was seen.

## Exposure

- `NodePort` (default): one 27 port window per desktop on every node IP.
  86 desktops fit one cluster. The node firewall must allow 30000-32767
  TCP and UDP.
- `LoadBalancer`: one IP per desktop, Moonlight's default port 47989.

## Development

    make test                  # envtest
    make lint
    hack/e2e-operator.sh       # VdiInstance + VdiPairing on a cluster

`hack/e2e-operator.sh` streams with the `moonlight-cli` image from
desku.be-images. Design notes: `docs/design.md`; by hand results with
podman, no cluster: the desku.be-images README.

## Known gaps

- The operator does not hand a GPU to the pod yet; cluster desktops
  render and encode on the CPU. With podman the GPU path works.
- The gaming images also need a GPU, see the first gap.
- The pod's volume is group owned through OpenShift's fsGroup; plain
  Kubernetes needs one set.
- A private registry needs a pull secret in every namespace that runs
  desktops, linked to its default service account or named in
  `spec.imagePullSecrets`.
- A VdiPairing with the right PIN approves whatever client is waiting.

## License

Apache 2.0, see LICENSE.
