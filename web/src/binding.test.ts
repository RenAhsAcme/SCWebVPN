import { describe, expect, test } from 'bun:test';

import { parseBindingCode } from './binding';

describe('binding code response', () => {
  const now = Date.parse('2026-08-10T10:00:00Z');

  test('accepts one in-memory code with a future expiry', () => {
    expect(
      parseBindingCode(
        {
          code: '0123456789abcdef0123456789abcdef',
          expires_at: '2026-08-10T10:05:00Z',
        },
        now,
      ),
    ).toEqual({ code: '0123456789abcdef0123456789abcdef', expiresAt: now + 300_000 });
  });

  const invalid = [
    null,
    {},
    { code: 'too-short', expires_at: '2026-08-10T10:05:00Z' },
    { code: '0123456789abcdef\nsecond-line', expires_at: '2026-08-10T10:05:00Z' },
    { code: '0123456789abcdef0123456789abcdef', expires_at: '2026-08-10T09:59:59Z' },
  ];
  invalid.forEach((value, index) => {
    test(`rejects malformed or expired response ${index + 1}`, () => {
      expect(() => parseBindingCode(value, now)).toThrow();
    });
  });
});
