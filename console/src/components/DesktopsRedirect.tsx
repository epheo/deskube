import { useEffect } from 'react';
import type { FC } from 'react';
import { useActiveNamespace } from '@openshift-console/dynamic-plugin-sdk';
import { pathForNamespace } from '../hooks/useNamespaceSync';

const DesktopsRedirect: FC = () => {
  const [activeNamespace] = useActiveNamespace();

  useEffect(() => {
    window.history.replaceState(null, '', pathForNamespace(activeNamespace));
    window.dispatchEvent(new PopStateEvent('popstate'));
  }, [activeNamespace]);

  return null;
};

export default DesktopsRedirect;
