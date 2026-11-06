import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { useToast } from '#base/components/ui/toast';
import { sampleOpenwaCheck } from '~/preview/samples';
import { useChannelsStore } from '~/stores/channels';
import { useOpenwaConnect } from './useOpenwaConnect';
import { useReplaceSecret } from './useReplaceSecret';

const agentId = 'Y4sKu8RmT2pLq6WnX9cVb';

function fill(form: ReturnType<typeof useOpenwaConnect>, deviceId = 'dev_yasaku-rumah', secret = 'whsec_1') {
  form.baseUrl.value = 'https://openwa.example.id';
  form.deviceId.value = deviceId;
  form.secret.value = secret;
  form.deviceKey.value = 'owk_1';
}

const bad = { id: 'chn_1', provider: 'openwa' as const, status: 'bad_signature' as const, baseUrl: '', deviceId: 'd', webhookUrl: '', rejected24h: 3, rejectedLastHour: 3 };

describe('useOpenwaConnect', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('prefills the default base URL, refuses step 2 until filled, clears each error on change, then connects', async () => {
    await useChannelsStore().ensureLoaded(agentId);
    const check = vi.fn().mockResolvedValue('connected');
    const form = useOpenwaConnect(agentId, check);
    form.show();
    expect(form.baseUrl.value).toBe('https://openwa.altalune.id');
    expect(form.step.value).toBe(1);
    expect(form.webhookUrl.value).toBe('https://oagent.altalune.id/hooks/openwa/chn_8fK2pQ');
    expect([form.deviceId.value, form.secret.value, form.deviceKey.value]).toEqual(['dev_yasaku-rumah', 'sample-webhook-secret', 'sample-device-key']);
    form.baseUrl.value = '';
    form.deviceId.value = '';
    form.secret.value = '';
    form.deviceKey.value = '';
    form.next();
    form.next();
    expect(form.step.value).toBe(2);
    expect(form.errors.value).toEqual({ baseUrl: 'openwa.base_url_https', deviceId: 'openwa.device_required', secret: 'openwa.secret_required', deviceKey: 'openwa.key_required' });
    form.baseUrl.value = 'https://openwa.example.id';
    await nextTick();
    expect(form.errors.value.baseUrl).toBeUndefined();
    fill(form);
    await nextTick();
    expect(form.errors.value).toEqual({});
    form.next();
    expect(form.step.value).toBe(3);
    form.back();
    form.next();
    await form.saveAndTest();
    expect(check).toHaveBeenCalledWith({ deviceId: 'dev_yasaku-rumah', secret: 'whsec_1' });
    expect(form.result.value).toBe('connected');
    expect([form.secret.value, form.deviceKey.value]).toEqual(['', '']);
    expect(useChannelsStore().connection(agentId)).toMatchObject({ provider: 'openwa', status: 'connected', deviceId: 'dev_yasaku-rumah', rejected24h: 0 });
    form.hide();
    expect(form.open.value).toBe(false);
  });

  it('never saves when the modal closes while the check runs', async () => {
    let answer: (v: string) => void = () => {};
    const form = useOpenwaConnect(agentId, () => new Promise((resolve) => {
      answer = resolve as (v: string) => void;
    }));
    form.show();
    fill(form);
    form.step.value = 3;
    const run = form.saveAndTest();
    expect(form.checking.value).toBe(true);
    form.hide();
    answer('connected');
    await run;
    expect(useChannelsStore().connection(agentId)).toBeUndefined();
    expect(form.checking.value).toBe(false);
  });

  it('stores each failure, keeps the saved id for the webhook, and finishes later as waiting', async () => {
    const check = vi.fn().mockResolvedValue('bad_signature');
    const form = useOpenwaConnect(agentId, check);
    form.show();
    form.baseUrl.value = '';
    form.step.value = 3;
    await form.saveAndTest();
    expect(form.step.value).toBe(2);
    expect(check).not.toHaveBeenCalled();
    fill(form);
    form.step.value = 3;
    await form.saveAndTest();
    expect(useChannelsStore().connection(agentId)).toMatchObject({ status: 'bad_signature', rejectedLastHour: 3, lastMessageAt: undefined });
    useChannelsStore().save(agentId, { ...useChannelsStore().connection(agentId)!, id: 'chn_saved' });
    form.show(true);
    expect(form.step.value).toBe(3);
    expect(form.webhookUrl.value).toBe('https://oagent.altalune.id/hooks/openwa/chn_saved');
    expect(form.baseUrl.value).toBe('https://openwa.example.id');
    form.finishLater();
    expect(useChannelsStore().connection(agentId)).toMatchObject({ status: 'waiting' });
    expect(form.open.value).toBe(false);
    form.show();
    form.baseUrl.value = '';
    form.finishLater();
    expect(form.open.value).toBe(true);
  });

  it('asks for the secret and device key again when the base URL or device changes on a saved connection', async () => {
    const check = vi.fn().mockResolvedValue('connected');
    const form = useOpenwaConnect(agentId, check);
    form.show();
    fill(form);
    await form.saveAndTest();
    form.show(true);
    expect(form.secretsNote.value).toBe('openwa.secrets_keep');
    form.step.value = 3;
    await form.saveAndTest();
    expect(check).toHaveBeenCalledTimes(2);
    form.show();
    form.deviceId.value = 'dev_rumah-baru';
    expect(form.secretsNote.value).toBe('openwa.secrets_reenter');
    form.step.value = 3;
    await form.saveAndTest();
    expect(form.step.value).toBe(2);
    expect(form.errors.value).toEqual({ secret: 'openwa.secret_required', deviceKey: 'openwa.key_required' });
    form.deviceId.value = 'dev_yasaku-rumah';
    form.baseUrl.value = 'https://openwa2.example.id';
    expect(form.needsSecrets.value).toBe(true);
    form.secret.value = 'whsec_2';
    form.deviceKey.value = 'owk_2';
    await form.saveAndTest();
    expect(check).toHaveBeenLastCalledWith({ deviceId: 'dev_yasaku-rumah', secret: 'whsec_2' });
    expect(useChannelsStore().connection(agentId)!.baseUrl).toBe('https://openwa2.example.id');
  });

  it('leaves the base URL empty when there is no default', () => {
    useChannelsStore().settings.defaultOpenwaBaseUrl = '';
    const form = useOpenwaConnect(agentId, vi.fn());
    form.show();
    expect(form.baseUrl.value).toBe('');
  });

  it('answers the sample check by what was typed', async () => {
    expect(await sampleOpenwaCheck({ deviceId: 'dev_403', secret: 'a' }, 0)).toBe('key_refused');
    expect(await sampleOpenwaCheck({ deviceId: 'dev_lain', secret: 'a' }, 0)).toBe('device_mismatch');
    expect(await sampleOpenwaCheck({ deviceId: 'dev_diam', secret: 'a' }, 0)).toBe('no_message');
    expect(await sampleOpenwaCheck({ deviceId: 'dev_ok', secret: 'salah' }, 0)).toBe('bad_signature');
    expect(await sampleOpenwaCheck({ deviceId: 'dev_ok', secret: 'a' }, 0)).toBe('connected');
  });
});

