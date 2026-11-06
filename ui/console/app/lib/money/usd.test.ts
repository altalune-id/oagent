import { describe, expect, it } from 'vitest';
import { parseUsd, usdText } from './usd';

describe('usd', () => {
  it('shows micro-USD as dollars with at least two decimals', () => {
    expect(usdText(400_000n)).toBe('0.40');
    expect(usdText(2_500_001n)).toBe('2.500001');
    expect(usdText(3_000_000n)).toBe('3.00');
    expect(usdText(125_000n)).toBe('0.125');
    expect(usdText(0n)).toBe('0.00');
  });

  it('reads dollars into micro-USD exactly', () => {
    expect(parseUsd(' $0.4 ')).toBe(400_000n);
    expect(parseUsd('1.600000')).toBe(1_600_000n);
    expect(parseUsd('12')).toBe(12_000_000n);
    expect(parseUsd('0.0000001')).toBeNull();
    expect(parseUsd('-1')).toBeNull();
    expect(parseUsd('1e3')).toBeNull();
    expect(parseUsd('')).toBeNull();
  });
});
