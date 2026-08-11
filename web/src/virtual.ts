export function virtualHostname(id: string): string {
  const bytes = Uint8Array.from(atob(id.replace(/-/g, '+').replace(/_/g, '/') + '=='), (value) =>
    value.charCodeAt(0),
  );
  if (bytes.byteLength !== 16) throw new Error('服务标识不是 128 位值');
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = 0;
  let value = 0;
  let label = '';
  for (const byte of bytes) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      label += alphabet[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits) label += alphabet[(value << (5 - bits)) & 31];
  return `s-${label.toLowerCase()}.webvpn.invalid`;
}
