import { describe, expect, it, vi } from 'vitest';

import { createLogger, strandsLogger } from './log.ts';

describe('createLogger', () => {
  it('keeps only allow-listed fields, as one JSON line', () => {
    const lines: string[] = [];
    const log = createLogger(line => lines.push(line));
    log.info('turn started', { run: 'r1', apiKey: 'sk-test', headers: { a: 1 }, ms: 5 });
    expect(lines).toEqual(['{"level":"info","msg":"turn started","run":"r1","ms":5}']);
  });

  it('writes an error as its name only, and caps long strings', () => {
    const lines: string[] = [];
    const log = createLogger(line => lines.push(line));
    log.error('turn failed', { error: new TypeError('key sk-test'), code: 'x'.repeat(300) });
    const parsed = JSON.parse(lines[0]!) as Record<string, string>;
    expect(parsed.error).toBe('TypeError');
    expect(parsed.code).toHaveLength(200);
  });

  it('writes a thrown non-Error by its type only', () => {
    const lines: string[] = [];
    const log = createLogger(line => lines.push(line));
    log.error('turn threw', { error: 'sk-test leaked' });
    log.error('turn threw', { error: { key: 'sk-test' } });
    expect(lines).toEqual([
      '{"level":"error","msg":"turn threw","error":"string"}',
      '{"level":"error","msg":"turn threw","error":"object"}',
    ]);
  });

  it('keeps arrays of strings and drops other shapes', () => {
    const lines: string[] = [];
    const log = createLogger(line => lines.push(line));
    log.warn('proxy env ignored', { vars: ['HTTP_PROXY'], hosts: [{ h: 1 }], fields: { x: 1 } });
    expect(lines).toEqual(['{"level":"warn","msg":"proxy env ignored","vars":["HTTP_PROXY"]}']);
  });

  it('writes to stdout by default', () => {
    const spy = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);
    createLogger().info('hello');
    expect(spy).toHaveBeenCalledWith('{"level":"info","msg":"hello"}\n');
    spy.mockRestore();
  });
});

describe('strandsLogger', () => {
  it('drops debug and info, and logs warn and error with no free text', () => {
    const lines: string[] = [];
    const s = strandsLogger(createLogger(line => lines.push(line)));
    s.debug('x', 'sk-test');
    s.info('x', 'sk-test');
    s.warn('client=<y>, error=<Error POSTing: Bearer sk-test>');
    s.error(new Error('sk-test'));
    expect(lines).toEqual(['{"level":"warn","msg":"strands warning"}', '{"level":"error","msg":"strands error"}']);
  });
});
