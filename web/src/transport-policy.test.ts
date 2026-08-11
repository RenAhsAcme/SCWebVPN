import { expect, test } from 'bun:test';

import { agentTransportURL, sanitizeUpstreamHeaders } from './transport-policy';

test('terminates HTTPS and WSS at the Agent without changing the fixed authority', () => {
  expect(agentTransportURL(new URL('https://s-test.webvpn.invalid/path')).href).toBe(
    'http://s-test.webvpn.invalid:443/path',
  );
  expect(agentTransportURL(new URL('wss://s-test.webvpn.invalid/socket')).href).toBe(
    'ws://s-test.webvpn.invalid:443/socket',
  );
  expect(agentTransportURL(new URL('http://s-test.webvpn.invalid/path')).href).toBe(
    'http://s-test.webvpn.invalid/path',
  );
});

test('confines redirects and cookies to the synthetic origin', () => {
  const remote = new URL('https://s-test.webvpn.invalid/start');
  expect(
    sanitizeUpstreamHeaders(
      [
        ['Location', 'https://192.168.1.1/admin?next=1'],
        ['Set-Cookie', 'sid=value; Domain=router.lan; Secure; HttpOnly'],
        ['Connection', 'X-Internal-Hop'],
        ['X-Internal-Hop', 'drop'],
        ['Content-Type', 'text/html'],
      ],
      remote,
    ),
  ).toEqual([
    ['Location', 'https://s-test.webvpn.invalid/admin?next=1'],
    ['Set-Cookie', 'sid=value; Secure; HttpOnly'],
    ['Content-Type', 'text/html'],
  ]);
});
