import process from 'node:process';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { handleShutdown } from './shutdown.ts';
import { captureLogger } from './test/logger.ts';

type Handler = (signal: NodeJS.Signals) => void;

let handlers: Map<string, Handler>;
let exit: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  handlers = new Map();
  vi.spyOn(process, 'on').mockImplementation(((event: string, fn: Handler) => {
    handlers.set(event, fn);
    return process;
  }) as typeof process.on);
  exit = vi.spyOn(process, 'exit').mockImplementation((() => undefined) as never);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('handleShutdown', () => {
  it('handles SIGTERM and SIGINT, closes once, then exits 0', async () => {
    const close = vi.fn(async () => {});
    handleShutdown(captureLogger(), 1_000, close);
    expect([...handlers.keys()]).toEqual(['SIGTERM', 'SIGINT']);
    handlers.get('SIGTERM')!('SIGTERM');
    await vi.waitFor(() => expect(exit).toHaveBeenCalledWith(0));
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('exits 1 when close fails', async () => {
    const lines: string[] = [];
    handleShutdown(captureLogger(lines), 1_000, async () => {
      throw new Error('boom');
    });
    handlers.get('SIGINT')!('SIGINT');
    await vi.waitFor(() => expect(exit).toHaveBeenCalledWith(1));
    expect(lines.join('\n')).toContain('"msg":"shutdown failed"');
  });

  it('exits 1 at once on a second signal', () => {
    handleShutdown(captureLogger(), 1_000, () => new Promise(() => {}));
    handlers.get('SIGTERM')!('SIGTERM');
    handlers.get('SIGTERM')!('SIGTERM');
    expect(exit).toHaveBeenCalledWith(1);
  });

  it('exits 1 when close outlives the grace period plus the margin', () => {
    vi.useFakeTimers();
    handleShutdown(captureLogger(), 1_000, () => new Promise(() => {}));
    handlers.get('SIGTERM')!('SIGTERM');
    vi.advanceTimersByTime(5_999);
    expect(exit).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(exit).toHaveBeenCalledWith(1);
  });
});
