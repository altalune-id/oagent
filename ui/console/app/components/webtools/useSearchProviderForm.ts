import type { ShownError } from '#base/lib/apperror/message';
import { create } from '@bufbuild/protobuf';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useValidation } from '#base/composables/useValidation';
import { SetWebSearchProviderRequestSchema, WebSearchProviderKind } from '#gen/webtool/v1/webtool_pb';
import { decimalSeparator, per1kText, perCallMicros } from '~/lib/money/usd';
import { useWebToolsStore } from '~/stores/webTools';

export const searchProviders: { kind: WebSearchProviderKind, labelKey: string, hintKey: string }[] = [
  { kind: WebSearchProviderKind.BRAVE, labelKey: 'webtools.brave', hintKey: 'webtools.brave_hint' },
  { kind: WebSearchProviderKind.EXA, labelKey: 'webtools.exa', hintKey: 'webtools.exa_hint' },
  { kind: WebSearchProviderKind.TAVILY, labelKey: 'webtools.tavily', hintKey: 'webtools.tavily_hint' },
];

// useSearchProviderForm sets the project's one search provider; a new or switched kind needs its own key.
export function useSearchProviderForm(projectId: string) {
  const { t, locale } = useI18n();
  const store = useWebToolsStore();
  const toast = useToast();
  const validation = useValidation(SetWebSearchProviderRequestSchema, {
    fields: ['kind', 'api_key', 'price_per_call_micros'],
    codes: { WEB001: 'kind', WEB002: 'api_key' },
  });

  const selectable = (kind: WebSearchProviderKind) => store.availableKinds.includes(kind);
  const current = computed(() => store.usableProvider);

  const kind = ref(WebSearchProviderKind.UNSPECIFIED);
  const apiKey = ref('');
  const price = ref('');
  const replacing = ref(false);
  const editing = ref(false);
  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const local = ref<Record<string, string>>({});

  const formShown = computed(() => !current.value || editing.value || replacing.value);
  const keyShown = computed(() => !current.value || kind.value !== current.value.kind || replacing.value);
  const errors = computed(() => ({ ...validation.errors.value, ...local.value }));

  function reset() {
    kind.value = current.value?.kind ?? searchProviders.find(p => selectable(p.kind))?.kind ?? WebSearchProviderKind.UNSPECIFIED;
    price.value = current.value ? per1kText(current.value.pricePerCallMicros) : '';
    apiKey.value = '';
    replacing.value = false;
    editing.value = false;
    error.value = null;
    local.value = {};
    validation.clear();
  }

  reset();
  watch(current, reset);

  async function submit(): Promise<boolean> {
    if (!selectable(kind.value)) {
      return false;
    }
    const key = apiKey.value.trim();
    const micros = perCallMicros(price.value, decimalSeparator(locale.value));
    error.value = null;
    validation.clear();
    local.value = {
      ...(keyShown.value && !key ? { api_key: t('webtools.key_required') } : {}),
      ...(micros === null ? { price_per_call_micros: t('webtools.price_invalid') } : {}),
    };
    if (micros === null || Object.keys(local.value).length > 0) {
      return false;
    }
    const req = { projectId, kind: kind.value, pricePerCallMicros: micros, ...(keyShown.value ? { apiKey: key } : {}) };
    if (!validation.check(create(SetWebSearchProviderRequestSchema, req))) {
      error.value = validation.formError.value;
      return false;
    }
    busy.value = true;
    try {
      await store.setProvider(req);
      toast.push('success', t('webtools.saved'));
      reset();
      return true;
    } catch (e) {
      error.value = validation.fromServer(e, 'error.save_web_search_provider');
      return false;
    } finally {
      busy.value = false;
    }
  }

  const removing = useConfirm<boolean>(async () => {
    await store.deleteProvider(projectId);
    toast.push('success', t('webtools.removed'));
  }, 'error.remove_web_search_provider');

  return { kind, apiKey, price, replacing, editing, formShown, busy, error, errors, current, keyShown, selectable, submit, cancel: reset, removing };
}
