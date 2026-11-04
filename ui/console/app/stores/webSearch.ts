import type { ShownError } from '#base/lib/apperror/message';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { sampleWebSearch } from '~/preview/samples';

export type SearchProvider = 'brave' | 'exa' | 'tavily';

export interface WebSearchSetting {
  provider: SearchProvider | '';
  keyAddedAt?: Date;
  usedBy: string[];
}

export const useWebSearchStore = defineStore('webSearch', () => {
  const setting = ref<WebSearchSetting>({ provider: '', usedBy: [] });
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  let loadedFor = '';

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    try {
      setting.value = await Promise.resolve().then(sampleWebSearch);
      loadedFor = projectId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_web_search');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function save(provider: SearchProvider, at = new Date()) {
    setting.value = { ...setting.value, provider, keyAddedAt: at };
  }

  function remove() {
    setting.value = { provider: '', usedBy: setting.value.usedBy };
  }

  return { setting, loading, error, load, ensureLoaded, save, remove };
});
