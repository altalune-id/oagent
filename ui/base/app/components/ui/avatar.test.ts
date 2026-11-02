import { describe, expect, it } from 'vitest';
import { avatarView, initials } from './avatar';

describe('initials', () => {
  it('takes the first and last word of the name, else the first letter of the email', () => {
    expect(initials('Ana Maria', 'ana@x.io')).toBe('AM');
    expect(initials('  ana  de la maria ', '')).toBe('AM');
    expect(initials('Cy', 'cy@x.io')).toBe('C');
    expect(initials('', 'bob@x.io')).toBe('B');
    expect(initials('   ', '')).toBe('?');
    expect(initials('Élodie Ünal', '')).toBe('ÉÜ');
  });
});

describe('avatarView', () => {
  it('shows the picture when there is one, with the name as its alt text', () => {
    expect(avatarView({ id: 'x', name: 'Ana Maria', email: 'ana@x.io', avatarUrl: 'https://x/a.png' }))
      .toEqual({ kind: 'image', src: 'https://x/a.png', alt: 'Ana Maria' });
    expect(avatarView({ id: 'x', name: '', email: 'ana@x.io', avatarUrl: 'https://x/a.png' }))
      .toEqual({ kind: 'image', src: 'https://x/a.png', alt: 'ana@x.io' });
  });

  it('shows initials otherwise', () => {
    expect(avatarView({ id: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Ana Maria', email: 'ana@x.io' }))
      .toEqual({ kind: 'initials', text: 'AM', label: 'Ana Maria' });
    expect(avatarView({ id: '', name: '', email: 'bob@x.io', avatarUrl: '' }))
      .toEqual({ kind: 'initials', text: 'B', label: 'bob@x.io' });
  });
});
