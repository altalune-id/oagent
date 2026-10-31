import codes from '@shared/apperror/codes.json';
import { describe, expect, it } from 'vitest';

import enUS from '../../../i18n/locales/en-US.json';
import idID from '../../../i18n/locales/id-ID.json';
import { expectSameKeysAndParams, paramsOf } from '../../../test/locales';

const locales: Record<string, Record<string, string>> = { 'en-US': enUS, 'id-ID': idID };

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
    expectSameKeysAndParams(locales);
  });
});
