import { describe, expect, it } from 'vitest';

import { isPerson } from './person';

describe('isPerson', () => {
  it('counts a password and an OIDC sign-in as a person', () => {
    expect(isPerson({ source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc' })).toBe(true);
    expect(isPerson({ source: 'oidc', userId: 'F5pLm2XwQ8kRt3VnZ7bYc' })).toBe(true);
  });

  it('never counts a key, basic auth, the system or a session without a user', () => {
    for (const source of ['apikey', 'basic', 'system', '']) {
      expect(isPerson({ source, userId: 'F5pLm2XwQ8kRt3VnZ7bYc' }), source).toBe(false);
    }
    expect(isPerson({ source: 'oidc', userId: '' })).toBe(false);
    expect(isPerson(null)).toBe(false);
    expect(isPerson(undefined)).toBe(false);
  });
});
