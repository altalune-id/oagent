import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { SEARCH_KINDS } from '../../../contract/index.ts';
import { drivers } from './index.ts';

describe('search drivers', () => {
  it('registers exactly the kinds Go lists', () => {
    const shared = JSON.parse(readFileSync(fileURLToPath(new URL('../../../../../../internal/webtool/testdata/search_kinds.json', import.meta.url)), 'utf8')) as string[];
    expect(Object.keys(drivers).sort()).toEqual([...shared].sort());
    expect([...SEARCH_KINDS].sort()).toEqual([...shared].sort());
  });
});
