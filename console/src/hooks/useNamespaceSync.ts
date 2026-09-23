import { useCallback, useEffect, useRef } from 'react';
import { useActiveNamespace } from '@openshift-console/dynamic-plugin-sdk';
import { ALL_NAMESPACES, BASE_PATH } from '../constants';

const ALL_NS_SLUG = 'all-namespaces';
const NS_PREFIX = `${BASE_PATH}/ns/`;
const ALL_NS_PATH = `${BASE_PATH}/${ALL_NS_SLUG}`;

function slugFromPath(): string | undefined {
  const path = window.location.pathname;
  if (path === ALL_NS_PATH || path === `${ALL_NS_PATH}/`) {
    return ALL_NS_SLUG;
  }
  if (path.startsWith(NS_PREFIX)) {
    return path.slice(NS_PREFIX.length).split('/')[0] || undefined;
  }
  return undefined;
}

export function pathForNamespace(ns: string): string {
  return ns === ALL_NAMESPACES ? ALL_NS_PATH : `${NS_PREFIX}${ns}`;
}

function replaceUrl(target: string): void {
  if (window.location.pathname !== target) {
    window.history.replaceState(null, '', target);
    window.dispatchEvent(new PopStateEvent('popstate'));
  }
}

// Keeps the URL and the console namespace picker in agreement, in both directions.
export const useNamespaceSync = (): void => {
  const [activeNamespace, setActiveNamespace] = useActiveNamespace();
  const fromUrl = useRef(false);
  const urlSlug = slugFromPath();

  const syncFromUrl = useCallback(() => {
    const slug = slugFromPath();
    if (!slug) return;
    const target = slug === ALL_NS_SLUG ? ALL_NAMESPACES : slug;
    if (target !== activeNamespace) {
      fromUrl.current = true;
      setActiveNamespace(target);
    }
  }, [activeNamespace, setActiveNamespace]);

  useEffect(() => {
    syncFromUrl();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    window.addEventListener('popstate', syncFromUrl);
    return () => window.removeEventListener('popstate', syncFromUrl);
  }, [syncFromUrl]);

  useEffect(() => {
    if (fromUrl.current) {
      fromUrl.current = false;
      return;
    }
    const expected = activeNamespace === ALL_NAMESPACES ? ALL_NS_SLUG : activeNamespace;
    if (urlSlug !== expected) {
      replaceUrl(pathForNamespace(activeNamespace));
    }
  }, [activeNamespace, urlSlug]);
};
