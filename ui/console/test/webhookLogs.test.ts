// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { deferred, setClient, setRoute } from '#test/nuxt-globals';
import Logs from '~/pages/o/[org]/projects/[project]/webhooks/[webhook].vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const delivery = { id: 'D1', eventType: 'agent.created', attempt: 2, status: 'failed', lastError: 'boom', payload: '{"a":1}' };

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }] as never;
  setRoute({ path: '/o/acme/projects/web/webhooks/H1', params: { org: 'acme', project: 'web', webhook: 'H1' } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions: ['webhook:read', 'webhook:write'] })) });
});

describe('webhook logs', () => {
  it('heads the page, shows loading, then lists deliveries with a status dot and an Attempts button', async () => {
    const pending = deferred<{ deliveries: typeof delivery[] }>();
    const listWebhookDeliveryAttempts = vi.fn().mockResolvedValue({ attempts: [{ attempt: 1, statusCode: 0, error: 'no such host', durationMs: 40, responseBody: '', responseTruncated: false, responseHeaders: [] }] });
    setClient('$webhookClient', { listWebhookDeliveries: vi.fn().mockReturnValue(pending.promise), listWebhookDeliveryAttempts });
    const w = await mountPage(Logs, { attachTo: document.body });
    expect(w.get('h1').text()).toBe('webhooks.deliveries');
    expect(w.get('[role="status"]').text()).toContain('common.loading');
    pending.resolve({ deliveries: [delivery] });
    await flushPromises();
    const cells = w.findAll('tbody td');
    expect(cells[0]!.classes()).toEqual(['font-mono', 'text-xs']);
    expect(cells[1]!.text()).toBe('webhooks.status_failed');
    expect(cells[1]!.find('.badge').exists()).toBe(false);
    await cells[4]!.get('button').trigger('click');
    await flushPromises();
    expect(listWebhookDeliveryAttempts).toHaveBeenCalled();
    const dialog = [...document.querySelectorAll('dialog')].find(d => d.open)!;
    expect(dialog.textContent).toContain('no such host');
    expect(dialog.textContent).toContain('webhooks.no_response');
    expect(dialog.textContent).not.toMatch(/(^|\s)0(\s|$)/);
    expect([...dialog.querySelectorAll('button')].map(b => b.textContent!.trim())).toContain('webhooks.retry');
    w.unmount();
  });

  it('offers Retry after a failed load, then shows the empty state', async () => {
    const listWebhookDeliveries = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ deliveries: [] });
    setClient('$webhookClient', { listWebhookDeliveries });
    const w = await mountPage(Logs);
    await flushPromises();
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(listWebhookDeliveries).toHaveBeenCalledTimes(2);
    expect(w.get('h2').text()).toBe('webhooks.no_deliveries');
  });
});
