import type { Webhook } from './webhooks';
import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WebhookSchema } from '../../gen/webhook/v1/webhook_pb';
import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { toWebhook, useWebhooksStore } from './webhooks';

const hookPB = create(WebhookSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', url: 'https://a.test', description: 'd', eventTypes: ['agent.created'], active: true });
const hook: Webhook = { id: 'UxVy4UK3vmrHzPZ273hCA', orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', url: 'https://a.test', description: 'd', eventTypes: ['agent.created'], active: true };

describe('toWebhook', () => {
  it('copies the fields it needs', () => {
    expect(toWebhook(hookPB)).toEqual(hook);
  });
});

describe('useWebhooksStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the webhooks of a project', async () => {
    const listWebhooks = vi.fn().mockResolvedValue({ webhooks: [hookPB] });
    setClient('$webhookClient', { listWebhooks });
    const webhooks = useWebhooksStore();
    await webhooks.load('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(listWebhooks).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF' });
    expect(webhooks.items).toEqual([hook]);
    expect(webhooks.forProject('x6MH9WPWjktMwCBKHy4pF')).toEqual([hook]);
    expect(webhooks.forProject('KdrF3t3FHtppBPmMgj4sy')).toEqual([]);
  });

  it('treats a missing list as empty', async () => {
    setClient('$webhookClient', { listWebhooks: vi.fn().mockResolvedValue({}) });
    const webhooks = useWebhooksStore();
    await webhooks.load('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(webhooks.items).toEqual([]);
  });

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900');
    setClient('$webhookClient', { listWebhooks: vi.fn().mockRejectedValue(connectError('GEN900')) });
    const webhooks = useWebhooksStore();
    await webhooks.load('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(webhooks.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
  });

  it('shows the fallback for a plain error', async () => {
    setClient('$webhookClient', { listWebhooks: vi.fn().mockRejectedValue(new Error('boom')) });
    const webhooks = useWebhooksStore();
    await webhooks.load('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(webhooks.error).toEqual({ message: 'error.load_webhooks' });
    expect(webhooks.loading).toBe(false);
  });

  it('shares an in-flight load and skips a pair already loaded or incomplete', async () => {
    const pending = deferred<{ webhooks: typeof hookPB[] }>();
    const listWebhooks = vi.fn(() => pending.promise);
    setClient('$webhookClient', { listWebhooks });
    const webhooks = useWebhooksStore();
    await webhooks.ensureLoaded('', 'x6MH9WPWjktMwCBKHy4pF');
    await webhooks.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a', '');
    const first = webhooks.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    const second = webhooks.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(listWebhooks).toHaveBeenCalledTimes(1);
    pending.resolve({ webhooks: [hookPB] });
    await Promise.all([first, second]);
    await webhooks.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    expect(listWebhooks).toHaveBeenCalledTimes(1);
  });

  it('adds, replaces and removes webhooks', () => {
    setClient('$webhookClient', {});
    const webhooks = useWebhooksStore();
    const second = { ...hook, id: 'a6SkMeA9Zj2rjRmCXx2dM' };
    webhooks.add(hook);
    webhooks.add(hook);
    webhooks.add(second);
    expect(webhooks.items).toHaveLength(2);
    webhooks.replace({ ...hook, active: false });
    expect(webhooks.items).toEqual([{ ...hook, active: false }, second]);
    webhooks.remove('UxVy4UK3vmrHzPZ273hCA');
    expect(webhooks.items).toEqual([second]);
  });
});
