# DeskuBe design, restart of 2026-09-23

## What was proven

- A KubeVirt VM with Xorg dummy, XFCE, and Sunshine streams to Moonlight with no GPU.
- Sunshine on base port 31989 keeps every derived port inside the NodePort range.
- Pairing needs no human in the guest: `GET /api/pin` lists pending requests, `POST /api/pin` approves one.
- A thin LVM snapshot clones a golden image in seconds; a cloud-init install takes five minutes.
- Audio works once the Sunshine null sink is declared in PipeWire config instead of loaded at stream start.
- The dummy display takes new modes at runtime, so a prep command can match the client resolution.

See `hack/stream-proof` and `hack/golden`.

## Why the old design could not work

- Routes carry HTTP only. Moonlight video, audio, and control are UDP.
- RTSP advertises the guest's UDP ports, so a Service cannot remap them. The guest port must equal the external port.
- Pairing was never handled, so nobody could ever connect.

## Resources

### VdiInstance

One desktop for one owner.

    spec:
      image:        {dataSource: {name, namespace}}   golden image
      instanceType: u1.xlarge                          KubeVirt cluster instancetype
      preference:   fedora
      owner:        alice                              Kubernetes user, gets RBAC on this object
      running:      true                               power state, idle shutdown flips it
      idleTimeout:  30m                                optional
      expose:       {type: NodePort | LoadBalancer, basePort: optional}
      stream:       {encoder: software, capture: x11}
      sshAuthorizedKeys: []

    status:
      phase:        Provisioning | Starting | Ready | Stopped | Failed
      address:      203.0.113.10:31989                  what to type in Moonlight
      basePort:     31989
      adminSecret:  <name>-sunshine-admin
      sessionState: free | busy                        from /serverinfo
      lastActive:   time                               busy seen, or became Ready
      conditions:   VMReady, StreamingReady, NetworkReady, Ready

The controller owns, per instance:

- a Secret with the Sunshine admin password,
- a Secret with the rendered cloud-init,
- a VirtualMachine cloned from the golden image,
- a Service with the seven ports,
- a Role and RoleBinding for the owner.

### VdiPairing

The PIN Moonlight shows, handed to the operator instead of the guest.

    spec:
      instanceRef: alice-desk
      pin: "4693"
      clientName: laptop
    status:
      phase: Pending | Paired | Failed | Expired

The controller reads the admin Secret, lists pending pairings on the
instance's web port through the ClusterIP, and approves the one that
matches. A portal can wrap this later; kubectl works today.

## Port allocation

NodePort exposure needs one window of 27 ports per desktop:
base-5, base, base+1, base+9, base+10, base+11, base+21.
Bases start at 30016 and step by 32, so 86 desktops fit one cluster.
LoadBalancer exposure uses base 47989 and one IP per desktop.

## Order of work

1. VdiInstance rewrite: VM, Service, Secrets, port allocation, readiness from `/serverinfo`.
2. VdiPairing.
3. Owner RBAC.
4. Idle shutdown from `sessionState`: done, `spec.idleTimeout`.
5. Pools and templates.
