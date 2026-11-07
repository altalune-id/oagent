import { useToast } from '#base/components/ui/toast';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useChannelsStore } from '~/stores/channels';

export type Replacing = 'secret' | 'key';

// useReplaceSecret sets a connection's signing secret, optionally retiring the previous one, or replaces its device key; the typed value is write-only and cleared on close.
export function useReplaceSecret(connectionId: () => string | undefined) {
  const { t } = useI18n();
  const channels = useChannelsStore();
  const toast = useToast();
  const errorMessage = useErrorMessage();
  const kind = ref<Replacing | null>(null);
  const value = ref('');
  const error = ref('');
  const saving = ref(false);
  const dropPrevious = ref(false);

  function show(next: Replacing) {
    value.value = '';
    error.value = '';
    dropPrevious.value = false;
    kind.value = next;
  }

  function hide() {
    value.value = '';
    error.value = '';
    kind.value = null;
  }

  async function save(): Promise<boolean> {
    const id = connectionId();
    if (!id || saving.value) {
      return false;
    }
    if (!value.value.trim()) {
      error.value = kind.value === 'secret' ? t('openwa.secret_required') : t('openwa.key_required');
      return false;
    }
    saving.value = true;
    try {
      if (kind.value === 'secret') {
        await channels.setSecret(id, value.value, dropPrevious.value);
      } else {
        await channels.update(id, { deviceKey: value.value });
      }
      toast.push('success', kind.value === 'secret' ? t('openwa.secret_replaced') : t('openwa.key_replaced'));
      hide();
      return true;
    } catch (e) {
      error.value = errorMessage(e, 'error.save_channel').message;
      return false;
    } finally {
      saving.value = false;
    }
  }

  watch(value, () => {
    error.value = '';
  });

  return { kind, value, error, saving, dropPrevious, show, hide, save };
}
