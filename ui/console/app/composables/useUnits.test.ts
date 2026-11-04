import { describe, expect, it } from 'vitest';
import { useUnits } from './useUnits';
import '#test/nuxt-globals';

describe('useUnits', () => {
  it('formats counts, seconds and cents as dollars in en-US', () => {
    const { count, seconds, money } = useUnits();
    expect(count(12904)).toBe('12,904');
    expect(seconds(41)).toBe('41s');
    expect(seconds(2.84)).toBe('2.8s');
    expect(money(386)).toBe('$3.86');
    expect(money(1.6)).toBe('$0.016');
  });
});
