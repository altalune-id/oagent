import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { setClient } from '#test/nuxt-globals';
import { useAuthStore } from '~/stores/auth';
import { can, useCan } from './useCan';

describe('can', () => {
  it('follows the signed-in person\'s permissions, operators and wildcards', () => {
    expect(can({ permissions: ['*'] }, 'org:write')).toBe(true);
    expect(can({ permissions: ['apikey:*'] }, 'apikey:write')).toBe(true);
    expect(can({ operator: true }, 'org:write')).toBe(true);
    expect(can({ permissions: ['apikey:read', 'org:read'] }, 'apikey:write')).toBe(false);
    expect(can({}, 'member:write')).toBe(false);
  });

  it('leaves the decision to the server when nobody is signed in', () => {
    expect(can(null, 'org:write')).toBe(true);
  });
});

describe('useCan', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('follows the auth store', async () => {
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions: ['org:read'] }) });
    const canWrite = useCan('org:write');
    expect(canWrite.value).toBe(true);
    await useAuthStore().load();
    expect(canWrite.value).toBe(false);
  });
});
