import { describe, expect, it } from 'vitest';
import { maxPagesValid, websiteValid } from './knowledge';

describe('knowledge sources', () => {
  it('accepts only full http or https website addresses', () => {
    expect(websiteValid(' https://bank.example.id/promo ')).toBe(true);
    expect(websiteValid('http://example.id')).toBe(true);
    expect(websiteValid('bank.example.id/promo')).toBe(false);
    expect(websiteValid('ftp://bank.example.id')).toBe(false);
    expect(websiteValid('https://localhost')).toBe(false);
  });

  it('keeps the page limit a whole number from 1 to 500', () => {
    expect([1, 50, 500].every(maxPagesValid)).toBe(true);
    expect([0, 501, 2.5, Number.NaN].some(maxPagesValid)).toBe(false);
  });
});
