import { expect, test } from 'bun:test';

import { virtualHostname } from './virtual';

test('matches the Agent synthetic authority without exposing a target', () => {
  expect(virtualHostname('AQIDBAUGBwgJCgsMDQ4PEA')).toBe(
    's-aebagbafaydqqcikbmga2dqpca.webvpn.invalid',
  );
});

test('rejects identifiers that are not 128 bits', () => {
  expect(() => virtualHostname('AQID')).toThrow();
});
