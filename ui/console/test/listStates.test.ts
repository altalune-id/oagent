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

describe('webhooks table', () => {
  const hook = { id: 'H3nVb8XqT1wLp6RkZ2mJc', projectId, url: 'https://hooks.acme.test/oagent', active: true, eventTypes: [] };

  it('shows a mono url, a status dot with a word, and every action in a named row menu', async () => {
    setClient('$webhookClient', { listWebhooks: vi.fn().mockResolvedValue({ webhooks: [hook] }) });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId, canManage: true } });
    await flushPromises();
    const cells = w.findAll('tbody td');
    expect(cells[0]!.classes()).toEqual(['font-mono', 'text-xs']);
    expect(cells[0]!.get('span').attributes('title')).toBe(hook.url);
    expect(cells[2]!.text()).toBe('webhooks.active');
    expect(w.get('tbody button[popovertarget] .sr-only').text()).toBe(`webhooks.actions_for|{"url":"${hook.url}"}`);
    expect(w.findAll('tbody [popover] li').map(li => li.text())).toEqual(['webhooks.logs', 'webhooks.ping', 'webhooks.deactivate', 'webhooks.rotate', 'common.delete']);
    expect(w.get('tbody [popover] [data-icon="lucide:trash-2"]').classes()).toContain('text-error');
  });

  it('offers only Logs in the menu without webhook:write', async () => {
    setClient('$webhookClient', { listWebhooks: vi.fn().mockResolvedValue({ webhooks: [{ ...hook, active: false }] }) });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId } });
    await flushPromises();
    expect(w.findAll('tbody td')[2]!.text()).toBe('webhooks.inactive');
    expect(w.findAll('tbody [popover] li').map(li => li.text())).toEqual(['webhooks.logs']);
  });
});
