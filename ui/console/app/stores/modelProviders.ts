import type { ShownError } from '#base/lib/apperror/message';
import type { CreateModelProviderRequest, ModelProvider as ModelProviderPB, UpdateModelProviderRequest } from '#gen/modelprovider/v1/modelprovider_pb';
import { create } from '@bufbuild/protobuf';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { CreateModelProviderRequestSchema, ModelProviderKind, UpdateModelProviderRequestSchema } from '#gen/modelprovider/v1/modelprovider_pb';

export interface ModelPrices {
  inputMicros: bigint;
  outputMicros: bigint;
  cacheReadMicros: bigint;
  cacheWriteMicros: bigint;
  audioMinuteMicros?: bigint;
}

export interface ProviderModel {
  id: string;
  vision: boolean;
  embedding: boolean;
  transcription?: boolean;
  prices?: ModelPrices;
}

// keyOf is the select value of a model on a provider.
export function keyOf(c: { providerId: string, model: string }): string {
  return `${c.providerId}|${c.model}`;
}

export interface ModelProvider {
  id: string;
  orgId: string;
  projectId: string;
  name: string;
  kind: ModelProviderKind;
  baseUrl: string;
  keySet: boolean;
  models: ProviderModel[];
}

export interface ProviderInput {
  name: string;
  kind: ModelProviderKind;
  baseUrl: string;
  apiKey: string;
  models: ProviderModel[];
}

export function toModelProvider(p: ModelProviderPB): ModelProvider {
  return {
    id: p.id,
    orgId: p.orgId,
    projectId: p.projectId,
    name: p.name,
    kind: p.kind,
    baseUrl: p.baseUrl,
    keySet: p.apiKeySet,
    models: p.models.map(m => ({
      id: m.id,
      vision: m.vision,
      embedding: m.embedding,
      ...(m.transcription ? { transcription: true } : {}),
      prices: m.prices && {
        inputMicros: m.prices.inputMicros,
        outputMicros: m.prices.outputMicros,
        cacheReadMicros: m.prices.cacheReadMicros,
        cacheWriteMicros: m.prices.cacheWriteMicros,
        audioMinuteMicros: m.prices.audioMinuteMicros,
      },
    })),
  };
}

function wireModels(models: ProviderModel[], kind: ModelProviderKind) {
  const anthropic = kind === ModelProviderKind.ANTHROPIC;
  return models.map(m => ({ id: m.id.trim(), vision: m.vision, embedding: !anthropic && m.embedding, transcription: !anthropic && !!m.transcription, prices: m.prices && { ...m.prices, ...(anthropic ? { audioMinuteMicros: 0n } : {}) } }));
}

// createRequest builds the create call for a provider; the key is trimmed.
export function createRequest(orgId: string, projectId: string, input: ProviderInput): CreateModelProviderRequest {
  return create(CreateModelProviderRequestSchema, { orgId, projectId, name: input.name.trim(), kind: input.kind, baseUrl: input.baseUrl.trim(), apiKey: input.apiKey.trim(), models: wireModels(input.models, input.kind) });
}

// updateRequest builds the update call for a provider; an empty key keeps the stored one.
export function updateRequest(id: string, input: ProviderInput): UpdateModelProviderRequest {
  const apiKey = input.apiKey.trim();
  return create(UpdateModelProviderRequestSchema, { id, name: input.name.trim(), kind: input.kind, baseUrl: input.baseUrl.trim(), models: wireModels(input.models, input.kind), ...(apiKey ? { apiKey } : {}) });
}

export const useModelProvidersStore = defineStore('modelProviders', () => {
  const items = ref<ModelProvider[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $modelProviderClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  function put(provider: ModelProvider) {
    const known = items.value.some(p => p.id === provider.id);
    items.value = known ? items.value.map(p => (p.id === provider.id ? provider : p)) : [...items.value, provider];
    return provider;
  }

  async function load(projectId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listModelProviders({ projectId });
      if (mine !== latest) {
        return;
      }
      items.value = (res.modelProviders ?? []).map(toModelProvider);
      loadedFor = projectId;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_providers');
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

  async function add(orgId: string, projectId: string, input: ProviderInput) {
    const res = await client.createModelProvider(createRequest(orgId, projectId, input));
    return put(toModelProvider(res.modelProvider!));
  }

  async function update(id: string, input: ProviderInput) {
    const res = await client.updateModelProvider(updateRequest(id, input));
    return put(toModelProvider(res.modelProvider!));
  }

  async function remove(id: string) {
    await client.deleteModelProvider({ id });
    items.value = items.value.filter(p => p.id !== id);
  }

  return { items, loading, error, load, ensureLoaded, add, update, remove };
});
