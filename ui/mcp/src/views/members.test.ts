import { describe, expect, it } from 'vitest';

import { memberRows, roleLabel } from './members';

describe('memberRows', () => {
  it('maps members as protojson sends them', () => {
    expect(
      memberRows({
        members: [
          { userId: 'u1', email: 'ada@example.com', name: 'Ada', role: 'owner', joinedAt: '2026-10-01T00:00:00Z' },
          { userId: 'u2', email: 'bob@example.com', role: 'member' },
        ],
      }),
    ).toEqual([
      { id: 'u1', name: 'Ada', email: 'ada@example.com', role: 'Owner' },
      { id: 'u2', name: '', email: 'bob@example.com', role: 'Member' },
    ]);
  });

  it('reads an empty or malformed result as no rows', () => {
    expect(memberRows({})).toEqual([]);
    expect(memberRows(undefined)).toEqual([]);
    expect(memberRows({ members: 'nope' })).toEqual([]);
    expect(memberRows({ members: [null, 3] })).toEqual([]);
  });
});

describe('roleLabel', () => {
  it('names every role and passes unknown ones through', () => {
    expect(roleLabel('owner')).toBe('Owner');
    expect(roleLabel('admin')).toBe('Admin');
    expect(roleLabel('member')).toBe('Member');
    expect(roleLabel('guest')).toBe('guest');
    expect(roleLabel(undefined)).toBe('');
  });
});
