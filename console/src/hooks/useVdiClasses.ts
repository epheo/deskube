import { useMemo } from 'react';
import { useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import { VdiClassGVK } from '../constants';
import { VdiClass } from '../types';

const WATCH = { groupVersionKind: VdiClassGVK, isList: true };

// Owners cannot list classes: callers must expect the error.
export const useVdiClasses = (): [VdiClass[], boolean, unknown] => {
  const [list, loaded, error] = useK8sWatchResource<VdiClass[]>(WATCH);
  const sorted = useMemo(
    () => [...(list ?? [])].sort((a, b) => (a.metadata?.name ?? '').localeCompare(b.metadata?.name ?? '')),
    [list],
  );
  return [sorted, loaded, error];
};
