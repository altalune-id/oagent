import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, nextTick } from 'vue';
import { connectionPB, fakeChannelClient } from '~~/test/fixtures/channels';
import { useToast } from '#base/components/ui/toast';
import { ConnectionStatus } from '#gen/channel/v1/channel_pb';
import { connectError, setKnownKeys } from '#test/nuxt-globals';
import { useChannelsStore } from '~/stores/channels';
import { checkPolls, devicePattern, useOpenwaConnect } from './useOpenwaConnect';
import { useReplaceSecret } from './useReplaceSecret';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const agentId = 'A7kPq2WxN4mLt8VcR3bZs';

function fill(form: ReturnType<typeof useOpenwaConnect>, over: Partial<Record<'deviceId' | 'deviceKey', string>> = {}) {
  form.openwaOrg.value = 'altalune';
  form.openwaProject.value = 'rumah';
  form.deviceId.value = over.deviceId ?? 'dev_8fK2pQ7mXr9Lt4Wv';
  form.deviceKey.value = over.deviceKey ?? 'owk_1';
}

beforeEach(() => {
  setActivePinia(createPinia());
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useOpenwaConnect', () => {
  it('takes OpenWA device ids of the form dev_ and 16 characters', () => {
    expect(['dev_8fK2pQ7mXr9Lt4Wv', 'dev_abcdefgh-_123456'].every(d => devicePattern.test(d))).toBe(true);
    expect(['dev_short', 'device_8fK2pQ7mXr9Lt4Wv', 'dev_8fK2pQ7mXr9Lt4Wv0'].some(d => devicePattern.test(d))).toBe(false);
  });

  it('prefills the operator\'s base URL, refuses an incomplete step 2, clears each error on change, then creates without a secret and shows the server\'s webhook URL', async () => {
    const { client } = fakeChannelClient();
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show();
    expect([form.step.value, form.baseUrl.value]).toEqual([1, 'https://openwa.altalune.id']);
    await form.next();
    expect(form.step.value).toBe(2);
    form.baseUrl.value = 'openwa.altalune.id';
    form.deviceId.value = 'dev_short';
    await nextTick();
    await form.save();
    expect(Object.keys(form.errors.value).sort()).toEqual(['baseUrl', 'deviceId', 'deviceKey', 'openwaOrg', 'openwaProject']);
    expect(client.createConnection).not.toHaveBeenCalled();
    form.baseUrl.value = 'https://openwa.altalune.id';
    await nextTick();
    expect(form.errors.value.baseUrl).toBeUndefined();
    fill(form);
    await form.next();
    expect(client.createConnection).toHaveBeenCalledOnce();
    expect(client.createConnection.mock.calls[0]![0]).not.toHaveProperty('signingSecret');
    expect([form.step.value, form.secretSet.value]).toEqual([3, false]);
    expect(form.webhookUrl.value).toMatch(/^https:\/\/oagent\.altalune\.id\/hooks\/openwa\/Ch/);
    expect(form.deviceKey.value).toBe('');
    form.back();
    expect(form.step.value).toBe(2);
  });

  it('pastes the secret OpenWA minted, refusing an empty one, then checks', async () => {
    vi.useFakeTimers();
    const { client, set } = fakeChannelClient();
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show();
    fill(form);
    form.step.value = 2;
    await form.save();
    await form.check();
    expect([form.checking.value, form.result.value]).toEqual([false, null]);
    await form.saveSecret();
    expect(form.errors.value.secret).toBe('openwa.secret_required');
    expect(client.setConnectionSecret).not.toHaveBeenCalled();
    form.secret.value = ' whsec_1 ';
    const id = useChannelsStore().connection(agentId)!.id;
    const done = form.saveSecret();
    await vi.advanceTimersByTimeAsync(0);
    expect(client.setConnectionSecret).toHaveBeenCalledWith({ id, signingSecret: 'whsec_1', dropPrevious: false });
    expect([form.secret.value, form.secretSet.value, form.checking.value]).toEqual(['', true, true]);
    set(id, { status: ConnectionStatus.CONNECTED });
    await vi.advanceTimersByTimeAsync(checkPolls.every);
    await done;
    expect(form.result.value).toBe('connected');
  });

  it('keeps the secret form open when setting it fails', async () => {
    const { client } = fakeChannelClient([connectionPB({ secretSet: false, status: ConnectionStatus.WAITING })]);
    await useChannelsStore().load(projectId, agentId);
    client.setConnectionSecret.mockRejectedValueOnce(new Error('offline'));
    const form = useOpenwaConnect(projectId, agentId);
    form.show(true);
    form.secret.value = 'whsec_1';
    await form.saveSecret();
    expect([form.formError.value?.message, form.checking.value, form.secretSet.value]).toEqual(['error.save_channel', false, false]);
  });

  it('names a server without a public URL', async () => {
    const { client } = fakeChannelClient();
    await useChannelsStore().load(projectId, agentId);
    setKnownKeys('apperror.CHN006');
    client.createConnection.mockRejectedValueOnce(connectError('CHN006'));
    const form = useOpenwaConnect(projectId, agentId);
    form.show();
    fill(form);
    form.step.value = 2;
    await form.save();
    expect(form.formError.value?.message).toBe('apperror.CHN006');
  });

  it('sends an http address and leaves the policy to the server, naming its refusal on the field', async () => {
    const { client } = fakeChannelClient();
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show();
    fill(form);
    form.baseUrl.value = 'http://10.0.0.5:8080';
    form.step.value = 2;
    client.createConnection.mockRejectedValueOnce(connectError('CHN002', { field: 'base_url' }));
    await form.save();
    expect(client.createConnection).toHaveBeenCalledWith(expect.objectContaining({ baseUrl: 'http://10.0.0.5:8080' }));
    expect([form.errors.value.baseUrl, form.formError.value, form.step.value]).toEqual(['openwa.base_url_refused', null, 2]);
    await form.save();
    expect(form.step.value).toBe(3);
    expect(useChannelsStore().connection(agentId)?.baseUrl).toBe('http://10.0.0.5:8080');
  });

  it('shows the server refusal and stays on step 2', async () => {
    const { client } = fakeChannelClient();
    await useChannelsStore().load(projectId, agentId);
    client.createConnection.mockRejectedValueOnce(new Error('offline'));
    const form = useOpenwaConnect(projectId, agentId);
    form.show();
    fill(form);
    form.step.value = 2;
    await form.save();
    expect(form.formError.value?.message).toBe('error.save_channel');
    expect(form.step.value).toBe(2);
  });

  it('edits an existing connection keeping its key unless the URL or device changes', async () => {
    const { client } = fakeChannelClient([connectionPB()]);
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show(true);
    expect(form.step.value).toBe(3);
    expect([form.openwaOrg.value, form.deviceId.value, form.keyNote.value]).toEqual(['altalune', 'dev_8fK2pQ7mXr9Lt4Wv', 'openwa.key_keep']);
    form.openwaProject.value = 'toko';
    await form.save();
    expect(client.updateConnection).toHaveBeenCalledWith(expect.objectContaining({ openwaProject: 'toko' }));
    expect(client.updateConnection.mock.calls[0]![0]).not.toHaveProperty('deviceKey');
    form.deviceId.value = 'dev_newnewnewnewnew1';
    expect(form.keyNote.value).toBe('openwa.key_reenter');
    await form.save();
    expect(Object.keys(form.errors.value)).toEqual(['deviceKey']);
    fill(form, { deviceId: 'dev_newnewnewnewnew1' });
    await form.save();
    expect(client.updateConnection).toHaveBeenLastCalledWith(expect.objectContaining({ deviceId: 'dev_newnewnewnewnew1', deviceKey: 'owk_1' }));
    expect(client.setConnectionSecret).not.toHaveBeenCalled();
  });

  it('checks until the status leaves waiting, notices a new bad signature, and gives up after the last poll', async () => {
    vi.useFakeTimers();
    const { set } = fakeChannelClient([connectionPB({ status: ConnectionStatus.WAITING, lastResultAt: undefined })]);
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show(true);
    const connected = form.check();
    expect(form.checking.value).toBe(true);
    await vi.advanceTimersByTimeAsync(checkPolls.every);
    set('Ch8fK2pQ7mXr9Lt4WvN3c', { status: ConnectionStatus.CONNECTED });
    await vi.advanceTimersByTimeAsync(checkPolls.every);
    await connected;
    expect([form.result.value, form.checking.value]).toEqual(['connected', false]);

    set('Ch8fK2pQ7mXr9Lt4WvN3c', { status: ConnectionStatus.WAITING });
    await useChannelsStore().refresh('Ch8fK2pQ7mXr9Lt4WvN3c');
    const bad = form.check();
    set('Ch8fK2pQ7mXr9Lt4WvN3c', { lastBadSignatureAt: timestampFromDate(new Date()) });
    await vi.advanceTimersByTimeAsync(checkPolls.every);
    await bad;
    expect(form.result.value).toBe('bad_signature');

    const quiet = form.check();
    await vi.advanceTimersByTimeAsync(checkPolls.every * checkPolls.times);
    await quiet;
    expect(form.result.value).toBe('waiting');
  });

  it('stops checking when closed', async () => {
    vi.useFakeTimers();
    fakeChannelClient([connectionPB({ status: ConnectionStatus.WAITING })]);
    await useChannelsStore().load(projectId, agentId);
    const form = useOpenwaConnect(projectId, agentId);
    form.show(true);
    const pending = form.check();
    form.hide();
    await vi.advanceTimersByTimeAsync(checkPolls.every * 2);
    await pending;
    expect([form.result.value, form.checking.value, form.open.value]).toEqual([null, false, false]);
  });
});

