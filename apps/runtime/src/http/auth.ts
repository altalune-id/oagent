import type { Buffer } from 'node:buffer';
import { createHash, timingSafeEqual } from 'node:crypto';

function digest(value: string): Buffer {
  return createHash('sha256').update(value).digest();
}

export function bearerCheck(token: string): (header: string | undefined) => boolean {
  const want = digest(token);
  return (header) => {
    const m = /^Bearer (.+)$/.exec(header ?? '');
    return m !== null && timingSafeEqual(digest(m[1]!), want);
  };
}
