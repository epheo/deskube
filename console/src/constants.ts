import { K8sGroupVersionKind } from '@openshift-console/dynamic-plugin-sdk';

export const VdiInstanceGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiInstance' };
export const DEFAULT_IMAGE = 'ghcr.io/epheo/deskube-desktop:main';

export const ALL_NAMESPACES = '#ALL_NS#';
export const BASE_PATH = '/desktops';
