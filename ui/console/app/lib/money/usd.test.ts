import { describe, expect, it } from 'vitest';
import { decimalSeparator, parseUsd, per1kText, perCallMicros, usdText } from './usd';

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

  it('reads a price per 1,000 calls into micro-USD per call', () => {
    expect(perCallMicros('5')).toBe(5000n);
    expect(perCallMicros('0')).toBe(0n);
    expect(perCallMicros('8.125')).toBe(8125n);
    expect(perCallMicros('8.1255')).toBeNull();
    expect(perCallMicros('')).toBeNull();
    expect(perCallMicros('abc')).toBeNull();
  });

  it('reads the locale\'s decimal separator, and only with it', () => {
    expect(decimalSeparator('en-US')).toBe('.');
    expect(decimalSeparator('id-ID')).toBe(',');
    expect(parseUsd('5,00', ',')).toBe(5_000_000n);
    expect(parseUsd('5.00', ',')).toBe(5_000_000n);
    expect(parseUsd('5,00')).toBeNull();
    expect(parseUsd('1.600,50', ',')).toBeNull();
    expect(perCallMicros('8,125', ',')).toBe(8125n);
    expect(perCallMicros('8,1255', ',')).toBeNull();
    expect(perCallMicros('1,600')).toBeNull();
  });

  it('shows a per-call price as the price per 1,000 calls', () => {
    expect(per1kText(8125n)).toBe('8.125');
    expect(per1kText(perCallMicros('5')!)).toBe('5.00');
  });
});
