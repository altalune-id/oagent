import { describe, expect, it } from 'vitest';
import { sampleEndUsers } from '~/preview/samples';
import { filterEndUsers, identityLabel, nameMatches } from './endUser';

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}|${JSON.stringify(params)}` : key);
const { items } = sampleEndUsers();

describe('end users', () => {
  it('labels each identity by kind and keeps phones masked', () => {
    expect(items[0]!.identities.map(i => identityLabel(i, t))).toEqual([
      'endusers.identity_phone|{"value":"+62 800 •••• 0001"}',
      'endusers.identity_lid|{"value":"•••• 4417"}',
    ]);
    expect(items.flatMap(u => u.identities).filter(i => i.kind === 'phone').every(i => /•••• \d{4}$/.test(i.value))).toBe(true);
  });

  it('filters by name or identity, ignoring case', () => {
    expect(filterEndUsers(items, '  ')).toHaveLength(3);
    expect(filterEndUsers(items, 'SARI').map(u => u.id)).toEqual(['eu_sari2']);
    expect(filterEndUsers(items, '0001').map(u => u.id)).toEqual(['eu_budi1', 'eu_api3']);
    expect(filterEndUsers(items, 'zzz')).toEqual([]);
  });

  it('needs the exact name to confirm a delete', () => {
    expect(nameMatches(' Sari ', 'Sari')).toBe(true);
    expect(nameMatches('sari', 'Sari')).toBe(false);
  });
});
