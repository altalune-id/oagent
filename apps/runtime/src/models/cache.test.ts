import { afterEach, describe, expect, it, vi } from 'vitest';

import { ClientCache } from './cache.ts';

type Client = {
  id: number,
};

function counter(): () => Client {
  let n = 0;
  return () => ({ id: ++n });
}

afterEach(() => {
  vi.useRealTimers();
});

describe('clientCache', () => {
  it('reuses the client for the same key and API key', () => {
    const cache = new ClientCache<Client>();
    const make = counter();
    const a = cache.get('k', 'sk', make);
    expect(cache.get('k', 'sk', make)).toBe(a);
    expect(cache.size).toBe(1);
  });

  it('gives another key its own client', () => {
    const cache = new ClientCache<Client>();
    const make = counter();
    expect(cache.get('a', 'sk', make)).not.toBe(cache.get('b', 'sk', make));
    expect(cache.size).toBe(2);
  });

  it('replaces the client when the API key changes for the same key', () => {
    const cache = new ClientCache<Client>();
    const make = counter();
    const old = cache.get('k', 'sk', make);
    const fresh = cache.get('k', 'sk-rotated', make);
    expect(fresh).not.toBe(old);
    expect(cache.size).toBe(1);
    expect(cache.get('k', 'sk-rotated', make)).toBe(fresh);
    expect(cache.get('k', 'sk', make)).not.toBe(old);
  });

  it('evicts the least recently used past the bound', () => {
    const cache = new ClientCache<Client>({ max: 2 });
    const make = counter();
    const a = cache.get('a', 'sk', make);
    const b = cache.get('b', 'sk', make);
    expect(cache.get('a', 'sk', make)).toBe(a);
    cache.get('c', 'sk', make);
    expect(cache.size).toBe(2);
    expect(cache.get('a', 'sk', make)).toBe(a);
    expect(cache.get('b', 'sk', make)).not.toBe(b);
  });

  it('drops a client idle for longer than the expiry', () => {
    vi.useFakeTimers();
    const cache = new ClientCache<Client>({ idleMs: 15 * 60_000 });
    const make = counter();
    const a = cache.get('p1', 'sk', make);
    vi.advanceTimersByTime(15 * 60_000);
    expect(cache.get('p1', 'sk', make)).toBe(a);
    vi.advanceTimersByTime(15 * 60_000 + 1);
    expect(cache.get('p1', 'sk', make)).not.toBe(a);
    cache.get('p2', 'sk', make);
    vi.advanceTimersByTime(15 * 60_000 + 1);
    cache.get('p3', 'sk', make);
    expect(cache.keys()).toEqual(['p3']);
  });

  it('defaults to 256 entries and a 15 minute expiry', () => {
    const cache = new ClientCache<Client>();
    expect(cache.max).toBe(256);
    expect(cache.idleMs).toBe(15 * 60_000);
  });
});
