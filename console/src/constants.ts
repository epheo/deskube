import { K8sGroupVersionKind } from '@openshift-console/dynamic-plugin-sdk';

export const VdiInstanceGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiInstance' };
export const VdiPairingGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiPairing' };
export const DataSourceGVK: K8sGroupVersionKind = { group: 'cdi.kubevirt.io', version: 'v1beta1', kind: 'DataSource' };
export const ClusterInstancetypeGVK: K8sGroupVersionKind = {
  group: 'instancetype.kubevirt.io',
  version: 'v1beta1',
  kind: 'VirtualMachineClusterInstancetype',
};

export const ALL_NAMESPACES = '#ALL_NS#';
export const BASE_PATH = '/desktops';
