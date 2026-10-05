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
  unprivileged container. `hack/pod/README.md` has the numbers.
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
      image:       registry/desktop:tag
      resources:   {requests, limits}            CPU and memory of the pod
      home:        {size: 20Gi, storageClassName}
      owner:       alice                         Kubernetes user, gets RBAC on this object
      running:     true                          power state, idle shutdown flips it
      idleTimeout: 30m                           optional
      expose:      {type: NodePort | LoadBalancer, basePort, host}
      stream:      {encoder: software | vaapi, keyboardLayout}

    status:
      phase:        Provisioning | Starting | Ready | Stopping | Stopped | Failed
      address:      203.0.113.10:31989            what to type in Moonlight
      basePort:     31989
      adminSecret:  <name>-sunshine-admin
      sessionState: free | busy                  from /serverinfo
      lastActive:   time                         busy seen, or became Ready
      conditions:   PodReady, StreamingReady, NetworkReady, Ready

The controller owns, per instance:

- a Secret with the Sunshine admin password,
- a PersistentVolumeClaim for the home, kept while the desktop is stopped,
- a Deployment with one replica, zero when stopped, strategy Recreate
  because the home and the ports belong to one pod at a time,
- a Service with the seven ports,
- a Role and RoleBinding for the owner.

The pod gets no service account token, drops every capability, and
runs under the default seccomp profile. Its readiness probe is
Sunshine's `/serverinfo`.

### VdiPairing

The PIN Moonlight shows, handed to the operator instead of the desktop.

    spec:
      instanceRef: alice-desk
      pin: "4693"
      clientName: laptop
    status:
      phase: Pending | Paired | Failed

The controller reads the admin Secret, lists pending pairings on the
instance's web port through the ClusterIP, and approves the one that
matches.

## Port allocation

NodePort exposure needs one window of 27 ports per desktop:
base-5, base, base+1, base+9, base+10, base+11, base+21.
Bases start at 30016 and step by 32, so 86 desktops fit one cluster.
LoadBalancer exposure uses base 47989 and one IP per desktop.

## Order of work

1. VdiInstance as a pod, pairing, owner RBAC, idle shutdown: written
   and tested with envtest; not run on a cluster yet.
2. GPU in the pod: a device plugin or DRA handing out the render node,
   with a fixed number of desktops per GPU.
3. Gaming profile: unmasked /proc with a pod user namespace, the GPU's
   primary node visible, and a node agent for gamepads so that a desktop
   never holds the real uinput.
4. Limits the kubelet does not know: GPU memory through the dmem cgroup
   controller, a frame cap per desktop.
5. Pools and templates.
