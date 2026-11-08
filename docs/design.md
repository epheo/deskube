# DeskuBe design

Restarted 2026-09-23 on KubeVirt VMs, moved to pods only on 2026-10-05.

## Why pods

- A pod shares the node's GPU through its render node. No vGPU exists
  for most integrated GPUs; a VM would get none, or the whole card.
- Hardware encoding costs 2 to 3 ms per frame. The VM encoded on the
  CPU in about 15 ms.
- A desktop starts in seconds from an image pull, with no disk clone.
- What a VM would still do better: Windows guests, and a hard wall
  between tenants. Neither is a goal now.

## What was proven

- sway on the wlroots headless backend, Sunshine capturing it, in an
  unprivileged container. The deskube-images README has the numbers.
- With a GPU: wlr capture and VAAPI, 120 fps at 2560x1440 delivered.
- Without one: the compositor draws on the CPU, capture goes through
  the screen cast portal, libx264 encodes, 60 fps at 1920x1080.
- Input without kernel devices: a preload library answers Sunshine's
  uinput calls and replays them into the compositor.
- Pairing needs no human at the desktop: `GET /api/pin` lists pending
  requests, `POST /api/pin` approves one.
- Sunshine on a base port inside the NodePort range keeps every derived
  port in it. RTSP advertises the UDP ports, so a Service cannot remap
  them: the pod port must equal the external port.

## Rules for images

- The newest Fedora release, not a pinned one.
- Wayland sessions only. No Xorg server, no X11 capture.
- Xwayland only in the gaming image: Steam and Proton games are X11.
- Runs as any UID in group 0, as OpenShift assigns one.

## Resources

### VdiInstance

One desktop for one owner.

    spec:
      className:   desktop                       a VdiClass; image, className or both
      image:       registry/desktop:tag
      resources:   {requests, limits}            CPU and memory of the pod
      home:        {size: 20Gi, storageClassName}
      owner:       alice                         Kubernetes user, gets RBAC on this object
      running:     true                          power state, idle shutdown flips it
      idleTimeout: 30m                           optional
      expose:      {type: NodePort | LoadBalancer, basePort, host, webUI}
      stream:      {encoder: software | vaapi, keyboardLayout}
      nestedSandboxes: false                    let Steam, Flatpak, podman sandbox themselves
      placement:   {nodeSelector, tolerations, affinity, priorityClassName, runtimeClassName}
      pairing:     {pin: "4693", clientName}     the PIN Moonlight shows; removed once handled

    status:
      phase:        Provisioning | Starting | Ready | Stopping | Stopped | Failed
      address:      203.0.113.10:31989            what to type in Moonlight
      webUI:        https://203.0.113.10:31990    only while expose.webUI publishes it
      basePort:     31989
      adminSecret:  <name>-sunshine-admin
      sessionState: free | busy                  from /serverinfo
      lastActive:   time                         busy seen, or became Ready
      pairing:      {pin, phase: Pending | Paired | Failed, message, requestedAt, pairedAt}
      conditions:   PodReady, StreamingReady, NetworkReady, Ready, SeccompFiltered, UpdatePending

The controller owns, per instance:

- a Secret with the Sunshine admin password,
- a PersistentVolumeClaim for the home, kept while the desktop is stopped,
- a Deployment with one replica, zero when stopped, strategy Recreate
  because the home and the ports belong to one pod at a time,
- a Service with six ports, the web UI port only with `expose.webUI`,
- a NetworkPolicy on the pod, ingress only: see Network below,
- a Role and RoleBinding for the owner: read the namespace's desktops,
  update and patch this one, and get the admin Secret while
  `expose.webUI` is on.

The pod gets no service account token, drops every capability, and
runs under the default seccomp profile. Its readiness probe is
Sunshine's `/serverinfo`.

