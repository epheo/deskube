# DeskuBe

Virtual desktops as pods, streamed with Sunshine and Moonlight.
No agent on the client beyond Moonlight itself.

DeskuBe is a Kubernetes operator. A desktop is a container: a Wayland
session and Sunshine, on the node's GPU when it has one and on the CPU
when it has none. The operator gives it a home volume, exposes the
seven Sunshine ports, and approves Moonlight pairing on the owner's
behalf.

## How it works

    VdiInstance --> Deployment (one desktop pod, or none when stopped)
                --> PersistentVolumeClaim (the home)
                --> Service (6 ports, 7 with the web UI; NodePort or LoadBalancer)
                --> NetworkPolicy (the pod takes nothing else)
                --> Secret (Sunshine admin password, a read-only file in the pod)
                --> Role + RoleBinding for the owner

The PIN in `spec.pairing` goes to Sunshine's admin API on the pod IP.

Sunshine runs in the pod on a base port the operator allocates.
Every other port is derived from it, and RTSP tells Moonlight the UDP
ports, so the pod and the Service use the same numbers.

Base port 31989 gives: 31984 https, 31989 http, 31990 web UI,
31998 video, 31999 control, 32000 audio, 32010 rtsp.

## Quick start

1. Pick the operator image. For every commit on main, CI publishes
   `ghcr.io/epheo/deskube:sha-<first 7 characters of the commit>` and
   moves `:main` to it.
   Deploy a `sha-` tag or a digest, not `:main`.
   The manager does not pull its image again when it restarts:
   a restart never waits on the registry, and a moving tag goes stale.
   To build your own:

       make docker-build docker-push CONTAINER_TOOL=podman IMG=<registry>/deskube:<tag>

   The desktop images (sway, Sunshine and the input shim on the newest
   Fedora release, plus a Steam variant) are built in their own
   repository; the operator only knows them by name.

2. Install the operator:

       make install
       make deploy

   `make deploy` takes the `sha-` image of the checked out commit.
   For a commit CI did not build, pass `IMG`.

   `make run`, or `go run ./cmd/main.go --probe-external`, runs it
   outside the cluster; it then probes desktops through the client
   address instead of the pod IP, and every desktop publishes its web
   UI port to any source. For development only.
   In the cluster the manager reads its namespace from `POD_NAMESPACE`,
   set by `config/manager`; without it or `--probe-external` it exits.

3. Create a desktop:

       apiVersion: vdi.desku.be/v1alpha1
       kind: VdiInstance
       metadata:
         name: alice-desk
       spec:
         image: ghcr.io/epheo/deskube-desktop:main
         owner: alice
         resources:
           requests: {cpu: "2", memory: 4Gi}
           limits: {memory: 4Gi}

4. Wait for `Ready` and read the address:

       $ oc get vdiinstance
       NAME         PHASE   ADDRESS              SESSION   OWNER
       alice-desk   Ready   203.0.113.10:30016    free      alice

5. In Moonlight, add the host `203.0.113.10:30016`. It shows a PIN.
   As the owner, write it on the desktop:

       oc patch vdiinstance alice-desk --type=merge \
         -p '{"spec":{"pairing":{"pin":"4693","clientName":"laptop"}}}'

   The operator approves the waiting client, reports in
   `status.pairing`, and removes `spec.pairing`.
   A PIN not used within ten minutes fails, as Moonlight's request does.

6. Moonlight lists `Desktop`. Connect.

The desktop takes the resolution and refresh rate Moonlight asks for,
applied when the app launches, and returns to 1920x1080 at 60 Hz when
the session ends.

`spec.running: false` removes the pod and keeps the home.
`spec.idleTimeout: 30m` does that after half an hour without a session;
the operator flips `running` to false, set it back to true to resume.

## Owners

`spec.owner` names a Kubernetes user.
The operator gives that user a Role in the namespace: read every
desktop there, update and patch their own, and read its admin Secret
while `spec.expose.webUI` is on.

