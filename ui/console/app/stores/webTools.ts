import type { ShownError } from '#base/lib/apperror/message';
import type { HTTPSecret as HTTPSecretPB, WebSearchProviderKind, WebSearchProvider as WebSearchProviderPB } from '#gen/webtool/v1/webtool_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';

export interface WebSearchProvider {
  kind: WebSearchProviderKind;
  keySet: boolean;
  pricePerCallMicros: bigint;
  updatedAt?: Date;
}

export interface HttpSecret {
  id: string;
  host: string;
  header: string;
  updatedAt?: Date;
}

export interface UsedByAgent {
  id: string;
  name: string;
}

function toProvider(p: WebSearchProviderPB): WebSearchProvider {
  return { kind: p.kind, keySet: p.keySet, pricePerCallMicros: p.pricePerCallMicros, updatedAt: p.updatedAt ? timestampDate(p.updatedAt) : undefined };
}

function toSecret(s: HTTPSecretPB): HttpSecret {
  return { id: s.id, host: s.host, header: s.header, updatedAt: s.updatedAt ? timestampDate(s.updatedAt) : undefined };
}

export const useWebToolsStore = defineStore('webTools', () => {
  const provider = ref<WebSearchProvider | null>(null);
  const availableKinds = ref<WebSearchProviderKind[]>([]);
  const usedBy = ref<UsedByAgent[]>([]);
  const secrets = ref<HttpSecret[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref<ShownError | null>(null);

  const { $webToolClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  const usableProvider = computed(() => (provider.value && availableKinds.value.includes(provider.value.kind) ? provider.value : null));
  const searchReady = computed<boolean | undefined>(() => (loaded.value && !error.value ? !!usableProvider.value : undefined));

  async function load(projectId: string) {
    const mine = ++latest;
    loadedFor = '';
    loading.value = true;
    loaded.value = false;
    error.value = null;
    try {
      const [res, list] = await Promise.all([client.getWebSearchProvider({ projectId }), client.listHTTPSecrets({ projectId })]);
      if (mine !== latest) {
        return;
      }
      provider.value = res.provider ? toProvider(res.provider) : null;
      availableKinds.value = [...(res.availableKinds ?? [])];
      usedBy.value = (res.usedBy ?? []).map(u => ({ id: u.id, name: u.name }));
      secrets.value = (list.secrets ?? []).map(toSecret);
      loaded.value = true;
      loadedFor = projectId;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_web_tools');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(projectId: string) {
    if (!projectId || loadedFor === projectId) {
      return Promise.resolve();
    }
    if (inflight && inflightFor === projectId) {
      return inflight;
    }
    inflightFor = projectId;
    const run = load(projectId).finally(() => {
      if (inflight === run) {
        inflight = null;
      }
    });
    inflight = run;
    return run;
  }

  async function setProvider(req: { projectId: string, kind: WebSearchProviderKind, apiKey?: string, pricePerCallMicros: bigint }) {
    const res = await client.setWebSearchProvider(req);
    provider.value = res.provider ? toProvider(res.provider) : null;
  }

  async function deleteProvider(projectId: string) {
    await client.deleteWebSearchProvider({ projectId });
    provider.value = null;
  }

  async function setSecret(req: { projectId: string, host: string, header: string, value: string }) {
    const res = await client.setHTTPSecret(req);
    const row = toSecret(res.secret!);
    const at = secrets.value.findIndex(s => s.id === row.id);
    if (at === -1) {
      secrets.value = [...secrets.value, row];
    } else {
      secrets.value = secrets.value.map(s => (s.id === row.id ? row : s));
    }
  }

  async function deleteSecret(id: string) {
    try {
      await client.deleteHTTPSecret({ id });
    } catch (e) {
      if (errorCode(e) !== 'WEB004') {
        throw e;
      }
    }
    secrets.value = secrets.value.filter(s => s.id !== id);
  }

  return { provider, availableKinds, usedBy, secrets, loading, loaded, error, usableProvider, searchReady, load, ensureLoaded, setProvider, deleteProvider, setSecret, deleteSecret };
});
