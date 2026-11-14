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

  it('shows a cost under one cent as less than a cent, with the exact amount for its title', () => {
    const { cost, costTitle } = useUnits();
    expect([cost(0.043), costTitle(0.043)]).toEqual(['< $0.01', '$0.00043']);
    expect([cost(0), costTitle(0)]).toEqual(['$0.00', undefined]);
    expect([cost(1.6), costTitle(1.6)]).toEqual(['$0.016', undefined]);
  });

  it('formats a media length as minutes and zero-padded seconds', () => {
    const { clock } = useUnits();
    expect([clock(0), clock(14), clock(59), clock(60), clock(299), clock(300), clock(605)]).toEqual(['0:00', '0:14', '0:59', '1:00', '4:59', '5:00', '10:05']);
  });
});
