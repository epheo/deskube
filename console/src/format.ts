import { Placement, Resources } from './types';

const BINARY_UNITS: Record<string, string> = { Ki: 'KiB', Mi: 'MiB', Gi: 'GiB', Ti: 'TiB' };

const cpuText = (q: string): string => (q.endsWith('m') ? `${Number(q.slice(0, -1)) / 1000} CPU` : `${q} CPU`);

const memoryText = (q: string): string => {
  const m = /^([0-9.]+)([KMGT]i)$/.exec(q);
  return m ? `${m[1]} ${BINARY_UNITS[m[2]]}` : q;
};

// A size is what the desktop is guaranteed; limits stand in when only they are set.
export const sizeText = (r?: Resources): string | undefined => {
  const cpu = r?.requests?.cpu ?? r?.limits?.cpu;
  const memory = r?.requests?.memory ?? r?.limits?.memory;
  const parts = [cpu && cpuText(cpu), memory && memoryText(memory)].filter(Boolean);
  return parts.length ? parts.join(', ') : undefined;
};

const SECONDS: Record<string, number> = { ms: 1e-3, s: 1, m: 60, h: 3600 };

const plural = (n: number, unit: string): string => `${n} ${unit}${n === 1 ? '' : 's'}`;

// A Go duration: 1h as written, 1h0m0s once Go code wrote it back.
export const durationText = (d: string): string => {
  let total = 0;
  for (const [, n, unit] of d.matchAll(/([0-9.]+)(ms|s|m|h)/g)) {
    total += Number(n) * SECONDS[unit];
  }
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = Math.round(total % 60);
  const parts = [
    hours && plural(hours, 'hour'),
    minutes && plural(minutes, 'minute'),
    seconds && plural(seconds, 'second'),
  ].filter(Boolean);
  return parts.length ? parts.join(' ') : d;
};

export const placementText = (p?: Placement): string => {
  const parts: string[] = [];
  const selector = Object.entries(p?.nodeSelector ?? {});
  if (selector.length) parts.push(`nodes ${selector.map(([k, v]) => `${k}=${v}`).join(', ')}`);
  if (p?.tolerations?.length) parts.push(`tolerates ${p.tolerations.map((t) => t.key ?? 'every taint').join(', ')}`);
  if (p?.affinity) parts.push('affinity rules');
  if (p?.priorityClassName) parts.push(`priority ${p.priorityClassName}`);
  if (p?.runtimeClassName) parts.push(`runtime ${p.runtimeClassName}`);
  return parts.length ? parts.join('; ') : 'Any node';
};
