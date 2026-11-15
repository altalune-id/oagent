import { describe, expect, it } from 'vitest';

import { WebToolError } from './errors.ts';
import { hopCheck, modelHeaders } from './policy.ts';

function refusal(check: (url: URL) => void, url: string): unknown {
  try {
    check(new URL(url));
  } catch (error) {
    return error;
  }
  return undefined;
}

describe('modelHeaders', () => {
  it('keeps only listed headers that are neither reserved nor refused to the model, in any case', () => {
    const allowed = new Set(['accept', 'cookie', 'authorization', 'x-trace', 'x-http-method-override', 'x-http-method', 'x-method-override']);
    expect(modelHeaders({ 'ACCEPT': 'a', 'Cookie': 'c', 'authorization': 'Bearer m', 'X-Trace': 't', 'X-Other': 'o', 'X-HTTP-Method-Override': 'DELETE', 'X-HTTP-Method': 'PATCH', 'X-Method-Override': 'PUT' }, allowed)).toEqual({ 'ACCEPT': 'a', 'X-Trace': 't' });
  });
});

describe('hopCheck', () => {
  const listed = hopCheck(['api.example.com']);
  const open = hopCheck([]);

  it('passes a listed https host on the default port', () => {
    expect(refusal(listed, 'https://api.example.com/v1?q=1')).toBeUndefined();
    expect(refusal(listed, 'https://api.example.com:443/')).toBeUndefined();
  });

  it('refuses http, another port and an unlisted host with blocked_host', () => {
    for (const url of ['http://api.example.com/', 'https://api.example.com:8443/', 'https://other.example.com/', 'https://sub.api.example.com/']) {
      const error = refusal(listed, url);
      expect(error, url).toBeInstanceOf(WebToolError);
      expect((error as WebToolError).class).toBe('blocked_host');
    }
  });

  it('names the host but never the path or query', () => {
    const error = refusal(listed, 'https://other.example.com/secret/path?token=abc') as WebToolError;
    expect(error.host).toBe('other.example.com');
    expect(error.message).not.toMatch(/secret|token|abc/);
  });

  it('lets an empty list reach any public host but no IP literal, single label or localhost', () => {
    expect(refusal(open, 'https://news.example.org/a')).toBeUndefined();
    for (const url of ['https://127.0.0.1/', 'https://[::1]/', 'https://intranet/', 'https://localhost/', 'https://0x7f.1/', 'http://news.example.org/']) {
      expect((refusal(open, url) as WebToolError | undefined)?.class, url).toBe('blocked_host');
    }
  });
});
