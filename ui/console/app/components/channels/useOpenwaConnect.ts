import type { ShownError } from '#base/lib/apperror/message';
import type { WhatsAppStatus } from '~/stores/channels';
import { onScopeDispose } from 'vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode, errorMeta } from '#base/lib/apperror/message';
import { isWebUrl } from '~/lib/url/url';
import { useChannelsStore } from '~/stores/channels';

export interface OpenwaErrors {
  baseUrl?: string;
  openwaOrg?: string;
  openwaProject?: string;
  deviceId?: string;
  secret?: string;
  deviceKey?: string;
}

export type OpenwaResult = WhatsAppStatus | 'bad_signature';

export const devicePattern = /^dev_[\w-]{16}$/;

export const checkPolls = { every: 2_000, times: 15 };

// useOpenwaConnect saves an OpenWA connection with its device key, shows the webhook URL to paste into OpenWA, sets the signing secret OpenWA then mints, and checks the status until the first verified delivery; secrets are write-only.
export function useOpenwaConnect(projectId: string, agentId: string) {
  const { t } = useI18n();
  const channels = useChannelsStore();
  const errorMessage = useErrorMessage();

  const open = ref(false);
  const step = ref(1);
  const baseUrl = ref('');
  const openwaOrg = ref('');
  const openwaProject = ref('');
  const deviceId = ref('');
  const secret = ref('');
  const deviceKey = ref('');
  const errors = ref<OpenwaErrors>({});
  const formError = ref<ShownError | null>(null);
  const saving = ref(false);
  const checking = ref(false);
  const result = ref<OpenwaResult | null>(null);
  const saved = computed(() => channels.connection(agentId));
  const webhookUrl = computed(() => saved.value?.webhookUrl ?? '');
  const needsKey = computed(() => !saved.value || saved.value.baseUrl !== baseUrl.value.trim() || saved.value.deviceId !== deviceId.value.trim());
  const keyNote = computed(() => (!saved.value ? '' : needsKey.value ? 'openwa.key_reenter' : 'openwa.key_keep'));
  const secretSet = computed(() => !!saved.value?.secretSet);
  let run = 0;

  onScopeDispose(hide, true);

  for (const [field, value] of [['baseUrl', baseUrl], ['openwaOrg', openwaOrg], ['openwaProject', openwaProject], ['deviceId', deviceId], ['secret', secret], ['deviceKey', deviceKey]] as const) {
    watch(value, () => clear(field));
  }

  function clear(field: keyof OpenwaErrors) {
    if (errors.value[field]) {
      const { [field]: _gone, ...rest } = errors.value;
      errors.value = rest;
    }
  }

  function show(resume = false) {
    const c = saved.value;
    baseUrl.value = c?.baseUrl ?? channels.settings.defaultOpenwaBaseUrl;
    openwaOrg.value = c?.openwaOrg ?? '';
    openwaProject.value = c?.openwaProject ?? '';
    deviceId.value = c?.deviceId ?? '';
    secret.value = '';
    deviceKey.value = '';
    result.value = null;
    formError.value = null;
    errors.value = {};
    step.value = resume && c ? 3 : 1;
    open.value = true;
  }

  function hide() {
    run++;
    checking.value = false;
    secret.value = '';
    deviceKey.value = '';
    open.value = false;
  }

  function validate(): boolean {
    const next: OpenwaErrors = {};
    if (!isWebUrl(baseUrl.value.trim())) {
      next.baseUrl = t('openwa.base_url_format');
    }
    if (!openwaOrg.value.trim()) {
      next.openwaOrg = t('openwa.org_required');
    }
    if (!openwaProject.value.trim()) {
      next.openwaProject = t('openwa.project_required');
    }
    if (!devicePattern.test(deviceId.value.trim())) {
      next.deviceId = t('openwa.device_format');
    }
    if (needsKey.value && !deviceKey.value.trim()) {
      next.deviceKey = t('openwa.key_required');
    }
    errors.value = next;
    return Object.keys(next).length === 0;
  }

  function back() {
    step.value = Math.max(1, step.value - 1);
  }

  async function next() {
    if (step.value === 1) {
      step.value = 2;
      return;
    }
    if (step.value === 2) {
      await save();
    }
  }

  async function save() {
    if (saving.value || !validate()) {
      return;
    }
    saving.value = true;
    formError.value = null;
    try {
      const c = saved.value;
      if (!c) {
        await channels.create(projectId, agentId, { baseUrl: baseUrl.value, openwaOrg: openwaOrg.value, openwaProject: openwaProject.value, deviceId: deviceId.value, deviceKey: deviceKey.value });
      } else {
        await channels.update(c.id, { baseUrl: baseUrl.value, openwaOrg: openwaOrg.value, openwaProject: openwaProject.value, deviceId: deviceId.value, deviceKey: deviceKey.value });
      }
      deviceKey.value = '';
      step.value = 3;
    } catch (e) {
      if (errorCode(e) === 'CHN002' && errorMeta(e).field === 'base_url') {
        errors.value = { baseUrl: t('openwa.base_url_refused') };
      } else {
        formError.value = errorMessage(e, 'error.save_channel');
      }
    } finally {
      saving.value = false;
    }
  }

  async function saveSecret() {
    const c = saved.value;
    if (!c || saving.value) {
      return;
    }
    if (!secret.value.trim()) {
      errors.value = { secret: t('openwa.secret_required') };
      return;
    }
    saving.value = true;
    formError.value = null;
    try {
      await channels.setSecret(c.id, secret.value);
      secret.value = '';
    } catch (e) {
      formError.value = errorMessage(e, 'error.save_channel');
      return;
    } finally {
      saving.value = false;
    }
    await check();
  }

  async function check() {
    const c = saved.value;
    if (!c || !c.secretSet || checking.value) {
      return;
    }
    checking.value = true;
    result.value = null;
    const mine = ++run;
    const badBefore = c.lastBadSignatureAt?.getTime() ?? 0;
    try {
      for (let i = 0; i < checkPolls.times; i++) {
        const now = await channels.refresh(c.id);
        if (mine !== run) {
          return;
        }
        if (now.status !== 'waiting') {
          result.value = now.status;
          return;
        }
        if ((now.lastBadSignatureAt?.getTime() ?? 0) > badBefore) {
          result.value = 'bad_signature';
          return;
        }
        await new Promise(resolve => setTimeout(resolve, checkPolls.every));
        if (mine !== run) {
          return;
        }
      }
      result.value = 'waiting';
    } catch (e) {
      formError.value = errorMessage(e, 'error.load_channels');
    } finally {
      if (mine === run) {
        checking.value = false;
      }
    }
  }

  return { open, step, baseUrl, openwaOrg, openwaProject, deviceId, secret, deviceKey, errors, formError, saving, checking, result, webhookUrl, needsKey, keyNote, secretSet, show, hide, next, back, save, saveSecret, check };
}
