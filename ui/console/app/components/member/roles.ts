export type Role = 'owner' | 'admin' | 'member';

export const roles: Role[] = ['owner', 'admin', 'member'];

// role is '' when the person is not a member of the org.
export interface Party {
  userId: string;
  role: string;
}

function isRole(role: string): role is Role {
  return (roles as string[]).includes(role);
}

// Mirrors member.Authorize (internal/org/member/rules.go) to hide controls; the server stays the authority.
function allowed(viewer: Party, target: Party, to: string): boolean {
  if (to !== '' && !isRole(to)) {
    return false;
  }
  if (!viewer.userId || viewer.userId === target.userId) {
    return false;
  }
  if (viewer.role === 'owner') {
    return true;
  }
  if (viewer.role !== 'admin') {
    return false;
  }
  return target.role !== 'owner' && to !== 'owner';
}

export function canChange(viewer: Party, target: Party, to: string): boolean {
  return to !== '' && allowed(viewer, target, to);
}

export function canRemove(viewer: Party, target: Party): boolean {
  return allowed(viewer, target, '');
}

// Mirrors invite.Authorize (internal/org/invite/rules.go): you invite only to a role you hold all of.
export function inviteRoles(viewerRole: string): Role[] {
  switch (viewerRole) {
    case 'owner':
      return [...roles];
    case 'admin':
      return ['admin', 'member'];
    default:
      return [];
  }
}

// roleOptions is empty when the viewer may not change the target's role at all.
export function roleOptions(viewer: Party, target: Party): Role[] {
  return roles.filter(r => canChange(viewer, target, r));
}
