import { describe, expect, it } from 'vitest';

import { displayName, initials } from './names';

describe('initials', () => {
  it('takes the first and last word of the name, else the email', () => {
    expect(initials('Ann Lee', 'a@x')).toBe('AL');
    expect(initials('  ann  maria lee ', 'a@x')).toBe('AL');
    expect(initials('Cy', 'c@x')).toBe('C');
    expect(initials('', 'bob@example.com')).toBe('B');
    expect(initials(' ', '')).toBe('?');
    expect(initials('Élodie Ünal', '')).toBe('ÉÜ');
  });
});

describe('displayName', () => {
  it('trims the name and falls back to the email', () => {
    expect(displayName({ name: ' Ann Lee ', email: 'ann@example.com' })).toBe('Ann Lee');
    expect(displayName({ name: '  ', email: 'bob@example.com' })).toBe('bob@example.com');
  });
});
