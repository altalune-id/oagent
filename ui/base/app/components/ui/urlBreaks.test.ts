import { describe, expect, it } from 'vitest';
import { urlBreaks } from './urlBreaks';

describe('urlBreaks', () => {
  it('splits after /p/ and /.well-known/ so a long URL wraps at path boundaries', () => {
    expect(urlBreaks('https://auth.example.com/p/acme')).toEqual(['https://auth.example.com/p/', 'acme']);
    expect(urlBreaks('https://h/p/acme/.well-known/openid-configuration')).toEqual(['https://h/p/', 'acme/.well-known/', 'openid-configuration']);
    expect(urlBreaks('https://h/.well-known/oauth-authorization-server/p/acme')).toEqual(['https://h/.well-known/', 'oauth-authorization-server/p/', 'acme']);
  });

  it('keeps a value without break points whole', () => {
    expect(urlBreaks('othra project keys rotate H9MspUj32ux8pfYKYsuKd --now')).toEqual(['othra project keys rotate H9MspUj32ux8pfYKYsuKd --now']);
    expect(urlBreaks('')).toEqual(['']);
  });
});
