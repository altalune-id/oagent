import type { APIKey as APIKeyPB } from '~~/gen/apikey/v1/apikey_pb';
import type { ShownError } from '~/lib/apperror/message';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '~/composables/useErrorMessage';

export interface APIKey {
  rowKey: string;
  kind: string;
  name: string;
  scopes: string[];
  projectId: string;
  projectIds: string[];
  allProjects: boolean;
  secretHint: string;
  expiresAt?: Date;
  revokedAt?: Date;
}

let rows = 0;

export function toAPIKey(k: APIKeyPB): APIKey {
  return {
    rowKey: `key-${++rows}`,
    kind: k.kind,
    name: k.name,
    scopes: [...k.scopes],
    projectId: k.projectId,
    projectIds: [...k.projectIds],
    allProjects: k.allProjects,
    secretHint: k.secretHint,
    expiresAt: k.expiresAt ? timestampDate(k.expiresAt) : undefined,
    revokedAt: k.revokedAt ? timestampDate(k.revokedAt) : undefined,
  };
}

export const useAPIKeysStore = defineStore('apiKeys', () => {
  const items = ref<APIKey[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $apiKeyClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  function isLive(k: APIKey, hint: string) {
    return !k.revokedAt && k.secretHint === hint;
  }

  function add(key: APIKey) {
    items.value = [key, ...items.value.filter(k => !isLive(k, key.secretHint))];
  }

  function markRevoked(hint: string) {
    const at = new Date();
    items.value = items.value.map(k => (isLive(k, hint) ? { ...k, revokedAt: at } : k));
  }

  async function load(orgId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listAPIKeys({ orgId });
      if (mine !== latest) {
        return;
      }
      items.value = (res.keys ?? []).map(toAPIKey);
      loadedFor = orgId;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_apikeys');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(orgId: string) {
    if (!orgId || loadedFor === orgId) {
      return Promise.resolve();
    }
    if (inflight && inflightFor === orgId) {
      return inflight;
    }
    inflightFor = orgId;
    const run = load(orgId).finally(() => {
      if (inflight === run) {
        inflight = null;
      }
    });
    inflight = run;
    return run;
  }

  return { items, loading, error, add, markRevoked, load, ensureLoaded };
});
