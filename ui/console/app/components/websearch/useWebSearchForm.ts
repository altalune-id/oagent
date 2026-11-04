import type { SearchProvider } from '~/stores/webSearch';
import { useToast } from '#base/components/ui/toast';
import { useWebSearchStore } from '~/stores/webSearch';

export const searchProviders: { id: SearchProvider, labelKey: string, hintKey: string }[] = [
  { id: 'brave', labelKey: 'websearch.brave', hintKey: 'websearch.brave_hint' },
  { id: 'exa', labelKey: 'websearch.exa', hintKey: 'websearch.exa_hint' },
  { id: 'tavily', labelKey: 'websearch.tavily', hintKey: 'websearch.tavily_hint' },
];

// useWebSearchForm picks the project's one search provider; a new provider needs its own key.
export function useWebSearchForm() {
  const { t } = useI18n();
  const store = useWebSearchStore();
  const toast = useToast();

  const choice = ref<SearchProvider>(store.setting.provider || 'brave');
  const key = ref('');
  const replacing = ref(false);
  const error = ref('');

  const current = computed(() => store.setting.provider);
  const keyShown = computed(() => !current.value || choice.value !== current.value || replacing.value);

  watch(current, (p) => {
    choice.value = p || choice.value;
  });

  function save(): boolean {
    if (!key.value.trim()) {
      error.value = t('websearch.key_required');
      return false;
    }
    store.save(choice.value);
    key.value = '';
    replacing.value = false;
    error.value = '';
    toast.push('success', t('websearch.saved'));
    return true;
  }

  function cancel() {
    choice.value = current.value || choice.value;
    key.value = '';
    replacing.value = false;
    error.value = '';
  }

  return { choice, key, replacing, error, current, keyShown, save, cancel };
}
