// MemberData is member.v1.Member as protojson sends it: empty fields are left out.
export interface MemberData {
  userId?: string;
  email?: string;
  name?: string;
  role?: string;
  joinedAt?: string;
}

export interface MemberRow {
  id: string;
  name: string;
  email: string;
  role: string;
}

const roles: Record<string, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: 'Member',
};

export function roleLabel(role: string | undefined): string {
  return role ? (roles[role] ?? role) : '';
}

export function memberRows(data: Record<string, unknown> | undefined): MemberRow[] {
  const list = data?.members;
  if (!Array.isArray(list)) {
    return [];
  }
  return list
    .filter((m): m is MemberData => typeof m === 'object' && m !== null)
    .map(m => ({ id: m.userId ?? '', name: m.name ?? '', email: m.email ?? '', role: roleLabel(m.role) }));
}
