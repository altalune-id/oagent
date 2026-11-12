import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { argsHash, canonicalJson } from './canonical.ts';

type Vector = { name: string, input: string, canonical?: string, argsHash?: string, error?: boolean };

const file = fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/canonical_vectors.json', import.meta.url));
const { vectors } = JSON.parse(readFileSync(file, 'utf8')) as { vectors: Vector[] };

describe('canonicalJson', () => {
  it('has the shared vectors', () => {
    expect(vectors.length).toBeGreaterThan(20);
  });

  for (const v of vectors) {
    it(v.name, () => {
      if (v.error) {
        expect(() => canonicalJson(JSON.parse(v.input))).toThrow();
        return;
      }
      const value: unknown = JSON.parse(v.input);
      expect(canonicalJson(value)).toBe(v.canonical);
      expect(argsHash(value)).toBe(v.argsHash);
    });
  }

  it('refuses values JSON cannot hold', () => {
    for (const bad of [Number.NaN, Number.POSITIVE_INFINITY, undefined, () => 1, 1n]) {
      expect(() => canonicalJson({ a: bad })).toThrow();
    }
  });
});
