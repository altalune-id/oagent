import type { ShownError } from '#base/lib/apperror/message';
import type { HttpSecret } from '~/stores/webTools';
import { create } from '@bufbuild/protobuf';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useValidation } from '#base/composables/useValidation';
import { SetHTTPSecretRequestSchema } from '#gen/webtool/v1/webtool_pb';
import { hostOf } from '~/components/agent/configure/runtime';
import { useWebToolsStore } from '~/stores/webTools';

// useHttpSecretForm adds a project HTTP secret, replaces a saved one's value, and deletes one after a confirm.
export function useHttpSecretForm(projectId: string) {
  const { t } = useI18n();
  const store = useWebToolsStore();
  const toast = useToast();
  const validation = useValidation(SetHTTPSecretRequestSchema, { fields: ['host', 'header', 'value'] });

  const open = ref(false);
  const editing = ref<HttpSecret | null>(null);
  const host = ref('');
  const header = ref('');
  const value = ref('');
  const busy = ref(false);
  const formError = ref<ShownError | null>(null);
  const exists = ref('');
  const errors = computed(() => (exists.value ? { ...validation.errors.value, header: exists.value } : validation.errors.value));

  function show(secret: HttpSecret | null) {
    editing.value = secret;
    host.value = secret?.host ?? '';
    header.value = secret?.header ?? '';
    value.value = '';
    formError.value = null;
    exists.value = '';
    validation.clear();
    open.value = true;
  }

  function hide() {
    show(null);
    open.value = false;
  }

  async function submit(): Promise<boolean> {
    const req = editing.value
      ? { projectId, host: editing.value.host, header: editing.value.header, value: value.value }
      : { projectId, host: hostOf(host.value), header: header.value.trim(), value: value.value };
    formError.value = null;
    exists.value = '';
    if (!editing.value && store.secrets.some(s => s.host === req.host && s.header.toLowerCase() === req.header.toLowerCase())) {
      exists.value = t('webtools.secret_exists');
      return false;
    }
    if (!validation.check(create(SetHTTPSecretRequestSchema, req))) {
      formError.value = validation.formError.value;
      return false;
    }
    busy.value = true;
    try {
      await store.setSecret(req);
      toast.push('success', t('webtools.secret_saved'));
      hide();
      return true;
    } catch (e) {
      formError.value = validation.fromServer(e, 'error.save_http_secret');
      return false;
    } finally {
      busy.value = false;
    }
  }

  const deleting = useConfirm<HttpSecret>(async (secret) => {
    await store.deleteSecret(secret.id);
    toast.push('success', t('webtools.secret_deleted'));
  }, 'error.delete_http_secret');

  return { open, editing, host, header, value, busy, errors, formError, showAdd: () => show(null), showReplace: show, hide, submit, deleting };
}
