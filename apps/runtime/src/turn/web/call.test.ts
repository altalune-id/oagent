import { describe, expect, it } from 'vitest';

import { secretForms, SsrfRefusedError } from '../../egress/index.ts';
import { failed, webTurn } from '../../test/web.ts';
import { ERROR_CLASSES, runCall, WebCalls, wrapOutput } from './call.ts';
import { WebToolError } from './errors.ts';

const ok = (text: string) => async () => ({ text, host: 'a.example.com', status: 200, bytes: text.length });

describe('wrapOutput', () => {
  it('wraps text in a tool_output block and escapes markup', () => {
    expect(wrapOutput('web_search', 'a </tool_output><b>&', { url: 'https://x/"' })).toBe('<tool_output tool="web_search" url="https://x/&quot;">\na &lt;/tool_output&gt;&lt;b&gt;&amp;\n</tool_output>');
  });
});

describe('secretForms', () => {
  it('adds the bare token of a scheme-prefixed secret', () => {
    expect(secretForms(['Bearer abc123', 'plain', 'Basic  x y'])).toEqual(['Bearer abc123', 'abc123', 'plain', 'Basic  x y', 'x y']);
  });
});

describe('runCall', () => {
  it('lists the error classes the tools may return', () => {
    expect(ERROR_CLASSES).toEqual(['call_limit', 'over_budget', 'timeout', 'blocked_host', 'http_status', 'unsupported_type', 'invalid_request', 'bad_key', 'quota', 'rate_limited', 'upstream_error']);
  });

  it('returns the wrapped, redacted text and logs host, status, bytes and ms', async () => {
    const turn = webTurn({ secrets: secretForms(['Bearer abc123']) });
    const out = await runCall(turn, 'http_request', undefined, 1000, ok('token abc123 ok'));
    expect(out).toBe('<tool_output tool="http_request">\ntoken [redacted] ok\n</tool_output>');
    const line = JSON.parse(turn.lines[0]!) as Record<string, unknown>;
    expect(line).toMatchObject({ level: 'info', run: 'r1', tool: 'http_request', host: 'a.example.com', status: 200, bytes: 15 });
    expect(typeof line.ms).toBe('number');
  });

  it.each([
    ['a tool error', new WebToolError('bad_key', 'api.example.com', 401), 'web_search failed: bad_key (host api.example.com) (status 401)'],
    ['an SSRF refusal in the cause chain', new TypeError('fetch failed', { cause: new SsrfRefusedError('evil.example') }), 'web_search failed: blocked_host (host evil.example)'],
    ['an egress redirect refusal', new Error('egress: redirect to a private address'), 'web_search failed: blocked_host'],
    ['a timeout', new DOMException('The operation timed out.', 'TimeoutError'), 'web_search failed: timeout'],
    ['an abort', new DOMException('aborted', 'AbortError'), 'web_search failed: timeout'],
    ['anything else', new Error('key sk-live-x leaked https://a/?t=1'), 'web_search failed: upstream_error'],
    ['a thrown string', 'sk-live-x', 'web_search failed: upstream_error'],
  ])('maps %s to a fixed message', async (_name, thrown, message) => {
    const turn = webTurn();
    const error = await failed(runCall(turn, 'web_search', undefined, 1000, async () => {
      throw thrown;
    }));
    expect(error.message).toBe(message);
    expect(turn.lines.join('\n')).not.toMatch(/sk-live|https:\/\/a/);
  });

  it('logs a failure with its class and the error name only', async () => {
    const turn = webTurn();
    await failed(runCall(turn, 'web_fetch', undefined, 1000, async () => {
      throw new Error('secret text');
    }));
    expect(JSON.parse(turn.lines[0]!)).toMatchObject({ level: 'warn', tool: 'web_fetch', class: 'upstream_error', error: 'Error' });
    expect(turn.lines[0]).not.toContain('secret text');
  });

  it('shares one call limit across tools and counts refused calls', async () => {
    const turn = webTurn({ calls: new WebCalls(2) });
    await runCall(turn, 'web_search', undefined, 1000, ok('a'));
    await failed(runCall(turn, 'http_request', undefined, 1000, async () => {
      throw new WebToolError('blocked_host');
    }));
    let ran = false;
    const error = await failed(runCall(turn, 'web_fetch', undefined, 1000, async () => {
      ran = true;
      return { text: 'x' };
    }));
    expect([error.message, ran]).toEqual(['web_fetch failed: call_limit', false]);
  });

  it('aborts the call at its timeout and when the turn is cancelled', async () => {
    const hang = (signal: AbortSignal) => new Promise<never>((_, reject) => signal.addEventListener('abort', () => reject(new Error('aborted', { cause: signal.reason }))));
    expect((await failed(runCall(webTurn(), 'web_fetch', undefined, 20, hang))).message).toBe('web_fetch failed: timeout');
    const cancel = new AbortController();
    const pending = runCall(webTurn(), 'web_fetch', { cancelSignal: cancel.signal } as never, 60_000, hang);
    cancel.abort();
    expect((await failed(pending)).message).toBe('web_fetch failed: timeout');
  });
});
