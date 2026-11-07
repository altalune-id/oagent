import type { LogOptions } from './log.ts';
import fs from 'node:fs';
import { describe, expect, it, vi } from 'vitest';

import { createLogger, strandsLogger } from './log.ts';
import { captureLogger } from './test/logger.ts';

const JSON_INFO: LogOptions = { logLevel: 'info', logFormat: 'json' };

function parsed(lines: string[]): Record<string, unknown>[] {
  return lines.map((l) => {
    const { time, ...rest } = JSON.parse(l) as Record<string, unknown>;
    expect(time).toMatch(/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/);
    return rest;
  });
}

describe('createLogger', () => {
  it('keeps only allow-listed fields, as one JSON line with an ISO time', () => {
    const lines: string[] = [];
    captureLogger(lines).info({ run: 'r1', apiKey: 'sk-test', headers: { a: 1 }, ms: 5 }, 'turn started');
    expect(parsed(lines)).toEqual([{ level: 'info', msg: 'turn started', run: 'r1', ms: 5 }]);
  });

  it('writes an error as its name only, and caps long strings', () => {
    const lines: string[] = [];
    captureLogger(lines).error({ error: new TypeError('key sk-test'), code: 'x'.repeat(300) }, 'turn failed');
    const [line] = parsed(lines);
    expect(line!.error).toBe('TypeError');
    expect(line!.code).toHaveLength(200);
    expect(lines[0]).not.toContain('sk-test');
  });

  it('writes a thrown non-Error by its type only', () => {
    const lines: string[] = [];
    const log = captureLogger(lines);
    log.error({ error: 'sk-test leaked' }, 'turn threw');
    log.error({ error: { key: 'sk-test' } }, 'turn threw');
    expect(parsed(lines)).toEqual([
      { level: 'error', msg: 'turn threw', error: 'string' },
      { level: 'error', msg: 'turn threw', error: 'object' },
    ]);
  });

  it('keeps arrays of strings and drops other shapes', () => {
    const lines: string[] = [];
    captureLogger(lines).warn({ vars: ['HTTP_PROXY'], hosts: [{ h: 1 }], fields: { x: 1 } }, 'proxy env ignored');
    expect(parsed(lines)).toEqual([{ level: 'warn', msg: 'proxy env ignored', vars: ['HTTP_PROXY'] }]);
  });

  it('filters child and grandchild bindings through the same allow-list', () => {
    const lines: string[] = [];
    const child = captureLogger(lines).child({ run: 'r1', apiKey: 'sk-test' });
    child.info('turn started');
    child.child({ server: 'yasaku', token: 'tok-test' }, { level: 'info' }).info('mcp server log');
    expect(parsed(lines)).toEqual([
      { level: 'info', msg: 'turn started', run: 'r1' },
      { level: 'info', msg: 'mcp server log', run: 'r1', server: 'yasaku' },
    ]);
  });

  it('drops lines below the configured level', () => {
    const lines: string[] = [];
    const log = captureLogger(lines, { logLevel: 'warn', logFormat: 'json' });
    log.info('hidden');
    log.warn('shown');
    expect(parsed(lines).map(l => l.msg)).toEqual(['shown']);
  });

  it('writes text as one line per entry, still filtered', () => {
    const lines: string[] = [];
    captureLogger(lines, { logLevel: 'info', logFormat: 'text' }).error({ run: 'r1', apiKey: 'sk-test', error: new Error('sk-test') }, 'turn started');
    expect(lines).toHaveLength(1);
    expect(lines[0]).toMatch(/ERROR.*turn started.*"run":"r1".*"error":"Error"/);
    expect(lines[0]).not.toContain('sk-test');
  });

  it('writes to stdout by default', () => {
    const spy = vi.spyOn(fs, 'writeSync').mockImplementation((_fd, data) => String(data).length);
    createLogger(JSON_INFO).info('hello');
    expect(spy.mock.calls.some(([fd, data]) => fd === 1 && String(data).includes('"msg":"hello"'))).toBe(true);
    spy.mockRestore();
  });
});

describe('strandsLogger', () => {
  it('logs fixed messages only, debug and info at debug level', () => {
    const lines: string[] = [];
    const s = strandsLogger(captureLogger(lines, { logLevel: 'debug', logFormat: 'json' }));
    s.debug('x', 'sk-test');
    s.info('x', 'sk-test');
    s.warn('client=<y>, error=<Error POSTing: Bearer sk-test>');
    s.error(new Error('sk-test'));
    expect(parsed(lines)).toEqual([
      { level: 'debug', msg: 'strands debug' },
      { level: 'debug', msg: 'strands info' },
      { level: 'warn', msg: 'strands warning' },
      { level: 'error', msg: 'strands error' },
    ]);
  });

  it('hides strands debug and info at the default level', () => {
    const lines: string[] = [];
    const s = strandsLogger(captureLogger(lines));
    s.debug('x');
    s.info('x');
    expect(lines).toEqual([]);
  });
});
