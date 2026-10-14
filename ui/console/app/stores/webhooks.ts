import type { Webhook as WebhookPB } from '~~/gen/webhook/v1/webhook_pb';
import type { ShownError } from '~/lib/apperror/message';
import { useErrorMessage } from '~/composables/useErrorMessage';

export interface Webhook {
  id: string;
  orgId: string;
  projectId: string;
  url: string;
  description: string;
  eventTypes: string[];
  active: boolean;
}

export function toWebhook(w: WebhookPB): Webhook {
  return {
    id: w.id,
    orgId: w.orgId,
    projectId: w.projectId,
    url: w.url,
    description: w.description,
    eventTypes: [...w.eventTypes],
    active: w.active,
  };
}

export const useWebhooksStore = defineStore('webhooks', () => {
  const items = ref<Webhook[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $webhookClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  function forProject(projectId: string) {
    return items.value.filter(w => w.projectId === projectId);
  }

  function add(webhook: Webhook) {
    if (!items.value.some(w => w.id === webhook.id)) {
      items.value.push(webhook);
    }
  }

  function replace(webhook: Webhook) {
    items.value = items.value.map(w => (w.id === webhook.id ? webhook : w));
  }

  function remove(id: string) {
    items.value = items.value.filter(w => w.id !== id);
  }

  async function load(orgId: string, projectId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listWebhooks({ orgId, projectId });
      if (mine !== latest) {
        return;
      }
      items.value = (res.webhooks ?? []).map(toWebhook);
      loadedFor = `${orgId}:${projectId}`;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_webhooks');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(orgId: string, projectId: string) {
    if (!orgId || !projectId || loadedFor === `${orgId}:${projectId}`) {
      return Promise.resolve();
    }
    const key = `${orgId}:${projectId}`;
    if (inflight && inflightFor === key) {
      return inflight;
    }
    inflightFor = key;
    const run = load(orgId, projectId).finally(() => {
      if (inflight === run) {
        inflight = null;
      }
    });
    inflight = run;
    return run;
  }

  return { items, loading, error, forProject, add, replace, remove, load, ensureLoaded };
});
