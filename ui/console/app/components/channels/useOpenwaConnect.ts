import type { OpenwaCheck, WhatsAppConnection, WhatsAppStatus } from '~/stores/channels';
import { isHttpsUrl } from '~/lib/url/url';
import { useChannelsStore } from '~/stores/channels';

export interface OpenwaErrors {
  baseUrl?: string;
  deviceId?: string;
  secret?: string;
  deviceKey?: string;
}

const statusOf: Record<OpenwaCheck, WhatsAppStatus> = {
  connected: 'connected',
  no_message: 'waiting',
  bad_signature: 'bad_signature',
  key_refused: 'key_refused',
  device_mismatch: 'device_mismatch',
};

// useOpenwaConnect walks the three openwa steps (webhook, secret and key, check) and stores the connection; secrets are write-only.
export function useOpenwaConnect(agentId: string, check?: (input: { deviceId: string, secret: string }) => Promise<OpenwaCheck>) {
  const { t } = useI18n();
  const channels = useChannelsStore();
  const probe = check ?? channels.check;

  const open = ref(false);
  const step = ref(1);
  const baseUrl = ref('');
  const deviceId = ref('');
  const secret = ref('');
  const deviceKey = ref('');
  const errors = ref<OpenwaErrors>({});
  const checking = ref(false);
  const result = ref<OpenwaCheck | null>(null);
  const saved = computed(() => channels.connection(agentId, 'openwa'));
  const channelId = ref('');
  const webhookUrl = computed(() => channels.webhookUrl(saved.value?.id ?? channelId.value));
  const needsSecrets = computed(() => !saved.value || saved.value.baseUrl !== baseUrl.value.trim() || saved.value.deviceId !== deviceId.value.trim());
  const secretsNote = computed(() => (!saved.value ? '' : needsSecrets.value ? 'openwa.secrets_reenter' : 'openwa.secrets_keep'));
  let run = 0;

  watch(baseUrl, () => clear('baseUrl'));
  watch(deviceId, () => clear('deviceId'));
  watch(secret, () => clear('secret'));
  watch(deviceKey, () => clear('deviceKey'));

  function clear(field: keyof OpenwaErrors) {
    if (errors.value[field]) {
      const { [field]: _gone, ...rest } = errors.value;
      errors.value = rest;
    }
  }

  function show(resume = false) {
    baseUrl.value = saved.value?.baseUrl ?? channels.settings.defaultOpenwaBaseUrl;
    const fill = saved.value ? { deviceId: saved.value.deviceId, secret: '', deviceKey: '' } : channels.prefill();
    deviceId.value = fill.deviceId;
    secret.value = fill.secret;
    deviceKey.value = fill.deviceKey;
    result.value = null;
    channelId.value = saved.value?.id ?? channels.newChannelId();
    step.value = resume && saved.value ? 3 : 1;
    errors.value = {};
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
    if (!isHttpsUrl(baseUrl.value.trim())) {
      next.baseUrl = t('openwa.base_url_https');
    }
    if (!/^[\w-]{3,64}$/.test(deviceId.value.trim())) {
      next.deviceId = t('openwa.device_required');
    }
    if (needsSecrets.value && !secret.value.trim()) {
      next.secret = t('openwa.secret_required');
    }
    if (needsSecrets.value && !deviceKey.value.trim()) {
      next.deviceKey = t('openwa.key_required');
    }
    errors.value = next;
    return Object.keys(next).length === 0;
  }

  function next() {
    if (step.value === 2 && !validate()) {
      return;
    }
    step.value = Math.min(3, step.value + 1);
  }

  function back() {
    step.value = Math.max(1, step.value - 1);
  }

  function store(status: WhatsAppStatus) {
    const connection: WhatsAppConnection = {
      id: saved.value?.id ?? channelId.value,
      provider: 'openwa',
      status,
      baseUrl: baseUrl.value.trim(),
      deviceId: deviceId.value.trim(),
      webhookUrl: webhookUrl.value,
      lastMessageAt: status === 'connected' ? new Date() : undefined,
      rejected24h: status === 'bad_signature' || status === 'device_mismatch' ? 3 : 0,
      rejectedLastHour: status === 'bad_signature' || status === 'device_mismatch' ? 3 : 0,
    };
    channels.save(agentId, connection);
  }

  function ready(): boolean {
    if (validate()) {
      return true;
    }
    step.value = 2;
    return false;
  }

  async function saveAndTest() {
    if (checking.value || !ready()) {
      return;
    }
    checking.value = true;
    result.value = null;
    const mine = ++run;
    const got = await probe({ deviceId: deviceId.value.trim(), secret: secret.value.trim() });
    if (mine !== run || !open.value) {
      return;
    }
    checking.value = false;
    result.value = got;
    store(statusOf[got]);
    secret.value = '';
    deviceKey.value = '';
  }

  function finishLater() {
    if (ready()) {
      store('waiting');
      hide();
    }
  }

  return { open, step, baseUrl, deviceId, secret, deviceKey, errors, checking, result, webhookUrl, needsSecrets, secretsNote, show, hide, next, back, saveAndTest, finishLater };
}
