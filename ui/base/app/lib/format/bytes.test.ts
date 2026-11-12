import { describe, expect, it } from 'vitest';
import { formatBytes } from './bytes';

describe('formatBytes', () => {
  it('steps by 1024 and says so with B, KiB, MiB and GiB, at most one decimal in the locale', () => {
    expect(formatBytes(512, 'en-US')).toBe('512 B');
    expect(formatBytes(1536, 'en-US')).toBe('1.5 KiB');
    expect(formatBytes(10 * 1024 * 1024, 'en-US')).toBe('10 MiB');
    expect(formatBytes(3 * 1024 ** 3, 'en-US')).toBe('3 GiB');
    expect(formatBytes(1536, 'id-ID')).toBe('1,5 KiB');
  });

  it('shows 0 B for negative or non-number input', () => {
    expect(formatBytes(-5, 'en-US')).toBe('0 B');
    expect(formatBytes(Number.NaN, 'en-US')).toBe('0 B');
  });
});
