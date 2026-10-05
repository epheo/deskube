import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';

export interface Condition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
}

export interface VdiInstance extends K8sResourceCommon {
  spec: {
    image: string;
    resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
    home?: { size?: string; storageClassName?: string };
    owner?: string;
    running?: boolean;
    idleTimeout?: string;
    expose?: { type?: 'NodePort' | 'LoadBalancer'; basePort?: number; host?: string };
  };
  status?: {
    phase?: 'Provisioning' | 'Starting' | 'Ready' | 'Stopping' | 'Stopped' | 'Failed';
    address?: string;
    basePort?: number;
    adminSecret?: string;
    sessionState?: string;
    lastActive?: string;
    conditions?: Condition[];
  };
}

export interface VdiPairing extends K8sResourceCommon {
  spec: { instanceRef: string; pin: string; clientName?: string };
  status?: { phase?: 'Pending' | 'Paired' | 'Failed'; message?: string; pairedAt?: string };
}
