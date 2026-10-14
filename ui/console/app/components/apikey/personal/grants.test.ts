import type { Scope } from './grants';

import { describe, expect, it } from 'vitest';
import { grants, mintableForMe } from './grants';

describe('grants', () => {
  it('matches like scope.Grants in Go', () => {
    expect(grants(['*'], 'agent:read')).toBe(true);
    expect(grants(['agent:*'], 'agent:read')).toBe(true);
    expect(grants(['agent:*'], 'webhook:read')).toBe(false);
    expect(grants(['agent:read'], 'agent:read')).toBe(true);
    expect(grants(['agent:read'], 'agent:write')).toBe(false);
    expect(grants([], 'agent:read')).toBe(false);
    expect(grants(['*'], '')).toBe(false);
  });
});

describe('mintableForMe', () => {
  const catalog: Scope[] = [
    { name: 'org:write', level: 'org', labelKey: 'scope.org_write' },
    { name: 'agent:read', level: 'project', labelKey: 'scope.agent_read' },
    { name: 'agent:write', level: 'project', labelKey: 'scope.agent_write' },
    { name: 'apikey:write', level: 'org', labelKey: 'scope.apikey_write', personOnly: true },
  ];

  it('drops person-only and above-role scopes', () => {
    expect(mintableForMe(catalog, ['agent:*', 'apikey:write']).map(s => s.name)).toEqual(['agent:read', 'agent:write']);
  });

  it('an owner mints every mintable scope', () => {
    expect(mintableForMe(catalog, ['*']).map(s => s.name)).toEqual(['org:write', 'agent:read', 'agent:write']);
  });

  it('nothing without permissions', () => {
    expect(mintableForMe(catalog, [])).toEqual([]);
  });
});
