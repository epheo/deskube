import { K8sGroupVersionKind } from '@openshift-console/dynamic-plugin-sdk';

export const VdiInstanceGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiInstance' };
export const VdiClassGVK: K8sGroupVersionKind = { group: 'vdi.desku.be', version: 'v1alpha1', kind: 'VdiClass' };
export const DEFAULT_IMAGE = 'ghcr.io/epheo/deskube-desktop:main';

export const CONDITION_READY = 'Ready';
export const CONDITION_UPDATE_PENDING = 'UpdatePending';
export const CONDITION_SECCOMP_FILTERED = 'SeccompFiltered';
export const REASON_CLASS_NOT_FOUND = 'ClassNotFound';
export const REASON_SECCOMP_PENDING = 'ProfilePending';

export const ALL_NAMESPACES = '#ALL_NS#';
export const BASE_PATH = '/desktops';
