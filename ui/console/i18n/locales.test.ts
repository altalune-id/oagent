import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { expectSameKeysAndParams, paramsOf } from '#test/locales';
import baseEnUS from '../../base/i18n/locales/en-US.json';
import enUS from './locales/en-US.json';
import idID from './locales/id-ID.json';

const locales: Record<string, Record<string, string>> = { 'en-US': enUS, 'id-ID': idID };
const ui = fileURLToPath(new URL('../../', import.meta.url));

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sources(path);
    }
    return /\.(?:vue|ts)$/.test(entry.name) && !/\.(?:test|stories)\.ts$/.test(entry.name) ? [path] : [];
  });
}

describe('console locales', () => {
  it('have the same keys and the same params per key', () => {
    expectSameKeysAndParams(locales);
  });

  it('compile every message', () => {
    for (const messages of Object.values(locales)) {
      Object.values(messages).forEach(paramsOf);
    }
  });

  it('leave the shared keys to the base layer', () => {
    expect(Object.keys(enUS).filter(key => key in baseEnUS)).toEqual([]);
  });

  it('define every literal key the console and base call t() with', () => {
    const keys = new Set([...Object.keys(enUS), ...Object.keys(baseEnUS)]);
    const used = [join(ui, 'console', 'app'), join(ui, 'base', 'app')].flatMap(sources).flatMap(path => [...readFileSync(path, 'utf8').matchAll(/\bt\('([a-z][\w.]*)'/g)].map(m => `${path.slice(ui.length)}: ${m[1]}`));
    expect(used.filter(entry => !keys.has(entry.split(': ')[1]!))).toEqual([]);
  });
});
