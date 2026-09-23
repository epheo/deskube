apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: __NAME__
  labels:
    app: __NAME__
spec:
  runStrategy: Always
  instancetype:
    kind: VirtualMachineClusterInstancetype
    name: u1.xlarge
  preference:
    kind: VirtualMachineClusterPreference
    name: fedora
  dataVolumeTemplates:
    - metadata:
        name: __NAME__-root
        annotations:
          cdi.kubevirt.io/cloneType: snapshot
      spec:
        sourceRef:
          kind: DataSource
          name: deskube-fedora-xfce
        storage:
          resources:
            requests:
              storage: 30Gi
  template:
    metadata:
      labels:
        app: __NAME__
    spec:
      domain:
        devices:
          disks:
            - name: rootdisk
              disk:
                bus: virtio
            - name: cloudinitdisk
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
            name: __NAME__-root
        - name: cloudinitdisk
          cloudInitNoCloud:
            secretRef:
              name: __NAME__-cloudinit
---
apiVersion: v1
kind: Service
metadata:
  name: __NAME__
  labels:
    app: __NAME__
spec:
  type: NodePort
  selector:
    app: __NAME__
  ports:
    - {name: https,   protocol: TCP, port: __P_HTTPS__,   nodePort: __P_HTTPS__}
    - {name: http,    protocol: TCP, port: __PORT__,      nodePort: __PORT__}
    - {name: web,     protocol: TCP, port: __P_WEB__,     nodePort: __P_WEB__}
    - {name: rtsp,    protocol: TCP, port: __P_RTSP__,    nodePort: __P_RTSP__}
    - {name: video,   protocol: UDP, port: __P_VIDEO__,   nodePort: __P_VIDEO__}
    - {name: control, protocol: UDP, port: __P_CONTROL__, nodePort: __P_CONTROL__}
    - {name: audio,   protocol: UDP, port: __P_AUDIO__,   nodePort: __P_AUDIO__}
