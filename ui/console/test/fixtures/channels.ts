import type { MessageInitShape } from '@bufbuild/protobuf';
import type { Connection } from '#gen/channel/v1/channel_pb';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { vi } from 'vitest';
import { ConnectionSchema, ConnectionStatus } from '#gen/channel/v1/channel_pb';
import { connectError, setClient } from '#test/nuxt-globals';

export const channelDefaults = {
  defaultOpenwaBaseUrl: 'https://openwa.altalune.id',
  replyFailure: 'Sorry, something went wrong on our side. Please try again in a moment.',
  replyVoice: 'Voice notes are not supported yet. Please send your message as text.',
  replyPhotoUnreadable: 'I could not read that photo. Please describe it in text instead.',
  quietWindowMs: 3_000,
  quietMaxMs: 10_000,
  maxMessageAgeMinutes: 30,
};

export function connectionPB(over: MessageInitShape<typeof ConnectionSchema> = {}): Connection {
  return create(ConnectionSchema, {
    id: 'Ch8fK2pQ7mXr9Lt4WvN3c',
    orgId: 'B6rTn1YkM8wQz3XpL5vGd',
    projectId: 'x6MH9WPWjktMwCBKHy4pF',
    agentId: 'A7kPq2WxN4mLt8VcR3bZs',
    provider: 'openwa',
    enabled: true,
    status: ConnectionStatus.CONNECTED,
    baseUrl: 'https://openwa.altalune.id',
    openwaOrg: 'altalune',
    openwaProject: 'rumah',
    deviceId: 'dev_8fK2pQ7mXr9Lt4Wv',
    webhookUrl: 'https://oagent.altalune.id/hooks/openwa/Ch8fK2pQ7mXr9Lt4WvN3c',
    secretSet: true,
    quietWindowMs: 3_000,
    quietMaxMs: 10_000,
    replyFailure: channelDefaults.replyFailure,
    replyVoice: channelDefaults.replyVoice,
    replyPhotoUnreadable: channelDefaults.replyPhotoUnreadable,
    lastResultAt: timestampFromDate(new Date('2026-11-07T09:00:00Z')),
    ...over,
  });
}

// fakeChannelClient is an in-memory ChannelService: update takes only the changes but refuses an empty address field and a new URL or device without a key, like the server; create starts waiting without a secret and with a server webhook URL, setting a secret keeps the old one as secondary unless dropped, and secrets are never stored.
export function fakeChannelClient(seed: Connection[] = []) {
  const held = new Map(seed.map(c => [c.id, c]));
  let seq = 0;
  const found = (id: string) => {
    const c = held.get(id);
    if (!c) {
      throw connectError('CHN001');
    }
    return c;
  };
  const client = {
    listConnections: vi.fn(async ({ projectId, agentId }: { projectId: string, agentId?: string }) => ({ connections: [...held.values()].filter(c => c.projectId === projectId && (!agentId || c.agentId === agentId)) })),
    getChannelSettings: vi.fn(async () => ({ ...channelDefaults })),
    getConnection: vi.fn(async ({ id }: { id: string }) => ({ connection: found(id) })),
    createConnection: vi.fn(async (req: Record<string, unknown>) => {
      const id = `Ch${String(++seq).padStart(19, '0')}`;
      const c = connectionPB({
        id,
        projectId: req.projectId as string,
        agentId: req.agentId as string,
        status: ConnectionStatus.WAITING,
        baseUrl: req.baseUrl as string,
        openwaOrg: req.openwaOrg as string,
        openwaProject: req.openwaProject as string,
        deviceId: req.deviceId as string,
        webhookUrl: `https://oagent.altalune.id/hooks/openwa/${id}`,
        secretSet: false,
        quietWindowMs: req.quietWindowMs as number,
        quietMaxMs: req.quietMaxMs as number,
        replyFailure: req.replyFailure as string,
        replyVoice: req.replyVoice as string,
        replyPhotoUnreadable: req.replyPhotoUnreadable as string,
        lastResultAt: undefined,
      });
      held.set(id, c);
      return { connection: c };
    }),
    updateConnection: vi.fn(async (req: Record<string, unknown> & { id: string }) => {
      const was = found(req.id);
      if (['baseUrl', 'openwaOrg', 'openwaProject', 'deviceId'].some(f => req[f] === '')) {
        throw connectError('GEN004');
      }
      if (((req.baseUrl && req.baseUrl !== was.baseUrl) || (req.deviceId && req.deviceId !== was.deviceId)) && !req.deviceKey) {
        throw connectError('CHN003');
      }
      const { id, deviceKey: _key, ...rest } = req;
      const c = create(ConnectionSchema, { ...found(id), ...rest });
      held.set(id, c);
      return { connection: c };
    }),
    setConnectionSecret: vi.fn(async ({ id, dropPrevious }: { id: string, signingSecret: string, dropPrevious: boolean }) => {
      const was = found(id);
      const c = create(ConnectionSchema, { ...was, secretSet: true, secondarySet: was.secretSet && !dropPrevious });
      held.set(id, c);
      return { connection: c };
    }),
    deleteConnection: vi.fn(async ({ id }: { id: string }) => {
      found(id);
      held.delete(id);
      return {};
    }),
  };
  setClient('$channelClient', client);
  return { client, held, set: (id: string, over: Partial<Connection>) => held.set(id, create(ConnectionSchema, { ...found(id), ...over })) };
}
