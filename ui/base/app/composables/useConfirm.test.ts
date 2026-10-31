import { describe, expect, it, vi } from 'vitest';

import { connectError, deferred, setKnownKeys } from '../../test/nuxt-globals';
import { useConfirm } from './useConfirm';

describe('useConfirm', () => {
  it('runs only after an ask, keeps the confirm open on a refusal, and closes on success', async () => {
    setKnownKeys('apperror.GEN900');
    const run = vi.fn().mockRejectedValueOnce(connectError('GEN900')).mockResolvedValueOnce(undefined);
    const c = useConfirm<string>(run, 'error.x');
    expect(await c.confirm()).toBe(false);
    c.ask('a');
    expect(await c.confirm()).toBe(false);
    expect(c.error.value?.message).toBe('apperror.GEN900');
    expect(c.target.value).toBe('a');
    expect(await c.confirm()).toBe(true);
    expect(run).toHaveBeenLastCalledWith('a');
    expect(c.target.value).toBeNull();
    c.ask('b');
    c.cancel();
    expect(c.target.value).toBeNull();
  });

  it('ignores a second confirm and a cancel while one runs', async () => {
    const pending = deferred<void>();
    const run = vi.fn().mockReturnValue(pending.promise);
    const c = useConfirm<string>(run, 'error.x');
    c.ask('a');
    const first = c.confirm();
    expect(await c.confirm()).toBe(false);
    c.cancel();
    expect(c.target.value).toBe('a');
    pending.resolve();
    expect(await first).toBe(true);
    expect(run).toHaveBeenCalledTimes(1);
  });
});
