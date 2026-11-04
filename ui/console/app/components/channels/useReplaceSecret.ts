import type { ChannelProvider } from '~/stores/channels';
import { useToast } from '#base/components/ui/toast';
import { useChannelsStore } from '~/stores/channels';

export type Replacing = 'secret' | 'key';

// useReplaceSecret replaces a connection's signing secret or device key; the typed value is write-only and cleared on close.
export function useReplaceSecret(agentId: string, provider: ChannelProvider = 'openwa') {
  const { t } = useI18n();
  const channels = useChannelsStore();
  const toast = useToast();
  const kind = ref<Replacing | null>(null);
  const value = ref('');
  const keepOld = ref(true);
  const error = ref('');

  function show(next: Replacing) {
    value.value = '';
    error.value = '';
    keepOld.value = true;
    kind.value = next;
  }

  function hide() {
    value.value = '';
    error.value = '';
    kind.value = null;
  }

  function save(): boolean {
    if (!value.value.trim()) {
      error.value = kind.value === 'secret' ? t('openwa.secret_required') : t('openwa.key_required');
      return false;
    }
    channels.replaced(agentId, provider);
    toast.push('success', kind.value === 'secret' ? t('openwa.secret_replaced') : t('openwa.key_replaced'));
    hide();
    return true;
  }

  watch(value, () => {
    error.value = '';
  });

  return { kind, value, keepOld, error, show, hide, save };
}
