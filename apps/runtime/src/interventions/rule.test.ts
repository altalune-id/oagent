import type { ApprovalSpec } from '../contract/index.ts';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { z } from 'zod';

import { ApprovalSpecSchema } from '../contract/index.ts';
import { evaluate, orderedKeys } from './rule.ts';

const Vector = z.strictObject({
  name: z.string(),
  schema: z.record(z.string(), z.unknown()),
  rule: z.unknown(),
  args: z.record(z.string(), z.unknown()),
  want: z.enum(['free', 'ask']),
  runtimeOnly: z.boolean().optional(),
});

const path = fileURLToPath(new URL('../../../../internal/approval/testdata/rule_vectors.json', import.meta.url));
const vectors = z.array(Vector).parse(JSON.parse(readFileSync(path, 'utf8')));

describe('approval rule', () => {
  it('loads the shared vectors', () => {
    expect(vectors.length).toBeGreaterThanOrEqual(10);
  });

  it.each(vectors.map(v => [v.name, v] as const))('%s', (_name, v) => {
    const parsed = ApprovalSpecSchema.safeParse(v.rule);
    if (!parsed.success) {
      expect(v.want, 'a rule the contract refuses is never free').toBe('ask');
    }
    expect(evaluate(parsed.success ? parsed.data : v.rule as ApprovalSpec, v.schema, v.args)).toBe(v.want);
  });

  it('fails closed when the live schema is missing', () => {
    expect(evaluate({ mode: 'conditional', freeWhen: [{ arg: 'confirm', values: [false], absent: true }] }, undefined, {})).toBe('ask');
  });

  it('orders keys by the schema properties, then the rest sorted', () => {
    const schema = { type: 'object', properties: { amount: {}, account: {}, note: {} } };
    expect(orderedKeys({ note: 'x', zeta: 1, amount: 1, alpha: 2 }, schema)).toEqual(['amount', 'note', 'alpha', 'zeta']);
  });
});
