import type { ModelPrice, ModelProvider, ProviderModel } from '~/stores/providers';
import { useToast } from '#base/components/ui/toast';
import { nameTaken } from '~/lib/names/names';
import { isHttpsUrl } from '~/lib/url/url';
import { useProvidersStore } from '~/stores/providers';

// maxModels is the most models one provider may list.
export const maxModels = 64;
// priceKeys are the four token prices a model may have, in USD per 1M tokens.
export const priceKeys = ['input', 'output', 'cacheRead', 'cacheWrite'] as const;
export type PriceKey = typeof priceKeys[number];
// mainPriceKeys show on each model row; the cache prices sit under More prices.
export const mainPriceKeys = ['input', 'output'] as const;
export const morePriceKeys = ['cacheRead', 'cacheWrite'] as const;

export interface ModelRow {
  key: number;
  id: string;
  vision: boolean;
  embedding: boolean;
  prices: Record<PriceKey, string>;
}

export interface ProviderErrors {
  name?: string;
  baseUrl?: string;
  key?: string;
}

// hasPrice reports whether a model has both token prices, which a cost cap needs.
export function hasPrice(model: ProviderModel): boolean {
  return model.price.input !== undefined && model.price.output !== undefined;
}

// hasMorePrices reports whether a model row has a cache price, so its More prices start open.
export function hasMorePrices(row: ModelRow): boolean {
  return morePriceKeys.some(k => row.prices[k].trim() !== '');
}

function toPrice(prices: Record<PriceKey, string>): ModelPrice {
  const price: ModelPrice = {};
  for (const k of priceKeys) {
    if (prices[k].trim() !== '') {
      price[k] = Number(prices[k]);
    }
  }
  return price;
}

function rowError(row: ModelRow, ids: string[]): string {
  const id = row.id.trim();
  if (!id) {
    return 'providers.model_id_required';
  }
  if (ids.filter(other => other === id).length > 1) {
    return 'providers.model_id_taken';
  }
  return priceKeys.some(k => row.prices[k].trim() !== '' && !(Number(row.prices[k]) >= 0)) ? 'providers.price_invalid' : '';
}

// useProviderForm holds the add and edit form of a model provider; the key is write-only and a new URL needs it again.
export function useProviderForm() {
  const { t } = useI18n();
  const providers = useProvidersStore();
  const toast = useToast();

  const open = ref(false);
  const editing = ref<ModelProvider | null>(null);
  const name = ref('');
  const baseUrl = ref('');
  const key = ref('');
  const replacingKey = ref(false);
  const models = ref<ModelRow[]>([]);
  const errors = ref<ProviderErrors>({});
  const modelErrors = ref<Record<number, string>>({});
  const busy = ref(false);
  let nextKey = 1;

  const urlChanged = computed(() => !!editing.value && baseUrl.value.trim() !== editing.value.baseUrl);
  const keyRequired = computed(() => !editing.value || urlChanged.value);
  const keyShown = computed(() => keyRequired.value || replacingKey.value);

  function row(model?: ProviderModel): ModelRow {
    const p = model?.price ?? {};
    const text = (n?: number) => (n === undefined ? '' : String(n));
    return { key: nextKey++, id: model?.id ?? '', vision: model?.vision ?? false, embedding: model?.embedding ?? false, prices: { input: text(p.input), output: text(p.output), cacheRead: text(p.cacheRead), cacheWrite: text(p.cacheWrite) } };
  }

  function show(provider?: ModelProvider) {
    editing.value = provider ?? null;
    name.value = provider?.name ?? '';
    baseUrl.value = provider?.baseUrl ?? '';
    key.value = '';
    replacingKey.value = false;
    models.value = provider ? provider.models.map(row) : [row()];
    errors.value = {};
    modelErrors.value = {};
    open.value = true;
  }

  function hide() {
    key.value = '';
    open.value = false;
  }

  function addModel() {
    if (models.value.length < maxModels) {
      models.value.push(row());
    }
  }

  function removeModel(k: number) {
    models.value = models.value.filter(m => m.key !== k);
  }

  function validate(): boolean {
    const next: ProviderErrors = {};
    if (!name.value.trim()) {
      next.name = t('providers.name_required');
    } else if (nameTaken(providers.items, name.value, editing.value?.id)) {
      next.name = t('providers.name_taken');
    }
    if (!isHttpsUrl(baseUrl.value.trim())) {
      next.baseUrl = t('providers.url_https');
    }
    if (keyRequired.value && !key.value.trim()) {
      next.key = t('providers.key_required');
    }
    const ids = models.value.map(m => m.id.trim());
    const rows: Record<number, string> = {};
    for (const m of models.value) {
      const err = rowError(m, ids);
      if (err) {
        rows[m.key] = t(err);
      }
    }
    errors.value = next;
    modelErrors.value = rows;
    return Object.keys(next).length === 0 && Object.keys(rows).length === 0;
  }

  async function submit(): Promise<boolean> {
    if (busy.value || !validate()) {
      return false;
    }
    busy.value = true;
    const base = editing.value;
    providers.save({
      id: base?.id ?? `prv_${Date.now().toString(36)}`,
      name: name.value.trim(),
      baseUrl: baseUrl.value.trim(),
      keySet: true,
      models: models.value.map(m => ({ id: m.id.trim(), vision: m.vision, embedding: m.embedding, price: toPrice(m.prices) })),
      usedBy: base?.usedBy ?? [],
    });
    toast.push('success', t('providers.saved', { name: name.value.trim() }));
    key.value = '';
    busy.value = false;
    open.value = false;
    return true;
  }

  return { open, editing, name, baseUrl, key, replacingKey, models, errors, modelErrors, busy, urlChanged, keyRequired, keyShown, show, hide, addModel, removeModel, submit };
}
