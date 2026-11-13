import type { Timestamp } from '@bufbuild/protobuf/wkt';
import type { ShownError } from '#base/lib/apperror/message';
import type { Connection } from '#gen/channel/v1/channel_pb';
import type { ChannelTiming, ReplyTexts } from '~/components/channels/replies';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { ConnectionStatus } from '#gen/channel/v1/channel_pb';

export type WhatsAppStatus = 'waiting' | 'connected' | 'device_mismatch' | 'key_refused';

export type ChannelProvider = 'openwa';

export interface WhatsAppConnection {
  id: string;
  agentId: string;
  provider: ChannelProvider;
  enabled: boolean;
  status: WhatsAppStatus;
  baseUrl: string;
  openwaOrg: string;
  openwaProject: string;
  deviceId: string;
  webhookUrl: string;
  secretSet: boolean;
  secondarySet: boolean;
  quietMs: number;
  quietMaxMs: number;
  replies: ReplyTexts;
  lastResultAt?: Date;
  lastBadSignatureAt?: Date;
}

export interface ChannelSettings {
  defaultOpenwaBaseUrl: string;
  defaultReplies: ReplyTexts;
  timing: ChannelTiming;
}

export interface OpenwaInput {
  baseUrl: string;
  openwaOrg: string;
  openwaProject: string;
  deviceId: string;
  deviceKey: string;
}

export interface ConnectionPatch {
  baseUrl?: string;
  openwaOrg?: string;
  openwaProject?: string;
  deviceId?: string;
  deviceKey?: string;
  quietMs?: number;
  quietMaxMs?: number;
  replies?: ReplyTexts;
  enabled?: boolean;
}

const statuses: Record<ConnectionStatus, WhatsAppStatus> = {
  [ConnectionStatus.UNSPECIFIED]: 'waiting',
  [ConnectionStatus.WAITING]: 'waiting',
  [ConnectionStatus.CONNECTED]: 'connected',
  [ConnectionStatus.DEVICE_MISMATCH]: 'device_mismatch',
  [ConnectionStatus.KEY_REFUSED]: 'key_refused',
};

const date = (t?: Timestamp) => (t ? timestampDate(t) : undefined);

// toConnection maps a wire connection; secrets never come back, only whether one is set and whether a previous one is still accepted.
export function toConnection(c: Connection): WhatsAppConnection {
  return {
    id: c.id,
    agentId: c.agentId,
    provider: 'openwa',
    enabled: c.enabled,
    status: statuses[c.status],
    baseUrl: c.baseUrl,
    openwaOrg: c.openwaOrg,
    openwaProject: c.openwaProject,
    deviceId: c.deviceId,
    webhookUrl: c.webhookUrl,
    secretSet: c.secretSet,
    secondarySet: c.secondarySet,
    quietMs: c.quietWindowMs,
    quietMaxMs: c.quietMaxMs,
    replies: { failure: c.replyFailure, voice: c.replyVoice, photo: c.replyPhotoUnreadable, approvalPrompt: c.approvalPrompt, approvalExpired: c.approvalExpired },
    lastResultAt: date(c.lastResultAt),
    lastBadSignatureAt: date(c.lastBadSignatureAt),
  };
}

// patchRequest builds an update body with only the changed fields; a device key is sent only when one was typed.
export function patchRequest(id: string, patch: ConnectionPatch) {
  return {
    id,
    ...(patch.baseUrl !== undefined ? { baseUrl: patch.baseUrl.trim() } : {}),
    ...(patch.openwaOrg !== undefined ? { openwaOrg: patch.openwaOrg.trim() } : {}),
    ...(patch.openwaProject !== undefined ? { openwaProject: patch.openwaProject.trim() } : {}),
    ...(patch.deviceId !== undefined ? { deviceId: patch.deviceId.trim() } : {}),
    ...(patch.deviceKey?.trim() ? { deviceKey: patch.deviceKey.trim() } : {}),
    ...(patch.quietMs !== undefined ? { quietWindowMs: patch.quietMs } : {}),
    ...(patch.quietMaxMs !== undefined ? { quietMaxMs: patch.quietMaxMs } : {}),
    ...(patch.replies ? { replyFailure: patch.replies.failure.trim(), replyVoice: patch.replies.voice.trim(), replyPhotoUnreadable: patch.replies.photo.trim(), approvalPrompt: patch.replies.approvalPrompt.trim(), approvalExpired: patch.replies.approvalExpired.trim() } : {}),
    ...(patch.enabled !== undefined ? { enabled: patch.enabled } : {}),
  };
}

