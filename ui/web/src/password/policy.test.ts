import type { Reason } from './policy';
import { describe, expect, it } from 'vitest';
import cases from '../../../../password/testdata/cases.json';
import { isCommon, policy, violations } from './policy';

interface Case { name: string; password: string; email?: string; name_hint?: string; reasons: Reason[]; score: number; }

describe('violations', () => {
  it.each(cases as Case[])('$name', (c) => {
    expect(violations(c.password, { email: c.email, name: c.name_hint })).toEqual(c.reasons);
  });

  it('treats missing hints as empty', () => {
    expect(violations('correct horse battery')).toEqual([]);
  });
});

describe('policy', () => {
  it('reads the shared lengths', () => {
    expect(policy.minLength).toBe(12);
    expect(policy.maxLength).toBe(256);
    expect(policy.hintMinLength).toBe(4);
  });

  it('loads the shared blocklist without its header', () => {
    expect(isCommon('iloveyou1234')).toBe(true);
    expect(isCommon('# license: mit (seclists)')).toBe(false);
    expect(isCommon('correct horse battery')).toBe(false);
  });
});
