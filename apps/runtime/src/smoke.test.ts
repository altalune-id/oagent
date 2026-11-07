import { describe, expect, it } from 'vitest';

describe('runtime package', () => {
  it('loads the Strands SDK at the pinned version', async () => {
    const sdk = await import('@strands-agents/sdk');
    expect(typeof sdk.Agent).toBe('function');
  });
});
