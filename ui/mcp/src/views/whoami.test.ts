import type { WhoamiData } from './whoami';

import { describe, expect, it } from 'vitest';
import { permissionBadges, sourceLabel, whoamiRows, whoamiTitle } from './whoami';

const person: WhoamiData = {
  source: 'local',
  userId: 'u1',
  email: 'ada@example.com',
  name: 'Ada',
  activeOrgSlug: 'acme',
  permissions: ['*'],
  operator: true,
};

function values(data: WhoamiData): string[] {
  return whoamiRows(data).map(r => r.value);
}

describe('whoamiTitle', () => {
  it('names a person', () => {
    expect(whoamiTitle(person)).toBe('Ada');
    expect(whoamiTitle({ ...person, name: '' })).toBe('ada@example.com');
  });

  it('labels a key, system and basic', () => {
    expect(whoamiTitle({ source: 'apikey', keyHint: 'k1x9' })).toBe('API key');
    expect(whoamiTitle({ source: 'system', name: 'stdio', operator: true })).toBe('System (stdio)');
    expect(whoamiTitle({ source: 'basic', name: 'admin', operator: true })).toBe('Operator (basic)');
  });
});

describe('whoamiRows', () => {
  it('shows a person', () => {
    expect(whoamiRows(person)).toEqual([
      { label: 'Email', value: 'ada@example.com' },
      { label: 'Org', value: 'acme' },
    ]);
  });

  it('shows an org key by its id', () => {
    expect(whoamiRows({ source: 'apikey', keyHint: 'k1x9', activeOrgSlug: 'acme', permissions: ['agent:read'] })).toEqual([
      { label: 'Key', value: '…k1x9' },
      { label: 'Org', value: 'acme' },
    ]);
  });

  it('shows the owner of a personal key', () => {
    const rows = whoamiRows({ source: 'apikey', keyHint: 'k2y8', ownerId: 'V1StGXR8Z5jdHi6BmyT2a', email: 'ada@example.com', name: 'Ada', activeOrgSlug: 'acme' });
    expect(rows).toEqual([
      { label: 'Acts for', value: 'Ada <ada@example.com>' },
      { label: 'Key', value: '…k2y8' },
      { label: 'Org', value: 'acme' },
    ]);
  });

  it('shows nothing for missing fields', () => {
    expect(whoamiRows({})).toEqual([]);
    expect(whoamiRows({ source: 'system', name: 'stdio', operator: true })).toEqual([]);
    for (const v of [...values({ source: 'apikey' }), ...values({ source: 'local', email: 'a@b.c' })]) {
      expect(v).not.toMatch(/undefined|null/);
    }
  });
});

describe('sourceLabel', () => {
  it('maps every source', () => {
    expect(sourceLabel('local')).toBe('Signed in');
    expect(sourceLabel('apikey')).toBe('API key');
    expect(sourceLabel('system')).toBe('System (stdio)');
    expect(sourceLabel('basic')).toBe('Operator (basic)');
    expect(sourceLabel(undefined)).toBe('');
  });
});

describe('permissionBadges', () => {
  it('collapses after eight', () => {
    const perms = Array.from({ length: 11 }, (_, i) => `p${i}`);
    expect(permissionBadges(perms)).toEqual({ shown: perms.slice(0, 8), more: 3 });
  });

  it('keeps short lists and handles none', () => {
    expect(permissionBadges(['agent:read'])).toEqual({ shown: ['agent:read'], more: 0 });
    expect(permissionBadges(undefined)).toEqual({ shown: [], more: 0 });
  });
});
