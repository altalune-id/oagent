import type { ShownError } from '#base/lib/apperror/message';
import { sampleProviders } from '~/preview/samples';

export interface ModelPrice {
  input?: number;
  output?: number;
  cacheRead?: number;
  cacheWrite?: number;
}

export interface ProviderModel {
  id: string;
  vision: boolean;
  embedding: boolean;
  price: ModelPrice;
}

export interface ModelProvider {
  id: string;
  name: string;
  baseUrl: string;
  keySet: boolean;
  models: ProviderModel[];
  usedBy: string[];
}

export const useProvidersStore = defineStore('providers', () => {
  const items = ref<ModelProvider[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  let loadedFor = '';

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    items.value = await Promise.resolve(sampleProviders());
    loadedFor = projectId;
    loading.value = false;
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function save(provider: ModelProvider) {
    const known = items.value.some(p => p.id === provider.id);
    items.value = known ? items.value.map(p => (p.id === provider.id ? provider : p)) : [...items.value, provider];
  }

  function remove(id: string) {
    items.value = items.value.filter(p => p.id !== id);
  }

  return { items, loading, error, load, ensureLoaded, save, remove };
});
