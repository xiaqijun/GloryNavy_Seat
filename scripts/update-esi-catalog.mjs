// Run explicitly when reviewing a compatibility-date upgrade; never at runtime.
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';

const dir = new URL('../internal/modules/eve/internal/esiclient/', import.meta.url);
const types = await readFile(new URL('types.go', dir), 'utf8');
const date = types.match(/const CompatibilityDate = "([\d-]+)"/)?.[1];
if (!date) throw new Error('Missing compatibility date');
const source = `https://esi.evetech.net/meta/openapi.json?compatibility_date=${date}`;
const response = await fetch(source, { signal: AbortSignal.timeout(30000) });
if (!response.ok) throw new Error(`OpenAPI HTTP ${response.status}`);
const raw = await response.text();
const spec = JSON.parse(raw);
if (spec.info?.version !== date) throw new Error('Unexpected OpenAPI version');
const routes = {};
const groups = new Map();
for (const [path, item] of Object.entries(spec.paths).sort()) {
  for (const method of ['get', 'post', 'put', 'delete', 'patch', 'head', 'options']) {
    const op = item[method];
    if (!op) continue;
    const key = `${method.toUpperCase()} ${path.replace(/\{[^}]+\}/g, '{id}').replace(/\/$/, '')}/`;
    const rate = op['x-rate-limit'];
    const policy = { group: '', capacity: 0, window_seconds: 0, cache_seconds: op['x-client-cache-ttl'] ?? 0 };
    if (rate) {
      const window = rate['window-size'].match(/^(\d+)(s|m|h)$/);
      policy.group = rate.group;
      policy.capacity = rate['max-tokens'];
      policy.window_seconds = window ? Number(window[1]) * ({ s: 1, m: 60, h: 3600 }[window[2]]) : 0;
      if (!/^[a-zA-Z0-9_.:-]{1,120}$/.test(policy.group) || !Number.isSafeInteger(policy.capacity) || policy.capacity <= 0 || policy.capacity >= 10000000 || policy.window_seconds < 1 || policy.window_seconds > 86400) throw new Error(`Invalid rate policy: ${key}`);
      const shape = `${policy.capacity}/${policy.window_seconds}`;
      if (groups.has(policy.group) && groups.get(policy.group) !== shape) throw new Error(`Inconsistent group: ${policy.group}`);
      groups.set(policy.group, shape);
    }
    if (!Number.isSafeInteger(policy.cache_seconds) || policy.cache_seconds < 0) throw new Error(`Invalid cache TTL: ${key}`);
    if (routes[key]) throw new Error(`Duplicate normalized route: ${key}`);
    routes[key] = policy;
  }
}
if (!Object.keys(routes).length) throw new Error('Empty catalog');
await writeFile(new URL('catalog.json', dir), JSON.stringify({ compatibility_date: date, source, sha256: createHash('sha256').update(raw).digest('hex'), routes }, null, 2) + '\n');
console.log(`ESI ${date}: ${Object.keys(routes).length} routes, ${groups.size} groups`);
