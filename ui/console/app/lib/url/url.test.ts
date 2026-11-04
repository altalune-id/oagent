import { describe, expect, it } from 'vitest';
import { isHttpsUrl } from './url';

describe('isHttpsUrl', () => {
  it('accepts only full https URLs', () => {
    expect(isHttpsUrl('https://openrouter.ai/api/v1')).toBe(true);
    expect(isHttpsUrl('http://openrouter.ai')).toBe(false);
    expect(isHttpsUrl('openrouter')).toBe(false);
  });
});
