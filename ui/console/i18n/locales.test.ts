import { describe, expect, it } from 'vitest';

import { expectSameKeysAndParams, paramsOf } from '#test/locales';
import baseEnUS from '../../base/i18n/locales/en-US.json';
import enUS from './locales/en-US.json';
import idID from './locales/id-ID.json';

const locales: Record<string, Record<string, string>> = { 'en-US': enUS, 'id-ID': idID };

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
});
