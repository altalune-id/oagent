import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { channelDefaults, connectionPB, fakeChannelClient } from '~~/test/fixtures/channels';
import { ConnectionStatus } from '#gen/channel/v1/channel_pb';
import { patchRequest, toConnection, useChannelsStore } from './channels';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const agentId = 'A7kPq2WxN4mLt8VcR3bZs';
const input = { baseUrl: ' https://openwa.altalune.id ', openwaOrg: ' altalune ', openwaProject: ' rumah ', deviceId: ' dev_8fK2pQ7mXr9Lt4Wv ', deviceKey: ' owk_def ' };

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('channel wire mapping', () => {
  it('maps every status, with a bad signature as a separate time, never a status', () => {
    expect([ConnectionStatus.UNSPECIFIED, ConnectionStatus.WAITING, ConnectionStatus.CONNECTED, ConnectionStatus.DEVICE_MISMATCH, ConnectionStatus.KEY_REFUSED].map(status => toConnection(connectionPB({ status })).status))
      .toEqual(['waiting', 'waiting', 'connected', 'device_mismatch', 'key_refused']);
    const c = toConnection(connectionPB({ lastBadSignatureAt: timestampFromDate(new Date('2026-11-07T09:30:00Z')), secondarySet: true }));
    expect(c).toMatchObject({ status: 'connected', secondarySet: true, openwaOrg: 'altalune', quietMs: 3_000, replies: { failure: channelDefaults.replyFailure } });
    expect(c.lastBadSignatureAt?.toISOString()).toBe('2026-11-07T09:30:00.000Z');
  });

  it('sends only what changed in an update, trimmed, and a device key only when one was typed', () => {
    expect(patchRequest('C1', { quietMs: 2_500 })).toEqual({ id: 'C1', quietWindowMs: 2_500 });
    expect(patchRequest('C1', { deviceKey: '  ' })).toEqual({ id: 'C1' });
    expect(patchRequest('C1', { baseUrl: ' https://x.id ', deviceKey: ' k ', replies: { failure: ' a ', voice: 'b', photo: 'c' }, enabled: false }))
      .toEqual({ id: 'C1', baseUrl: 'https://x.id', deviceKey: 'k', replyFailure: 'a', replyVoice: 'b', replyPhotoUnreadable: 'c', enabled: false });
  });
});

describe('channels store', () => {
  it('loads an agent\'s connections and the operator defaults', async () => {
    const { client } = fakeChannelClient([connectionPB()]);
    const store = useChannelsStore();
    await store.ensureLoaded(projectId, agentId);
    await store.ensureLoaded(projectId, agentId);
    expect(client.listConnections).toHaveBeenCalledOnce();
    expect(client.listConnections).toHaveBeenCalledWith({ projectId, agentId });
    expect(store.connection(agentId)?.deviceId).toBe('dev_8fK2pQ7mXr9Lt4Wv');
    expect(store.settings).toEqual({ defaultOpenwaBaseUrl: 'https://openwa.altalune.id', defaultReplies: { failure: channelDefaults.replyFailure, voice: channelDefaults.replyVoice, photo: channelDefaults.replyPhotoUnreadable }, timing: { quietMs: 3_000, quietMaxMs: 10_000, maxMessageAgeMinutes: 30 } });
  });

  it('creates with the key trimmed, no secret and the operator defaults, and keeps no key', async () => {
    const { client } = fakeChannelClient();
    const store = useChannelsStore();
    await store.load(projectId, agentId);
    const c = await store.create(projectId, agentId, input);
    expect(client.createConnection).toHaveBeenCalledWith({ projectId, agentId, baseUrl: 'https://openwa.altalune.id', openwaOrg: 'altalune', openwaProject: 'rumah', deviceId: 'dev_8fK2pQ7mXr9Lt4Wv', deviceKey: 'owk_def', quietWindowMs: 3_000, quietMaxMs: 10_000, replyFailure: channelDefaults.replyFailure, replyVoice: channelDefaults.replyVoice, replyPhotoUnreadable: channelDefaults.replyPhotoUnreadable });
    expect(c).toMatchObject({ status: 'waiting', secretSet: false, webhookUrl: `https://oagent.altalune.id/hooks/openwa/${c.id}` });
    expect(JSON.stringify(store.$state)).not.toMatch(/owk_def/);
  });

  it('updates, sets the secret keeping or dropping the old one, refreshes and removes', async () => {
    const { client, set } = fakeChannelClient([connectionPB()]);
    const store = useChannelsStore();
    await store.load(projectId, agentId);
    const id = store.connection(agentId)!.id;
    await store.update(id, { quietMs: 1_500, quietMaxMs: 6_000 });
    expect(store.connection(agentId)).toMatchObject({ quietMs: 1_500, quietMaxMs: 6_000 });
    await store.setSecret(id, ' whsec_new ');
    expect(client.setConnectionSecret).toHaveBeenCalledWith({ id, signingSecret: 'whsec_new', dropPrevious: false });
    expect(store.connection(agentId)).toMatchObject({ secretSet: true, secondarySet: true });
    await store.setSecret(id, 'whsec_newer', true);
    expect(client.setConnectionSecret).toHaveBeenLastCalledWith({ id, signingSecret: 'whsec_newer', dropPrevious: true });
    expect(store.connection(agentId)?.secondarySet).toBe(false);
    expect(JSON.stringify(store.$state)).not.toMatch(/whsec_/);
    set(id, { status: ConnectionStatus.KEY_REFUSED });
    expect((await store.refresh(id)).status).toBe('key_refused');
    await store.remove(id);
    expect(store.connection(agentId)).toBeUndefined();
  });

  it('falls back to the operator default for an empty set reply', async () => {
    fakeChannelClient([connectionPB({ replyVoice: '' })]);
    const store = useChannelsStore();
    await store.load(projectId, agentId);
    expect(store.replies(agentId)).toEqual({ failure: channelDefaults.replyFailure, voice: channelDefaults.replyVoice, photo: channelDefaults.replyPhotoUnreadable });
    expect(store.replies('other')).toEqual({ failure: channelDefaults.replyFailure, voice: channelDefaults.replyVoice, photo: channelDefaults.replyPhotoUnreadable });
  });

  it('loads a whole project for the overview, and shows a load error', async () => {
    const { client } = fakeChannelClient([connectionPB(), connectionPB({ id: 'Ch2', agentId: 'B9' })]);
    const store = useChannelsStore();
    await store.loadProject(projectId);
    expect(store.list(agentId)).toHaveLength(1);
    expect(store.list('B9')).toHaveLength(1);
    expect(store.list('none')).toEqual([]);
    client.listConnections.mockRejectedValue(new Error('offline'));
    await store.loadProject(projectId);
    expect(store.list(agentId)).toEqual([]);
    await store.load(projectId, agentId);
    expect(store.error?.message).toBe('error.load_channels');
    expect(store.loading).toBe(false);
    vi.restoreAllMocks();
  });
});
