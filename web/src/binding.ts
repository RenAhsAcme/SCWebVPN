export type BindingCode = {
  code: string;
  expiresAt: number;
};

export function parseBindingCode(value: unknown, now = Date.now()): BindingCode {
  if (!value || typeof value !== 'object') throw new Error('控制面返回了无效的绑定码');
  const response = value as { code?: unknown; expires_at?: unknown };
  if (
    typeof response.code !== 'string' ||
    response.code.length < 16 ||
    response.code.length > 256 ||
    response.code.trim() !== response.code ||
    /[\r\n\0]/.test(response.code)
  ) {
    throw new Error('控制面返回了无效的绑定码');
  }
  const expiresAt =
    typeof response.expires_at === 'string' ? Date.parse(response.expires_at) : Number.NaN;
  if (!Number.isFinite(expiresAt) || expiresAt <= now) {
    throw new Error('控制面返回了已失效的绑定码');
  }
  return { code: response.code, expiresAt };
}
