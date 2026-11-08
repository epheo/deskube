import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';

export interface Condition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
  observedGeneration?: number;
}

export interface Resources {
  requests?: Record<string, string>;
  limits?: Record<string, string>;
}

export interface Home {
  size?: string;
  storageClassName?: string;
}

export type ExposeType = 'NodePort' | 'LoadBalancer';

export interface ClassExpose {
  type?: ExposeType;
  // Unset takes the class's; false overrides a class's true.
  webUI?: boolean;
}

export interface Expose extends ClassExpose {
  basePort?: number;
  host?: string;
}

export interface Stream {
  encoder?: 'software' | 'vaapi';
  keyboardLayout?: string;
}

export interface Toleration {
  key?: string;
  operator?: 'Exists' | 'Equal';
  value?: string;
  effect?: 'NoSchedule' | 'PreferNoSchedule' | 'NoExecute';
  tolerationSeconds?: number;
}

export interface Placement {
  nodeSelector?: Record<string, string>;
  tolerations?: Toleration[];
  affinity?: Record<string, unknown>;
  priorityClassName?: string;
  runtimeClassName?: string;
}

export interface VdiClassSpec {
  image: string;
  imagePullPolicy?: 'Always' | 'IfNotPresent' | 'Never';
  imagePullSecrets?: { name: string }[];
  resources?: Resources;
  home?: Home;
  idleTimeout?: string;
  expose?: ClassExpose;
  stream?: Stream;
  nestedSandboxes?: boolean;
  placement?: Placement;
}

export interface VdiClass extends K8sResourceCommon {
  spec: VdiClassSpec;
}

export type Phase = 'Provisioning' | 'Starting' | 'Ready' | 'Stopping' | 'Stopped' | 'Failed';

export type PairingPhase = 'Pending' | 'Paired' | 'Failed';

export interface PairingStatus {
  pin?: string;
  phase?: PairingPhase;
  message?: string;
  requestedAt?: string;
  pairedAt?: string;
}

// Needs image, className, or both. An unset field takes the class's.
export interface VdiInstanceSpec extends Omit<VdiClassSpec, 'image' | 'expose'> {
  className?: string;
  image?: string;
  owner?: string;
  running?: boolean;
  expose?: Expose;
  pairing?: { pin: string; clientName?: string };
}

export interface VdiInstance extends K8sResourceCommon {
  spec: VdiInstanceSpec;
  status?: {
    phase?: Phase;
    address?: string;
    // The operator resolved the class's webUI: empty means not published.
    webUI?: string;
    basePort?: number;
    adminSecret?: string;
    sessionState?: 'free' | 'busy';
    lastActive?: string;
    pairing?: PairingStatus;
    observedGeneration?: number;
    conditions?: Condition[];
  };
}
