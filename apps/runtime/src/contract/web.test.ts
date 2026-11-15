import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { MODEL_REFUSED_HEADERS, RESERVED_HEADERS, validHost } from './web.ts';

function shared<T>(path: string): T {
  return JSON.parse(readFileSync(fileURLToPath(new URL(`../../../../internal/kernel/webpolicy/testdata/${path}`, import.meta.url)), 'utf8')) as T;
}

describe('header policy', () => {
  it('reserves exactly the headers Go reserves', () => {
    const headers = shared<{ reserved: string[], modelRefused: string[] }>('headers.json');
    expect([...RESERVED_HEADERS].sort()).toEqual(headers.reserved.map(h => h.toLowerCase()).sort());
    expect([...MODEL_REFUSED_HEADERS].sort()).toEqual(headers.modelRefused.map(h => h.toLowerCase()).sort());
  });
});

describe('validHost', () => {
  it('matches every shared host vector', () => {
    for (const v of shared<{ host: string, valid: boolean }[]>('hosts.json')) {
      expect({ host: v.host, valid: validHost(v.host) }).toEqual(v);
    }
  });
});
