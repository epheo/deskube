import { K8sGroupVersionKind } from '@openshift-console/dynamic-plugin-sdk';

export const VdiInstanceGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiInstance' };
export const VdiPairingGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiPairing' };
export const DEFAULT_IMAGE = 'registry.desku.be/deskube/desktop:dev';

export const ALL_NAMESPACES = '#ALL_NS#';
export const BASE_PATH = '/desktops';
