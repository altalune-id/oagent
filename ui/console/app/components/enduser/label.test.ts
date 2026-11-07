import { describe, expect, it } from 'vitest';
import { IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { identityKindLabel, personLabel } from './label';

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}|${JSON.stringify(params)}` : key);

describe('personLabel', () => {
  it('prefers the display name, trimmed', () => {
    expect(personLabel({ displayName: '  Sari Wulandari ', identityKind: IdentityKind.WA_PHONE, identityHint: '90' }, t)).toBe('Sari Wulandari');
  });

  it('falls back to the kind and masked hint, or the kind alone', () => {
    expect(personLabel({ displayName: '', identityKind: IdentityKind.WA_PHONE, identityHint: '90' }, t)).toBe('person.masked|{"kind":"person.kind_wa_phone","hint":"90"}');
    expect(personLabel({ displayName: ' ', identityKind: IdentityKind.WIDGET_ANON, identityHint: '' }, t)).toBe('person.kind_widget_anon');
  });

  it('names every kind, and an unknown one as unknown', () => {
    expect([IdentityKind.API_USER, IdentityKind.API_KEY, IdentityKind.WA_LID, IdentityKind.WIDGET_SUB, IdentityKind.UNSPECIFIED, 99 as IdentityKind].map(k => identityKindLabel(k, t))).toEqual(['person.kind_api_user', 'person.kind_api_key', 'person.kind_wa_lid', 'person.kind_widget_sub', 'person.kind_unknown', 'person.kind_unknown']);
  });
});