export const useChannelsStore = defineStore('channels', () => {
  const { $channelClient: client } = useNuxtApp();
  const byAgent = ref<Record<string, WhatsAppConnection[]>>({});
  const settings = ref<ChannelSettings>({ defaultOpenwaBaseUrl: '', defaultReplies: { failure: '', voice: '', photo: '', approvalPrompt: '', approvalExpired: '' }, timing: { quietMs: 0, quietMaxMs: 0, maxMessageAgeMinutes: 0, maxQuietMs: 0 } });
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  let loadedFor = '';

  function put(c: WhatsAppConnection) {
    const list = byAgent.value[c.agentId] ?? [];
    byAgent.value = { ...byAgent.value, [c.agentId]: list.some(x => x.id === c.id) ? list.map(x => (x.id === c.id ? c : x)) : [...list, c] };
    return c;
  }

  async function load(projectId: string, agentId: string) {
    loading.value = true;
    error.value = null;
    try {
      const [list, s] = await Promise.all([client.listConnections({ projectId, agentId }), client.getChannelSettings({})]);
      byAgent.value = { ...byAgent.value, [agentId]: list.connections.map(toConnection) };
      settings.value = {
        defaultOpenwaBaseUrl: s.defaultOpenwaBaseUrl,
        defaultReplies: { failure: s.replyFailure, voice: s.replyVoice, photo: s.replyPhotoUnreadable, approvalPrompt: s.approvalPrompt, approvalExpired: s.approvalExpired },
        timing: { quietMs: s.quietWindowMs, quietMaxMs: s.quietMaxMs, maxMessageAgeMinutes: s.maxMessageAgeMinutes, maxQuietMs: s.maxQuietMs },
      };
      loadedFor = agentId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_channels');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(projectId: string, agentId: string) {
    return loadedFor === agentId ? Promise.resolve() : load(projectId, agentId);
  }

  // loadProject reads every connection in a project, for the overview's setup steps; a failure leaves the steps unchecked.
  async function loadProject(projectId: string) {
    try {
      const list = (await client.listConnections({ projectId })).connections.map(toConnection);
      byAgent.value = Object.fromEntries(Object.entries(Object.groupBy(list, c => c.agentId)).map(([agent, cs]) => [agent, cs ?? []]));
    } catch {
      byAgent.value = {};
    }
  }

  function list(agentId: string): WhatsAppConnection[] {
    return byAgent.value[agentId] ?? [];
  }

  function connection(agentId: string): WhatsAppConnection | undefined {
    return byAgent.value[agentId]?.find(c => c.provider === 'openwa');
  }

  // replies gives a connection's set replies, with the operator defaults where a text was left empty.
  function replies(agentId: string): ReplyTexts {
    const own = connection(agentId)?.replies;
    const d = settings.value.defaultReplies;
    return { failure: own?.failure || d.failure, voice: own?.voice || d.voice, photo: own?.photo || d.photo, approvalPrompt: own?.approvalPrompt || d.approvalPrompt, approvalExpired: own?.approvalExpired || d.approvalExpired };
  }

  async function create(projectId: string, agentId: string, input: OpenwaInput) {
    const res = await client.createConnection({
      projectId,
      agentId,
      baseUrl: input.baseUrl.trim(),
      openwaOrg: input.openwaOrg.trim(),
      openwaProject: input.openwaProject.trim(),
      deviceId: input.deviceId.trim(),
      deviceKey: input.deviceKey.trim(),
      quietWindowMs: settings.value.timing.quietMs,
      quietMaxMs: settings.value.timing.quietMaxMs,
      replyFailure: settings.value.defaultReplies.failure,
      replyVoice: settings.value.defaultReplies.voice,
      replyPhotoUnreadable: settings.value.defaultReplies.photo,
    });
    return put(toConnection(res.connection!));
  }

  async function update(id: string, patch: ConnectionPatch) {
    return put(toConnection((await client.updateConnection(patchRequest(id, patch))).connection!));
  }

  async function setSecret(id: string, secret: string, dropPrevious = false) {
    return put(toConnection((await client.setConnectionSecret({ id, signingSecret: secret.trim(), dropPrevious })).connection!));
  }

  async function refresh(id: string) {
    return put(toConnection((await client.getConnection({ id })).connection!));
  }

  async function remove(id: string) {
    await client.deleteConnection({ id });
    byAgent.value = Object.fromEntries(Object.entries(byAgent.value).map(([agent, list]) => [agent, list.filter(c => c.id !== id)]));
  }

  return { byAgent, settings, loading, error, load, ensureLoaded, loadProject, list, connection, replies, create, update, setSecret, refresh, remove };
});
