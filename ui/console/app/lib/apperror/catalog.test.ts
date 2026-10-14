import { createParser } from '@intlify/message-compiler';
import codes from '@shared/apperror/codes.json';
import { describe, expect, it } from 'vitest';

import enUS from '../../../i18n/locales/en-US.json';
import idID from '../../../i18n/locales/id-ID.json';

const locales: Record<string, Record<string, string>> = { 'en-US': enUS, 'id-ID': idID };

// @intlify/message-compiler's NodeTypes is a const enum, so its values are not exported at runtime.
const NAMED = 4;

function paramsOf(message: string): string[] {
  const errors: string[] = [];
  const ast = createParser({ onError: err => errors.push(err.message) }).parse(message);
  expect(errors, `"${message}" does not compile`).toEqual([]);
  const names: string[] = [];
  const walk = (node: unknown): void => {
    if (Array.isArray(node)) {
      node.forEach(walk);
      return;
    }
    if (node && typeof node === 'object') {
      const n = node as { type?: number, key?: string };
      if (n.type === NAMED && typeof n.key === 'string') {
        names.push(n.key);
      }
      Object.values(node).forEach(walk);
    }
  };
  walk(ast);
  return names;
}

describe.each(Object.entries(locales))('%s', (_, messages) => {
  const known = new Set(codes.flatMap(c => [`apperror.${c.code}`, ...c.reasons.map(r => `apperror.${c.code}_${r}`)]));

  it.each(codes)('translates $code and its reasons with declared params only', (entry) => {
    for (const key of [`apperror.${entry.code}`, ...entry.reasons.map(r => `apperror.${entry.code}_${r}`)]) {
      const message = messages[key];
      expect(message, `${key} is missing`).toBeTypeOf('string');
      for (const param of paramsOf(message!)) {
        expect(entry.params, `${key} uses {${param}}`).toContain(param);
      }
    }
  });

  it('has no apperror key the catalog does not know', () => {
    const stray = Object.keys(messages).filter(k => k.startsWith('apperror.') && !known.has(k));
    expect(stray).toEqual([]);
  });

  it('compiles every message', () => {
    for (const message of Object.values(messages)) {
      paramsOf(message);
    }
  });
});

describe('locales', () => {
  it('have the same keys and the same params per key', () => {
    const [base, ...rest] = Object.values(locales);
    for (const other of rest) {
      expect(Object.keys(other!).sort()).toEqual(Object.keys(base!).sort());
      for (const [key, message] of Object.entries(base!)) {
        expect(paramsOf(other![key]!).sort(), key).toEqual(paramsOf(message).sort());
      }
    }
  });
});
