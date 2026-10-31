import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import en from '../../base/i18n/locales/en-US.json';

const root = dirname(fileURLToPath(import.meta.url));

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      return sources(path);
    }
    return /\.(?:vue|ts)$/.test(entry) && !/\.(?:test|d)\.ts$/.test(entry) ? [path] : [];
  });
}

describe('mcp copy', () => {
  it('names only keys the base locale has, so no view shows a raw key', () => {
    const missing: string[] = [];
    for (const file of sources(root)) {
      for (const [, key] of readFileSync(file, 'utf8').matchAll(/\bt\('([\w.]+)'/g)) {
        if (!(key! in en)) {
          missing.push(`${file.slice(root.length + 1)}: ${key}`);
        }
      }
    }
    expect(missing).toEqual([]);
  });
});
