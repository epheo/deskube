import { K8sResourceCommon } from '@openshift-console/dynamic-plugin-sdk';

export interface Condition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
}

export type PairingPhase = 'Pending' | 'Paired' | 'Failed';

export interface VdiInstance extends K8sResourceCommon {
  spec: {
    image: string;
    resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
    home?: { size?: string; storageClassName?: string };
    owner?: string;
    running?: boolean;
    idleTimeout?: string;
    expose?: { type?: 'NodePort' | 'LoadBalancer'; basePort?: number; host?: string };
    pairing?: { pin: string; clientName?: string };
  };
  status?: {
    phase?: 'Provisioning' | 'Starting' | 'Ready' | 'Stopping' | 'Stopped' | 'Failed';
    address?: string;
    basePort?: number;
    adminSecret?: string;
    sessionState?: string;
    lastActive?: string;
    pairing?: { pin?: string; phase?: PairingPhase; message?: string; requestedAt?: string; pairedAt?: string };
    conditions?: Condition[];
  };
}
