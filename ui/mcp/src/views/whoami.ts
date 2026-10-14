// WhoamiData is auth.v1.WhoamiResponse as protojson sends it: empty fields are left out.
export interface WhoamiData {
  source?: string;
  userId?: string;
  email?: string;
  name?: string;
  avatarUrl?: string;
  keyHint?: string;
  ownerId?: string;
  activeOrgId?: string;
  activeOrgSlug?: string;
  activeProjectId?: string;
  permissions?: string[];
  operator?: boolean;
}

export interface Row {
  label: string;
  value: string;
}

const sources: Record<string, string> = {
  local: 'Signed in',
  oidc: 'Signed in',
  token: 'Signed in',
  apikey: 'API key',
  system: 'System (stdio)',
  basic: 'Operator (basic)',
};

export function sourceLabel(source: string | undefined): string {
  return source ? (sources[source] ?? source) : '';
}

export function whoamiTitle(data: WhoamiData): string {
  switch (data.source) {
    case 'apikey':
    case 'system':
    case 'basic':
      return sourceLabel(data.source);
  }
  return data.name || data.email || '';
}

export function whoamiRows(data: WhoamiData): Row[] {
  const rows: Row[] = [];
  const add = (label: string, value: string | undefined) => {
    if (value) {
      rows.push({ label, value });
    }
  };
  if (data.source === 'apikey') {
    if (data.ownerId) {
      add('Acts for', data.name && data.email ? `${data.name} <${data.email}>` : data.email || data.name);
    }
    if (data.keyHint) {
      add('Key', `…${data.keyHint}`);
    }
  } else if (data.source !== 'system' && data.source !== 'basic') {
    if (data.name) {
      add('Email', data.email);
    }
  }
  add('Org', data.activeOrgSlug);
  return rows;
}

export function permissionBadges(permissions: string[] | undefined, max = 8): { shown: string[], more: number } {
  const all = permissions ?? [];
  return { shown: all.slice(0, max), more: Math.max(0, all.length - max) };
}