RBAC cannot tell one field from another.
A ValidatingAdmissionPolicy from `config/policy`, installed by
`make deploy`, narrows the writes.
Whoever may not create desktops in the namespace may change only
`spec.running` and `spec.pairing`.
Every other field, and the finalizers, stay with admins: whoever may
create desktops there, the manager included.
The policy needs Kubernetes 1.30 or later, OpenShift 4.17 or later.

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
Moonlight by writing the PIN on the desktop. Owners see their
namespace; admins see all.

    make console-deploy

CI publishes `ghcr.io/epheo/deskube-console-plugin:main`. To build
your own, `make console-build console-push CONSOLE_IMG=<image>`, then
pass the same `CONSOLE_IMG` to `make console-deploy`.

The deploy target registers the ConsolePlugin and enables it in the
console operator. The console pods restart once, then the item appears.

## Status

`status.phase` is Provisioning, Starting, Ready, Stopping, Stopped, or Failed.
Conditions: PodReady, NetworkReady, StreamingReady, Ready. PodReady
carries the kubelet's reason when a desktop does not start.
`status.sessionState` is `free` or `busy`, read from Sunshine.
`status.lastActive` is the last time a session was seen.
`status.pairing` is the last pairing request: its PIN, Pending, Paired
or Failed, and why.

## Exposure

- `NodePort` (default): one 27 port window per desktop on every node IP.
  86 desktops fit one cluster. The node firewall must allow 30000-32767
  TCP and UDP.
- `LoadBalancer`: one IP per desktop, Moonlight's default port 47989.
  Its Service takes no node ports: random ones would split the windows
  NodePort desktops need. Cost: a load balancer that forwards to node
  ports, as some cloud ones do, has nowhere to send traffic. MetalLB
  does not need them.

The operator skips any window where another desktop or any Service in
the cluster holds one of the seven ports.
`spec.expose.basePort` pins a window, on the 32 port grid or not.
A pinned window that overlaps one already held is refused:
NetworkReady is False, reason `PortConflict`, and the message names
the holder.

Each desktop has a NetworkPolicy:

- Moonlight's six ports take any source: NodePort and load balancer
  traffic arrives SNATed from node addresses. The kubelet's readiness
  probe is on one of them, base.
- The web UI port, base+1, takes only the manager's pods, by namespace
  and pod label, unless the web UI is published.
- Nothing else reaches the pod. Egress stays open: a desktop browses
  the internet.

Sunshine's admin web UI, on base+1, stays off the Service by default.
The operator reaches Sunshine on the pod IP and needs no port for it.
`spec.expose.webUI: true` publishes the web UI and lets it answer any
address; without it, private addresses only.
Changing `webUI` restarts the desktop.
Behind a LoadBalancer the web UI also needs `spec.expose.host`, set to
the address browsers use: Sunshine refuses changes from an origin it
was not started with, and the operator never restarts a desktop for an
address that arrives later.

## Development

    make test                  # envtest
    make lint
    hack/e2e-operator.sh       # a desktop, pairing, a stream, on a cluster

`hack/e2e-operator.sh` streams with the `moonlight-cli` image from
desku.be-images. Design notes: `docs/design.md`; by hand results with
podman, no cluster: the desku.be-images README.

## Known gaps

- The operator does not hand a GPU to the pod yet; cluster desktops
  render and encode on the CPU. With podman the GPU path works.
- The gaming images also need a GPU, see the first gap.
- The home volume and the admin password file are group owned through
  OpenShift's fsGroup; plain Kubernetes needs one set.
- The admin password reaches the pod as a file named by
  `DESKUBE_ADMIN_PASSWORD_FILE`, no longer as an environment variable.
  An image whose desk-session does not read it fails to pair, and its
  web UI takes the first password anyone sets.
- A private registry needs a pull secret in every namespace that runs
  desktops, linked to its default service account or named in
  `spec.imagePullSecrets`.
- A PIN approves the first waiting client Sunshine accepts it for.
  Owners who share a namespace can read each other's pending PIN; one
  could race the owner's Moonlight with a client that sends it.

## License

Apache 2.0, see LICENSE.
