// @vitest-environment happy-dom
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { mountPage } from '#test/mount-page';
import { deferred, setClient } from '#test/nuxt-globals';
import WebhookList from '~/components/webhook/list/WebhookList.vue';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const retry = (w: Awaited<ReturnType<typeof mountPage>>) => w.findAll('button').find(b => b.text().includes('common.retry'))!;

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('webhooks list', () => {
  it('shows a loading status without blocking the page, then the empty state', async () => {
    const pending = deferred<{ webhooks: never[] }>();
    setClient('$webhookClient', { listWebhooks: vi.fn().mockReturnValue(pending.promise) });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId } });
    expect(w.get('[role="status"]').text()).toContain('common.loading');
    pending.resolve({ webhooks: [] });
    await flushPromises();
    expect(w.get('h2').text()).toBe('webhooks.empty');
  });

  it('offers Retry after a failed load, then shows the empty state', async () => {
    const listWebhooks = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ webhooks: [] });
    setClient('$webhookClient', { listWebhooks });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId } });
    await flushPromises();
    await retry(w).trigger('click');
    await flushPromises();
    expect(listWebhooks).toHaveBeenCalledTimes(2);
    expect(w.get('h2').text()).toBe('webhooks.empty');
  });
});