The admin password reaches the pod as a read-only file named by
`DESKUBE_ADMIN_PASSWORD_FILE`.
As an environment variable it sat in PID 1's environment and in the
runtime's container config.
The file is 0440, read through the group: OpenShift's arbitrary UID is
in group 0 and the fsGroup, a sandbox desktop's UID 1000 in fsGroup 1000.
The desktop's own programs can still read it, as they could PID 1's
environment.
An image whose desk-session does not read the file gets no password:
its pairing fails and its web UI takes the first password anyone sets.

### Pairing

The owner writes the PIN Moonlight shows into `spec.pairing` of their
own desktop.
RBAC by resourceName already keeps it there; the VdiPairing it replaces
let any owner pair with any desktop in a shared namespace.

Reading is another matter: list ignores resourceNames, so owners in one
namespace see each other's desktops, a pending PIN included.
One can pair a client of their own with it before the owner's does.
RBAC cannot hide one field, and DeskuBe does not try.
Give owners who do not trust each other a namespace each.

While the desktop is Ready, the controller lists pending requests with
`GET /api/pin` on the pod IP and submits the PIN to each with
`POST /api/pin`.
The first one Sunshine accepts is paired.
Ten minutes after `status.pairing.requestedAt` the request fails, as
Moonlight's own does.

The outcome goes to `status.pairing` by merge patch, after the rest of
the status: a conflict there cannot lose an approval Sunshine accepted.
A JSON patch then removes `spec.pairing` after testing the PIN, so a
newer PIN written meanwhile stays.
`status.pairing.pin` tells a new request from a final one whose removal
failed, and tells the console which PIN an outcome answers.

### What an owner may change

RBAC grants update by object, not by field.
Without more, an owner could set `image` and `nestedSandboxes` and run
anything under the nested-container SCC.
A ValidatingAdmissionPolicy in `config/policy` applies to updates of
VdiInstances by whoever may not create one in the namespace, asked of
CEL's `authorizer`.
They may change `spec.running` and `spec.pairing`; every other spec
field and `metadata.finalizers` must stay as they were.
`className` too: a class brings an image and sandboxes of its own.
`placement` too: it can preempt pods and reach tainted nodes.
Admins and the manager can create, so the policy skips them.
The policy compares each spec field by name.
A unit test fails when a field of VdiInstanceSpec is neither compared
nor listed as owner mutable.
ValidatingAdmissionPolicy v1 needs Kubernetes 1.30.

### VdiClass

A kind of desktop an admin offers, cluster scoped, like a StorageClass.

    spec:
      image:           registry/desktop:tag     required
      imagePullPolicy: IfNotPresent
      imagePullSecrets: [{name}]                Secrets in each desktop's namespace
      resources:       {requests, limits}
      home:            {size, storageClassName}
      idleTimeout:     1h
      expose:          {type, webUI}            basePort and host belong to one desktop
      stream:          {encoder, keyboardLayout}
      nestedSandboxes: false
      placement:       {nodeSelector, tolerations, affinity, priorityClassName, runtimeClassName}

No status: nothing about a class needs reporting yet.

One function, `resolve`, works out a desktop's settings once per pass:
each field the instance sets, else the class's, else the default.
Every reader takes its result; reading `vdi.Spec` would miss the class.
basePort, host, owner, running and pairing exist on the instance only.

- expose, stream, home and placement merge field by field.
- resources come whole: a limit from one side under a request from the
  other is a pod the API server refuses.
- A placement field comes whole too: two nodeSelectors merged by key
  are a choice nobody made.
- webUI and nestedSandboxes are pointers, so false on an instance
  overrides a class's true.
- expose.type and home.size have no CRD default: a default reads as
  set on the instance and would beat the class. The operator applies
  NodePort and 20Gi.
- Port allocation resolves the other desktops too: a desktop that is a
  LoadBalancer through its class holds no NodePort window.

A field index on `spec.className` maps a class event to the instances
naming it, and to no other.
A change that alters the pod template restarts those desktops, each
when it has no session: see Changes and sessions.
home applies only to a claim not made yet.

