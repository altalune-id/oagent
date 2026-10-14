import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebhooksStore } from '~/stores/webhooks';
import { WebhookSchema } from '../../../../gen/webhook/v1/webhook_pb';
import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useWebhookCreate } from './useWebhookCreate';

const hookPB = create(WebhookSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', url: 'https://a.test', eventTypes: ['agent.created'], active: true });

describe('useWebhookCreate', () => {
  let createWebhook: ReturnType<typeof vi.fn>;
  let pingWebhook: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    createWebhook = vi.fn().mockResolvedValue({ webhook: hookPB, secret: 'whsec_1' });
    pingWebhook = vi.fn().mockResolvedValue({});
    setClient('$webhookClient', { createWebhook, pingWebhook });
  });

  function filled() {
    const form = useWebhookCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.url.value = ' https://a.test ';
    form.description.value = 'd';
    form.eventTypes.value = ['agent.created'];
    return form;
  }

  it('resets the form when shown', () => {
    const form = useWebhookCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.url.value = 'x';
    form.secret.value = 's';
    form.pinged.value = true;
    form.show();
    expect(form.open.value).toBe(true);
    expect([form.url.value, form.description.value, form.eventTypes.value, form.secret.value, form.pinged.value, form.error.value, form.pingError.value])
      .toEqual(['', '', [], '', false, null, null]);
  });

  it('creates a webhook, adds it and shows the secret', async () => {
    const form = filled();
    await form.submit();
    expect(createWebhook).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', url: 'https://a.test', description: 'd', eventTypes: ['agent.created'] });
    expect(useWebhooksStore().items.map(w => w.id)).toEqual(['UxVy4UK3vmrHzPZ273hCA']);
    expect(form.secret.value).toBe('whsec_1');
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.secret.value).toBe('');
  });

  it('validates the url and the events', async () => {
    const form = filled();
    form.url.value = '  ';
    await form.submit();
    form.url.value = 'https://a.test';
    form.eventTypes.value = [];
    await form.submit();
    expect(createWebhook).not.toHaveBeenCalled();
  });

  it('does not submit twice while busy, and cannot be hidden while busy', async () => {
    const pending = deferred<object>();
    createWebhook.mockReturnValue(pending.promise);
    const form = filled();
    const first = form.submit();
    await form.submit();
    form.hide();
    expect(form.open.value).toBe(true);
    pending.resolve({ secret: 'whsec_2' });
    await first;
    expect(createWebhook).toHaveBeenCalledTimes(1);
    expect(useWebhooksStore().items).toEqual([]);
    expect(form.secret.value).toBe('whsec_2');
  });

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.WHK005');
    createWebhook.mockRejectedValueOnce(connectError('WHK005', { limit: '3' })).mockRejectedValueOnce(new Error('boom'));
    const form = filled();
    await form.submit();
    expect(form.error.value).toEqual({ message: 'apperror.WHK005|{"limit":"3"}', reference: 'error.ref|{"id":"req-1"}' });
    await form.submit();
    expect(form.error.value).toEqual({ message: 'error.create_webhook' });
  });

  it('shows a URL the server refuses under the URL input', async () => {
    setKnownKeys('apperror.WHK002_https_required');
    createWebhook.mockRejectedValueOnce(connectError('WHK002', { reason: 'https_required' }));
    const form = filled();
    await form.submit();
    expect(form.fieldErrors.value).toEqual({ url: 'apperror.WHK002_https_required' });
    expect(form.error.value).toBeNull();
  });

  it('checks the URL and events before sending', async () => {
    const form = filled();
    form.url.value = '/hook';
    form.eventTypes.value = [];
    await form.submit();
    expect(createWebhook).not.toHaveBeenCalled();
    expect(form.fieldErrors.value).toEqual({ url: 'validation.invalid_format', event_types: 'validation.required' });
  });

  it('pings the created webhook', async () => {
    const form = filled();
    await form.ping();
    expect(pingWebhook).not.toHaveBeenCalled();
    await form.submit();
    await form.ping();
    expect(pingWebhook).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', id: 'UxVy4UK3vmrHzPZ273hCA' });
    expect(form.pinged.value).toBe(true);
    expect(form.pingBusy.value).toBe(false);
  });

  it('does not ping twice at once', async () => {
    const pending = deferred<object>();
    pingWebhook.mockReturnValue(pending.promise);
    const form = filled();
    await form.submit();
    const first = form.ping();
    await form.ping();
    pending.resolve({});
    await first;
    expect(pingWebhook).toHaveBeenCalledTimes(1);
  });

  it('shows ping errors', async () => {
    setKnownKeys('apperror.WHK010');
    pingWebhook.mockRejectedValueOnce(connectError('WHK010')).mockRejectedValueOnce(new Error('boom'));
    const form = filled();
    await form.submit();
    await form.ping();
    expect(form.pingError.value).toEqual({ message: 'apperror.WHK010', reference: 'error.ref|{"id":"req-1"}' });
    expect(form.pinged.value).toBe(false);
    await form.ping();
    expect(form.pingError.value).toEqual({ message: 'error.ping_webhook' });
  });
});
