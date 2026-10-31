import type { ShownError } from '#base/lib/apperror/message';

import { create } from '@bufbuild/protobuf';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useValidation } from '#base/composables/useValidation';
import { CreateWebhookRequestSchema } from '#gen/webhook/v1/webhook_pb';
import { toWebhook, useWebhooksStore } from '~/stores/webhooks';

export function useWebhookCreate(orgId: string, projectId: string) {
  const open = ref(false);
  const url = ref('');
  const description = ref('');
  const eventTypes = ref<string[]>([]);
  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const secret = ref('');
  const id = ref('');
  const pingBusy = ref(false);
  const pingError = ref<ShownError | null>(null);
  const pinged = ref(false);

  const webhooks = useWebhooksStore();
  const { $webhookClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const validation = useValidation(CreateWebhookRequestSchema, {
    fields: ['url', 'description', 'event_types'],
    codes: { WHK002: 'url', WHK003: 'event_types', WHK004: 'description' },
  });

  function show() {
    url.value = '';
    description.value = '';
    eventTypes.value = [];
    error.value = null;
    secret.value = '';
    id.value = '';
    pingError.value = null;
    pinged.value = false;
    validation.clear();
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    open.value = false;
    secret.value = '';
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { orgId, projectId, url: url.value.trim(), description: description.value, eventTypes: eventTypes.value };
    if (!validation.check(create(CreateWebhookRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createWebhook(req);
      if (res.webhook) {
        webhooks.add(toWebhook(res.webhook));
        id.value = res.webhook.id;
      }
      secret.value = res.secret;
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_webhook');
    } finally {
      busy.value = false;
    }
  }

  async function ping() {
    if (!id.value || pingBusy.value) {
      return;
    }
    pingBusy.value = true;
    pingError.value = null;
    try {
      await client.pingWebhook({ orgId, projectId, id: id.value });
      pinged.value = true;
    } catch (e) {
      pingError.value = errorMessage(e, 'error.ping_webhook');
    } finally {
      pingBusy.value = false;
    }
  }

  return { open, url, description, eventTypes, busy, error, fieldErrors: validation.errors, secret, pingBusy, pingError, pinged, show, hide, submit, ping };
}