A missing class: phase Provisioning, Ready False, reason
ClassNotFound, and nothing else touched. No requeue: the class watch
wakes the instance when the class appears.

So a class in use cannot go away. A second controller puts the
finalizer `vdi.desku.be/class-in-use` on a class while any instance
names it, and removes it once none does.
It watches instances and maps an update's old object too, so a class
an instance leaves hears of it.
Before removing the finalizer it lists instances past the cache: one
the cache has not seen yet would lose its class.
A class gone anyway, made before the finalizer or with it removed by
hand: its desktops keep their pods and report ClassNotFound, and
`running: false` still scales the Deployment to zero.
Cost: patch on vdiclasses. RBAC cannot narrow it to the finalizer, so
the manager could change any class. A test proves patch suffices and
update is not granted.

Placement goes to the pod as is. Unset is nil, as before placement
existed, so a desktop without one keeps its pod.
runtimeClassName is how a GPU runtime gets picked.

Cost: the manager gets, lists and watches every VdiClass. A class holds
no secret, only the names of pull secrets.
`config/rbac` has viewer and editor ClusterRoles. Owners get nothing.

## Port allocation

NodePort exposure needs one window of 27 ports per desktop:
base-5, base, base+1, base+9, base+10, base+11, base+21.
Bases start at 30016 and step by 32, so 86 desktops fit one cluster.
LoadBalancer exposure uses base 47989 and one IP per desktop.
Its Service sets `allocateLoadBalancerNodePorts: false`. By default
each of its ports takes a random node port, and those split the
windows. Load balancers that forward to node ports, some cloud ones,
need them; MetalLB does not. A Service an older manager made gives its
node ports back on the next pass.

A window is free only when none of its seven ports is held by another
desktop, allocated or pinned, or is any Service's NodePort.
The manager lists Services cluster wide past its cache, at allocation
only: its cache holds DeskuBe's objects alone.
A pinned base need not sit on the grid, so it is checked port by port.
It yields only to windows already allocated: two equal pins would
otherwise refuse each other forever. The first one keeps the window.
A refused pin reports NetworkReady False, reason PortConflict, naming
the holder.

The Service is created before the pod, so the API server's own check
on NodePorts comes first. When it refuses one, a desktop whose Service
does not exist yet drops its window: an unpinned one takes the next
free window, a pinned one reports PortConflict.
A Service that exists keeps its window even then: a stale cache makes
the manager create it again, and its own ports read as taken.

## The web UI

Sunshine's admin web UI can change anything in the desktop, so it is
not published by default.
The manager reaches /serverinfo and the admin API on the pod IP, and
needs no port on the Service for them.
`expose.webUI` publishes the web port and passes
DESKUBE_WEB_UI_ALLOWED=wan; otherwise the value is lan.
desk-session writes it to Sunshine's origin_web_ui_allowed. Pod IPs
count as lan for Sunshine, so the manager gets in either way.
The window keeps the web port while it is unpublished, so turning
webUI on never collides.
A manager run with --probe-external cannot route to pods. It uses the
client address, and every Service keeps the web port. Dev only.

status.webUI is the URL a browser opens: the address's host, base+1.
It follows expose.webUI as resolved, not the open port: a port open
only for a --probe-external manager is not one to send users to.
The console offers the web UI from status.webUI alone: owners cannot
read classes, so it could not resolve webUI for them.

Changing env restarts the desktop, so changing webUI does too.
The operator never changes env for an address that arrives later, like
a LoadBalancer IP. Sunshine only accepts web UI changes from origins it
was started with, so the web UI behind a LoadBalancer needs
`expose.host`.

## Network

Without a policy any pod in the cluster reached any desktop on its pod
IP, the admin API on base+1 included.
Each desktop has a NetworkPolicy on its pod, ingress only:

- The six Moonlight ports from any source. NodePort and load balancer
  traffic arrives SNATed from node addresses; no ipBlock names clients.
- The readiness probe is HTTP on base, one of the six. Kubernetes
  admits the pod's own node regardless.
