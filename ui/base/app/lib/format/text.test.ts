import { describe, expect, it } from 'vitest';
import { codePoints } from './text';

describe('codePoints', () => {
  it('counts code points, not UTF-16 units, so an emoji is one', () => {
    expect(codePoints('')).toBe(0);
    expect(codePoints('abc')).toBe(3);
    expect(codePoints('hi 😀')).toBe(4);
    expect('hi 😀'.length).toBe(5);
  });
});
