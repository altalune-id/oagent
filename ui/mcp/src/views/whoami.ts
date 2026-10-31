import { t } from '../copy';

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
  local: t('mcp.signed_in'),
  oidc: t('mcp.signed_in'),
  token: t('mcp.signed_in'),
  apikey: t('mcp.whoami_apikey'),
  system: t('mcp.whoami_system'),
  basic: t('mcp.whoami_basic'),
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
      add(t('mcp.whoami_acts_for'), data.name && data.email ? `${data.name} <${data.email}>` : data.email || data.name);
    }
    if (data.keyHint) {
      add(t('mcp.whoami_key'), `…${data.keyHint}`);
    }
  } else if (data.source !== 'system' && data.source !== 'basic') {
    if (data.name) {
      add(t('field.email'), data.email);
    }
  }
  add(t('mcp.whoami_org'), data.activeOrgSlug);
  return rows;
}

export function permissionBadges(permissions: string[] | undefined, max = 8): { shown: string[], more: number } {
  const all = permissions ?? [];
  return { shown: all.slice(0, max), more: Math.max(0, all.length - max) };
}
