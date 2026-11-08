import { useEffect } from 'react';
import { SetFeatureFlag, useAccessReview } from '@openshift-console/dynamic-plugin-sdk';
import { VdiClassGVK } from './constants';

// console-extensions.json requires it by name.
export const FLAG_CLASS_VIEWER = 'DESKUBE_CLASS_VIEWER';

export const useClassViewerFlag = (setFeatureFlag: SetFeatureFlag): void => {
  const [allowed, loading] = useAccessReview({ group: VdiClassGVK.group, resource: 'vdiclasses', verb: 'list' });
  useEffect(() => {
    if (!loading) setFeatureFlag(FLAG_CLASS_VIEWER, allowed);
  }, [allowed, loading, setFeatureFlag]);
};
