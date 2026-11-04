import { describe, expect, it, vi } from 'vitest';
import { tick, useNow } from './useNow';

describe('useNow', () => {
  it('shares one clock that tick moves to the current time', () => {
    vi.useFakeTimers({ now: new Date(2026, 10, 4, 12, 0) });
    tick();
    expect(useNow().value).toEqual(new Date(2026, 10, 4, 12, 0));
    vi.setSystemTime(new Date(2026, 10, 4, 12, 1));
    tick();
    expect(useNow()).toBe(useNow());
    expect(useNow().value).toEqual(new Date(2026, 10, 4, 12, 1));
    vi.useRealTimers();
  });
});
