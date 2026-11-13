import type { MaskOptions } from './mask.ts';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';

import { maskText } from './mask.ts';

const VectorFile = z.strictObject({
  description: z.string().min(1),
  vectors: z.array(z.strictObject({
    name: z.string().min(1),
    input: z.string(),
    want: z.string(),
    options: z.strictObject({
      email: z.boolean().optional(),
      phone: z.boolean().optional(),
      card: z.boolean().optional(),
      secrets: z.boolean().optional(),
      keyPrefix: z.string().optional(),
    }).optional(),
    notCaught: z.literal(true).optional(),
  })),
});

const file = fileURLToPath(new URL('./testdata/mask_vectors.json', import.meta.url));
const raw: unknown = JSON.parse(readFileSync(file, 'utf8'));
const { vectors } = VectorFile.parse(raw);
const defaults: MaskOptions = { email: true, phone: true, card: true, secrets: true };

function optionsOf(v: (typeof vectors)[number]): MaskOptions {
  return { ...defaults, ...v.options };
}

describe('maskText vectors', () => {
  it('has the vectors and refuses an unknown field', () => {
    expect(vectors.length).toBeGreaterThan(40);
    expect(VectorFile.safeParse({ description: 'x', vectors: [{ name: 'n', input: 'i', want: 'w', expected: 'w' }] }).success).toBe(false);
  });

  it.each(vectors.map(v => [v.name, v] as const))('%s', (_name, v) => {
    expect(maskText(v.input, optionsOf(v))).toBe(v.want);
    if (v.notCaught) {
      expect(v.want).toBe(v.input);
    }
  });

  it('is idempotent on every vector', () => {
    for (const v of vectors) {
      expect(maskText(v.want, optionsOf(v)), v.name).toBe(v.want);
    }
  });

  it.each([
    ['a run of local-part letters', 'a'.repeat(50_000)],
    ['dots before one at sign', `${'a.'.repeat(25_000)}@`],
    ['repeated at signs', 'a@'.repeat(25_000)],
    ['one long domain', `a@${'b.'.repeat(25_000)}`],
    ['a run of zeros', '0'.repeat(50_000)],
    ['spaced digits', '4 '.repeat(25_000)],
    ['spaced zeros', '0 '.repeat(25_000)],
    ['repeated 62 mobile starts', '62 8'.repeat(12_500)],
    ['repeated key prefixes', 'sk-'.repeat(20_000)],
    ['an unfinished token', `eyJ${'a'.repeat(50_000)}.`],
    ['a bearer with only spaces', `Bearer ${' '.repeat(50_000)}`],
    ['repeated private key headers', '-----BEGIN RSA PRIVATE KEY-----\n'.repeat(5000)],
    ['repeated basic words', 'Basic '.repeat(20_000)],
    ['repeated google prefixes', 'AIza'.repeat(20_000)],
    ['a bearer with a long slashed token', `Bearer ${'a/'.repeat(25_000)}`],
  ])('stays linear on %s', (_name, input) => {
    const start = performance.now();
    maskText(input, defaults);
    expect(performance.now() - start).toBeLessThan(500);
  });
});
