import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { config } from 'zod';
import './zod';

describe('zod', () => {
  it('runs jitless, so the app never calls Function() under the host CSP', () => {
    expect(config().jitless).toBe(true);
  });

  it('is set before anything else loads, since ext-apps builds its schemas on import', () => {
    const main = readFileSync(new URL('./main.ts', import.meta.url), 'utf8');
    expect(main.trim().split('\n')[0]).toBe('import \'./zod\';');
  });
});
