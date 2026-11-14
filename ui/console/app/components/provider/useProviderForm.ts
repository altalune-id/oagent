import type { ShownError } from '#base/lib/apperror/message';
import type { ModelPrices, ModelProvider, ProviderInput, ProviderModel } from '~/stores/modelProviders';
import { useToast } from '#base/components/ui/toast';
import { useValidation } from '#base/composables/useValidation';
import { CreateModelProviderRequestSchema, ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { parseUsd, usdText } from '~/lib/money/usd';
import { createRequest, useModelProvidersStore } from '~/stores/modelProviders';

export const priceKeys = ['input', 'output', 'cacheRead', 'cacheWrite'] as const;
export type PriceKey = typeof priceKeys[number];
export const mainPriceKeys = ['input', 'output'] as const;
export const morePriceKeys = ['cacheRead', 'cacheWrite'] as const;

export const providerKinds = [ModelProviderKind.OPENAI_COMPATIBLE, ModelProviderKind.ANTHROPIC] as const;
export const ANTHROPIC_URL = 'https://api.anthropic.com';

// kindKey names the locale key of a provider kind.
export function kindKey(kind: ModelProviderKind): string {
  return kind === ModelProviderKind.ANTHROPIC ? 'providers.kind_anthropic' : 'providers.kind_openai_compatible';
}

const microsKeys: Record<PriceKey, Exclude<keyof ModelPrices, 'audioMinuteMicros'>> = { input: 'inputMicros', output: 'outputMicros', cacheRead: 'cacheReadMicros', cacheWrite: 'cacheWriteMicros' };

export interface ModelRow {
  key: number;
  id: string;
  vision: boolean;
  embedding: boolean;
  prices: Record<PriceKey, string>;
  transcription: boolean;
  storedPrices?: ModelPrices;
}

export interface ProviderErrors {
  name?: string;
  baseUrl?: string;
  key?: string;
  models?: string;
}

// hasPrice reports whether a model has prices, which a cost cap needs.
export function hasPrice(model: ProviderModel): boolean {
  return !!model.prices;
}

// hasMorePrices reports whether a model row has a cache price, so its More prices start open.
export function hasMorePrices(row: ModelRow): boolean {
  return morePriceKeys.some(k => row.prices[k].trim() !== '');
}

export type ParsedPrices = { kind: 'unpriced' } | { kind: 'priced', prices: ModelPrices } | { kind: 'invalid' };

// toPrices reads a row's dollar prices into micro-USD: all empty is unpriced, a blank cache price is zero, and so is a blank output price on an embedding model.
export function toPrices(prices: Record<PriceKey, string>, embedding = false): ParsedPrices {
  if (priceKeys.every(k => prices[k].trim() === '')) {
    return { kind: 'unpriced' };
  }
  const out = {} as ModelPrices;
  for (const k of priceKeys) {
    const text = prices[k].trim();
    const optional = morePriceKeys.includes(k as typeof morePriceKeys[number]) || (embedding && k === 'output');
    const micros = text === '' && optional ? 0n : parseUsd(text);
    if (micros === null) {
      return { kind: 'invalid' };
    }
    out[microsKeys[k]] = micros;
  }
  return { kind: 'priced', prices: out };
}

function rowModel(row: ModelRow): ProviderModel {
  if (row.transcription) {
    return { id: row.id, vision: row.vision, embedding: row.embedding, transcription: true, prices: row.storedPrices };
  }
  const parsed = toPrices(row.prices, row.embedding);
  return { id: row.id, vision: row.vision, embedding: row.embedding, prices: parsed.kind === 'priced' ? parsed.prices : undefined };
}

function rowError(row: ModelRow, ids: string[]): string {
  const id = row.id.trim();
  if (!id) {
    return 'providers.model_id_required';
  }
  if (ids.filter(other => other === id).length > 1) {
    return 'providers.model_id_taken';
  }
  return toPrices(row.prices, row.embedding).kind === 'invalid' ? 'providers.price_invalid' : '';
}

// useProviderForm holds the add and edit form of a model provider; the key is write-only, never kept after a save, and a new URL needs it again.
export function useProviderForm(orgId: string, projectId: string) {
  const { t } = useI18n();
  const providers = useModelProvidersStore();
  const toast = useToast();
  const validation = useValidation(CreateModelProviderRequestSchema, {
    fields: ['name', 'base_url', 'api_key', 'models'],
    codes: { PRV002: 'name', PRV003: 'base_url', PRV004: 'models', PRV005: 'models', PRV006: 'api_key', PRV007: 'models' },
  });

  const open = ref(false);
  const editing = ref<ModelProvider | null>(null);
  const name = ref('');
  const kind = ref<ModelProviderKind>(ModelProviderKind.OPENAI_COMPATIBLE);
  const baseUrl = ref('');
  const key = ref('');
  const replacingKey = ref(false);
  const models = ref<ModelRow[]>([]);
  const keyMissing = ref(false);
  const modelErrors = ref<Record<number, string>>({});
  const priceErrors = ref<number[]>([]);
  const formError = ref<ShownError | null>(null);
  const busy = ref(false);
  let nextKey = 1;

  const anthropic = computed(() => kind.value === ModelProviderKind.ANTHROPIC);
  const urlV1 = computed(() => anthropic.value && /\/v1\/?$/.test(baseUrl.value.trim()));
  const urlChanged = computed(() => !!editing.value && baseUrl.value.trim() !== editing.value.baseUrl);
  const keyRequired = computed(() => !editing.value || urlChanged.value);
  const keyShown = computed(() => keyRequired.value || replacingKey.value);
  const errors = computed<ProviderErrors>(() => {
    const v = validation.errors.value;
    const out: ProviderErrors = { name: v.name, baseUrl: v.base_url, key: keyMissing.value ? t('providers.key_required') : v.api_key, models: v.models };
    return Object.fromEntries(Object.entries(out).filter(([, msg]) => msg)) as ProviderErrors;
  });

  function row(model?: ProviderModel): ModelRow {
    const p = model?.prices;
    const text = (k: PriceKey) => (p ? usdText(p[microsKeys[k]]) : '');
    return { key: nextKey++, id: model?.id ?? '', vision: model?.vision ?? false, embedding: model?.embedding ?? false, prices: { input: text('input'), output: text('output'), cacheRead: text('cacheRead'), cacheWrite: text('cacheWrite') }, transcription: !!model?.transcription, storedPrices: model?.prices };
  }

  function show(provider?: ModelProvider) {
    editing.value = provider ?? null;
    name.value = provider?.name ?? '';
    kind.value = provider?.kind ?? ModelProviderKind.OPENAI_COMPATIBLE;
    baseUrl.value = provider?.baseUrl ?? '';
    key.value = '';
    replacingKey.value = false;
    models.value = provider ? provider.models.map(row) : [row()];
    keyMissing.value = false;
    modelErrors.value = {};
    priceErrors.value = [];
    formError.value = null;
    validation.clear();
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    key.value = '';
    open.value = false;
  }

  function setKind(next: ModelProviderKind) {
    if (editing.value) {
      return;
    }
    if (next === ModelProviderKind.ANTHROPIC && !baseUrl.value.trim()) {
      baseUrl.value = ANTHROPIC_URL;
    }
    if (next !== ModelProviderKind.ANTHROPIC && baseUrl.value.trim() === ANTHROPIC_URL) {
      baseUrl.value = '';
    }
    kind.value = next;
  }

  function addModel() {
    models.value.push(row());
  }

  function removeModel(k: number) {
    models.value = models.value.filter(m => m.key !== k);
  }

  function input(): ProviderInput | null {
    const ids = models.value.map(m => m.id.trim());
    const rows: Record<number, string> = {};
    const prices: number[] = [];
    for (const m of models.value) {
      const err = rowError(m, ids);
      if (err) {
        rows[m.key] = t(err);
      }
      if (err === 'providers.price_invalid') {
        prices.push(m.key);
      }
    }
    modelErrors.value = rows;
    priceErrors.value = prices;
    keyMissing.value = keyRequired.value && !key.value.trim();
    const out = { name: name.value, kind: kind.value, baseUrl: baseUrl.value, apiKey: key.value, models: models.value.map(rowModel) };
    const storedKey = 'kept';
    const shaped = validation.check(createRequest(orgId, projectId, { ...out, apiKey: out.apiKey.trim() || storedKey }));
    formError.value = Object.keys(rows).length ? null : validation.formError.value;
    return shaped && !keyMissing.value && Object.keys(rows).length === 0 ? out : null;
  }

  async function submit(): Promise<boolean> {
    if (busy.value) {
      return false;
    }
    const req = input();
    if (!req) {
      return false;
    }
    busy.value = true;
    try {
      const was = editing.value;
      const saved = was ? await providers.update(was.id, req) : await providers.add(orgId, projectId, req);
      toast.push('success', t('providers.saved', { name: saved.name }));
      key.value = '';
      open.value = false;
      return true;
    } catch (e) {
      formError.value = validation.fromServer(e, 'error.save_provider');
      return false;
    } finally {
      busy.value = false;
    }
  }

  return { open, editing, name, kind, anthropic, urlV1, setKind, baseUrl, key, replacingKey, models, errors, modelErrors, priceErrors, formError, busy, urlChanged, keyRequired, keyShown, show, hide, addModel, removeModel, submit };
}
