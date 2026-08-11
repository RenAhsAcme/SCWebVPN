import type { RawHeaders } from '@mercuryworkshop/proxy-transports';

const hopByHop = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'proxy-connection',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
]);

export function agentTransportURL(remote: URL): URL {
  const target = new URL(remote);
  if (target.protocol === 'https:') {
    target.protocol = 'http:';
    target.port = remote.port || '443';
  } else if (target.protocol === 'wss:') {
    target.protocol = 'ws:';
    target.port = remote.port || '443';
  }
  return target;
}

export function sanitizeUpstreamHeaders(headers: RawHeaders, remote: URL): RawHeaders {
  const nominated = new Set(
    headers
      .filter(([name]) => name.toLowerCase() === 'connection')
      .flatMap(([, value]) => value.split(',').map((name) => name.trim().toLowerCase())),
  );
  return headers
    .filter(([name]) => {
      const lower = name.toLowerCase();
      return !hopByHop.has(lower) && !nominated.has(lower);
    })
    .map(([name, value]) => {
      const lower = name.toLowerCase();
      if (lower === 'location') {
        const target = new URL(value, remote);
        return [name, new URL(target.pathname + target.search + target.hash, remote).href];
      }
      if (lower === 'set-cookie') {
        return [name, value.replace(/;\s*domain=[^;]*/gi, '')];
      }
      return [name, value];
    });
}
