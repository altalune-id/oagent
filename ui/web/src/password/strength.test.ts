import type { Reason } from './policy';
import type { Score } from './strength';
import { describe, expect, it } from 'vitest';
import cases from '../../../../password/testdata/cases.json';
import { violations } from './policy';
import { label, score } from './strength';

interface Case { name: string; password: string; email?: string; name_hint?: string; reasons: Reason[]; score: Score; }

describe('score', () => {
  it.each(cases as Case[])('$name', (c) => {
    expect(score(c.password, violations(c.password, { email: c.email, name: c.name_hint }))).toBe(c.score);
  });
});

describe('label', () => {
  it('names every score', () => {
    expect(([0, 1, 2, 3, 4] as Score[]).map(label)).toEqual(['Too weak', 'Weak', 'Fair', 'Good', 'Strong']);
  });
});
