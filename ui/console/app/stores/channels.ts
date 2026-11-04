import type { ShownError } from '#base/lib/apperror/message';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { sampleChannelId, sampleChannelSettings, sampleOpenwaCheck, sampleOpenwaFill, sampleWebhookUrl } from '~/preview/samples';

export type WhatsAppStatus = 'waiting' | 'connected' | 'bad_signature' | 'key_refused' | 'device_mismatch';

export type ChannelProvider = 'openwa' | 'whatsapp_business';

export interface WhatsAppConnection {
  id: string;
  provider: ChannelProvider;
  status: WhatsAppStatus;
  baseUrl: string;
  deviceId: string;
  webhookUrl: string;
  lastMessageAt?: Date;
  rejected24h: number;
  rejectedLastHour: number;
}

export type OpenwaCheck = 'connected' | 'no_message' | 'bad_signature' | 'key_refused' | 'device_mismatch';

export interface OpenwaFill {
  deviceId: string;
  secret: string;
  deviceKey: string;
}

export interface ChannelSettings {
  defaultOpenwaBaseUrl: string;
}

// NOTE: shaped like the server setting that will carry the default OpenWA base URL; empty leaves the field empty.

// TODO: replace with the channel connection RPCs; until then this keeps each agent's connections on the page.
export const useChannelsStore = defineStore('channels', () => {
  const connections = ref<Record<string, WhatsAppConnection>>({});
  const settings = ref<ChannelSettings>({ defaultOpenwaBaseUrl: '' });
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  let loadedFor = '';

  async function load(agentId: string) {
    loading.value = true;
    error.value = null;
    try {
      settings.value = await Promise.resolve().then(sampleChannelSettings);
      loadedFor = agentId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_channels');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(agentId: string) {
    return loadedFor === agentId ? Promise.resolve() : load(agentId);
  }

  const keyOf = (agentId: string, provider: ChannelProvider) => `${agentId}:${provider}`;

  function connection(agentId: string, provider: ChannelProvider = 'openwa'): WhatsAppConnection | undefined {
    return connections.value[keyOf(agentId, provider)];
  }

  function list(agentId: string): WhatsAppConnection[] {
    return Object.entries(connections.value).filter(([key]) => key.startsWith(`${agentId}:`)).map(([, c]) => c);
  }

  function save(agentId: string, next: WhatsAppConnection) {
    connections.value = { ...connections.value, [keyOf(agentId, next.provider)]: next };
  }

  function replaced(agentId: string, provider: ChannelProvider = 'openwa') {
    const current = connection(agentId, provider);
    if (current) {
      save(agentId, { ...current, status: 'connected', rejectedLastHour: 0, lastMessageAt: new Date() });
    }
  }

  function remove(agentId: string, provider: ChannelProvider = 'openwa') {
    const { [keyOf(agentId, provider)]: _gone, ...rest } = connections.value;
    connections.value = rest;
  }

  function newChannelId(): string {
    return sampleChannelId;
  }

  function webhookUrl(channelId: string): string {
    return sampleWebhookUrl(channelId);
  }

  // TODO: drop with the channel RPCs; the preview starts a new connection with sample values so the walk can go on.
  function prefill(): OpenwaFill {
    return sampleOpenwaFill();
  }

  function check(input: { deviceId: string, secret: string }): Promise<OpenwaCheck> {
    return sampleOpenwaCheck(input);
  }

  return { connections, settings, loading, error, load, ensureLoaded, connection, list, save, replaced, remove, newChannelId, webhookUrl, prefill, check };
});