describe('useOpenwaConnect scope', () => {
  it('stops polling when its component goes away mid-check', async () => {
    vi.useFakeTimers();
    const { client } = fakeChannelClient([connectionPB({ status: ConnectionStatus.WAITING })]);
    await useChannelsStore().load(projectId, agentId);
    const scope = effectScope();
    const form = scope.run(() => useOpenwaConnect(projectId, agentId))!;
    form.show(true);
    const pending = form.check();
    await vi.advanceTimersByTimeAsync(0);
    scope.stop();
    const calls = client.getConnection.mock.calls.length;
    await vi.advanceTimersByTimeAsync(checkPolls.every * 3);
    await pending;
    expect(client.getConnection.mock.calls.length).toBe(calls);
    expect(form.checking.value).toBe(false);
  });
});

describe('useReplaceSecret', () => {
  it('sets a new secret, keeping or dropping the old one, or replaces the key, refusing an empty value and clearing on close', async () => {
    const { client } = fakeChannelClient([connectionPB()]);
    await useChannelsStore().load(projectId, agentId);
    const replace = useReplaceSecret(() => 'Ch8fK2pQ7mXr9Lt4WvN3c');
    replace.show('secret');
    expect(await replace.save()).toBe(false);
    expect(replace.error.value).toBe('openwa.secret_required');
    replace.value.value = 'whsec_new';
    expect(await replace.save()).toBe(true);
    expect(client.setConnectionSecret).toHaveBeenCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', signingSecret: 'whsec_new', dropPrevious: false });
    expect(useToast().toasts.at(-1)?.text).toBe('openwa.secret_replaced');
    replace.show('secret');
    expect(replace.dropPrevious.value).toBe(false);
    replace.dropPrevious.value = true;
    replace.value.value = 'whsec_newer';
    expect(await replace.save()).toBe(true);
    expect(client.setConnectionSecret).toHaveBeenLastCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', signingSecret: 'whsec_newer', dropPrevious: true });
    expect(useChannelsStore().connection(agentId)?.secondarySet).toBe(false);
    replace.show('key');
    replace.value.value = 'owk_new';
    client.updateConnection.mockRejectedValueOnce(new Error('offline'));
    expect(await replace.save()).toBe(false);
    expect(replace.error.value).toBe('error.save_channel');
    expect(await replace.save()).toBe(true);
    expect(client.updateConnection).toHaveBeenLastCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', deviceKey: 'owk_new' });
    expect([replace.kind.value, replace.value.value]).toEqual([null, '']);
    expect(await useReplaceSecret(() => undefined).save()).toBe(false);
  });
});
