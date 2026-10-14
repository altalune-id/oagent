import type { Role } from './roles';

import { describe, expect, it } from 'vitest';
import { canChange, canRemove, inviteRoles, roleOptions, roles } from './roles';

// Same rows as member.Authorize's table test (internal/org/member/rules_test.go), minus the operator: the console viewer is always a person.
const viewers: Record<string, string> = { 'owner': 'owner', 'admin': 'admin', 'member': 'member', 'non-member': '' };

function want(viewer: string, from: Role, to: Role | ''): boolean {
  switch (viewer) {
    case 'owner':
      return true;
    case 'admin':
      return from !== 'owner' && to !== 'owner';
    default:
      return false;
  }
}

describe('canChange and canRemove', () => {
  for (const [name, role] of Object.entries(viewers)) {
    for (const from of roles) {
      for (const to of [...roles, ''] as (Role | '')[]) {
        it(`${name}: ${from} -> "${to}"`, () => {
          const viewer = { userId: 'v', role };
          const target = { userId: 't', role: from };
          const got = to === '' ? canRemove(viewer, target) : canChange(viewer, target, to);
          expect(got).toBe(want(name, from, to));
        });
      }
    }
  }

  for (const self of roles) {
    for (const to of [...roles, ''] as (Role | '')[]) {
      it(`self ${self} -> "${to}" is refused`, () => {
        const viewer = { userId: 'v', role: self };
        const target = { userId: 'v', role: self };
        expect(to === '' ? canRemove(viewer, target) : canChange(viewer, target, to)).toBe(false);
      });
    }
  }

  for (const role of Object.values(viewers)) {
    it(`${role || 'non-member'} cannot set an unknown role`, () => {
      expect(canChange({ userId: 'v', role }, { userId: 't', role: 'member' }, 'root')).toBe(false);
    });
  }

  it('refuses a viewer with no user id', () => {
    expect(canRemove({ userId: '', role: 'owner' }, { userId: '', role: 'member' })).toBe(false);
  });
});

describe('inviteRoles', () => {
  it('lets owners invite to any role and admins to admin or member', () => {
    expect(inviteRoles('owner')).toEqual(['owner', 'admin', 'member']);
    expect(inviteRoles('admin')).toEqual(['admin', 'member']);
    expect(inviteRoles('member')).toEqual([]);
    expect(inviteRoles('')).toEqual([]);
  });
});

describe('roleOptions', () => {
  it('lists the roles the viewer may set, including the current one', () => {
    expect(roleOptions({ userId: 'v', role: 'owner' }, { userId: 't', role: 'member' })).toEqual(['owner', 'admin', 'member']);
    expect(roleOptions({ userId: 'v', role: 'admin' }, { userId: 't', role: 'member' })).toEqual(['admin', 'member']);
    expect(roleOptions({ userId: 'v', role: 'admin' }, { userId: 't', role: 'owner' })).toEqual([]);
    expect(roleOptions({ userId: 'v', role: 'member' }, { userId: 't', role: 'admin' })).toEqual([]);
    expect(roleOptions({ userId: 'v', role: 'owner' }, { userId: 'v', role: 'owner' })).toEqual([]);
  });
});
