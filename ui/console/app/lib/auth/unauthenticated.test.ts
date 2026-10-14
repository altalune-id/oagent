import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';

import { createRedirectOnce, isUnauthenticated } from './unauthenticated';

describe('isUnauthenticated', () => {
  it('is true only for a Connect 401', () => {
    expect(isUnauthenticated(new ConnectError('no', Code.Unauthenticated))).toBe(true);
    expect(isUnauthenticated(new ConnectError('no', Code.PermissionDenied))).toBe(false);
    expect(isUnauthenticated(new Error('boom'))).toBe(false);
    expect(isUnauthenticated(undefined)).toBe(false);
  });
});

describe('createRedirectOnce', () => {
  it('redirects once for many concurrent 401s', async () => {
    const go = vi.fn();
    const redirect = createRedirectOnce(go);
    await Promise.all([1, 2, 3].map(async n => redirect(`/login?n=${n}`)));
    expect(go).toHaveBeenCalledTimes(1);
    expect(go).toHaveBeenCalledWith('/login?n=1');
  });
});