describe('useReplaceSecret', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('refuses an empty value, clears it on close, and marks the connection fixed', async () => {
    useChannelsStore().save(agentId, bad);
    const replace = useReplaceSecret(agentId);
    replace.show('secret');
    expect(replace.save()).toBe(false);
    expect(replace.error.value).toBe('openwa.secret_required');
    replace.value.value = 'whsec_2';
    await nextTick();
    expect(replace.error.value).toBe('');
    replace.hide();
    expect([replace.kind.value, replace.value.value]).toEqual([null, '']);
    replace.show('key');
    expect(replace.save()).toBe(false);
    expect(replace.error.value).toBe('openwa.key_required');
    replace.value.value = 'owk_2';
    expect(replace.save()).toBe(true);
    expect(useChannelsStore().connection(agentId)!.status).toBe('connected');
    expect(useToast().toasts.at(-1)?.text).toBe('openwa.key_replaced');
    replace.show('secret');
    replace.value.value = 's';
    replace.save();
    expect(useToast().toasts.at(-1)?.text).toBe('openwa.secret_replaced');
  });
});

describe('useChannelsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('keeps one connection per provider, lists them, marks one fixed and removes it', () => {
    const store = useChannelsStore();
    store.replaced(agentId);
    expect(store.connection(agentId)).toBeUndefined();
    store.save(agentId, bad);
    store.save(agentId, { ...bad, id: 'chn_wb', provider: 'whatsapp_business' });
    store.save('other', bad);
    expect(store.list(agentId).map(c => c.provider)).toEqual(['openwa', 'whatsapp_business']);
    store.replaced(agentId);
    expect(store.connection(agentId)).toMatchObject({ status: 'connected', rejectedLastHour: 0, rejected24h: 3 });
    store.remove(agentId);
    expect(store.list(agentId).map(c => c.provider)).toEqual(['whatsapp_business']);
  });
});
