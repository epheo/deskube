// config/samples/vdi_v1alpha1_vdiclass.yaml, copied: the image builds from console/ alone.
export const vdiClassTemplate = `apiVersion: vdi.desku.be/v1alpha1
kind: VdiClass
metadata:
  name: desktop
spec:
  image: ghcr.io/epheo/deskube-desktop:main
  idleTimeout: 1h
  resources:
    requests:
      cpu: "2"
      memory: 4Gi
    limits:
      memory: 4Gi
  home:
    size: 20Gi
  # Desktops of this class only on labeled nodes, under a runtime of
  # their own. A pod no node matches stays Pending.
  # placement:
  #   nodeSelector:
  #     desku.be/desktops: "true"
  #   runtimeClassName: nvidia
`;