- The web port from pods labeled `control-plane: controller-manager`
  in the manager's namespace.
- The web port from any source with `expose.webUI`, or for a manager
  run with --probe-external, which comes in through a node port.

kustomize prefixes the manager's namespace, so the manager reads it
from `POD_NAMESPACE`, set through the downward API.
A unit test holds `config/manager` to that variable and the pod label.
Egress stays open: a desktop browses the internet.
The policy is made before the Deployment: a new desktop never runs open.
Cost: the manager may write any NetworkPolicy in the cluster, as it
already may Roles. It has no delete; garbage collection removes them.
envtest enforces no policy. Its tests prove the shape, a cluster the
effect.

## Changes and sessions

A new pod template restarts the desktop: Recreate ends the pod and the
session in it. VDI users expect changes at their next logoff, so a
desktop in a session keeps its pod.

- ensureDeployment copies the current template, fills the wanted one
  in place, and compares the two with semantic equality, as
  CreateOrUpdate does: a field the API server defaulted is no change.
- While running, with a ready pod and maybe a session, it puts the
  copy back. Replicas still apply: stopping always works.
- UpdatePending is True, its message the changed fields, the desktop
  container's by their own name.
- Busy as of the previous pass's probe holds the change without asking
  again. The pass that sees the session end requeues in a second.
- Anything else, free included, may be 30 seconds stale. Before
  changing the pod the pass asks Sunshine again, through the same
  probe function and host.
- Only a change asks: an unchanged desktop makes no extra call.
- The status takes that answer instead of asking again: a pass asks
  Sunshine once at most, and the status shows what the change was
  decided on.
- Busy now holds the change, and status.sessionState says busy.
- No answer holds it too, reason SessionUnknown: an unreachable
  Sunshine is no proof nobody watches. The pass requeues in 5 seconds.
- Without a ready pod there is no session: the change goes in at once.
  So it does on a stopped desktop, or one about to stop.
- The Service, NetworkPolicy and owner Role are not held. A webUI
  change thus opens or closes the port at once while Sunshine keeps its
  old origin rule until the restart.
- Cost: a Sunshine the manager cannot reach while its pod stays ready
  holds every pod change, an operator upgrade's too, until the desktop
  stops.

## Order of work

1. Done: VdiInstance as a pod, pairing, owner RBAC, idle shutdown.
   Proven on OpenShift 4.22 without a GPU on 2026-10-05: 59 fps at
   1920x1080 delivered through a NodePort, three sessions in a row.
2. Done: classes and placement, VdiClass and `spec.placement`.
   Proven in envtest only, on no cluster yet.
3. GPU in the pod: a device plugin or DRA handing out the render node,
   with a fixed number of desktops per GPU. A class's placement and
   runtimeClassName pick the GPU nodes and runtime.
   On NVIDIA: [gpu-nvidia.md](gpu-nvidia.md), the design and the
   experiments that prove it, none run yet.
4. Sandboxes inside a desktop, which Steam needs: done as
   `spec.nestedSandboxes`, proven on OpenShift 4.22 on 2026-10-05. The
   pod gets `hostUsers: false`, UID 1000 inside its namespace,
   `procMount: Unmasked`, no privilege escalation, no capability, and a
   service account of its own bound to the stock `nested-container`
   SCC. That SCC sets SELinux type container_engine_t; under
   container_t the /proc mount is denied. The runtime's default
   seccomp filter refuses nested namespaces, so the manager has the
   Security Profiles Operator install its own: podman's default,
   resolved for no capabilities. DeskuBe never writes on a node.
   Without that operator the desktop runs unfiltered and its status
   says so. Gamepads are set aside.
   Steam itself ran in such a pod, filter on, no GPU: its sandbox
   started and the client reached its sign in window.
5. Limits the kubelet does not know: GPU memory through the dmem cgroup
   controller, a frame cap per desktop.
6. Pools of desktops kept ready. Templates are classes, item 2.
