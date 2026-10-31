import { describe, expect, it } from 'vitest';

import { roleLabel } from './members';

describe('roleLabel', () => {
  it('names every role and passes unknown ones through', () => {
    expect(roleLabel('owner')).toBe('Owner');
    expect(roleLabel('admin')).toBe('Admin');
    expect(roleLabel('member')).toBe('Member');
    expect(roleLabel('guest')).toBe('guest');
    expect(roleLabel(undefined)).toBe('');
  });
});
