import type { EndUser, EndUserIdentity } from '~/stores/endUsers';

type Translate = (key: string, params?: Record<string, unknown>) => string;

// identityLabel names one identity for a badge; phones arrive already masked to the last 4 digits.
export function identityLabel(identity: EndUserIdentity, t: Translate): string {
  return t(`endusers.identity_${identity.kind}`, { value: identity.value });
}

// filterEndUsers keeps people whose name or identity matches the query, ignoring case.
export function filterEndUsers(items: EndUser[], query: string): EndUser[] {
  const q = query.trim().toLowerCase();
  if (!q) {
    return items;
  }
  return items.filter(u => [u.name, ...u.identities.map(i => i.value)].some(v => v.toLowerCase().includes(q)));
}

// nameMatches reports whether a typed confirmation matches the person's name, ignoring outer spaces.
export function nameMatches(typed: string, name: string): boolean {
  return typed.trim() === name;
}
