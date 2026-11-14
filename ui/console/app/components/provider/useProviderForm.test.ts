import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { fakeRuntimeClients, providerPBs } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { connectError, setKnownKeys } from '#test/nuxt-globals';
import { useModelProvidersStore } from '~/stores/modelProviders';
import { ANTHROPIC_URL, flagLocks, hasMorePrices, hasPrice, hasTranscriptionRow, isTranscriptionRow, kindKey, toPrices, useProviderForm } from './useProviderForm';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const blank = { input: '', output: '', cacheRead: '', cacheWrite: '' };

describe('provider helpers', () => {
  it('has a price only with prices', () => {
    expect(hasPrice({ id: 'a', vision: false, embedding: false, transcription: false, prices: { inputMicros: 0n, outputMicros: 1n, cacheReadMicros: 0n, cacheWriteMicros: 0n } })).toBe(true);
    expect(hasPrice({ id: 'a', vision: false, embedding: false })).toBe(false);
  });

  it('names each kind', () => {
    expect(kindKey(ModelProviderKind.ANTHROPIC)).toBe('providers.kind_anthropic');
    expect(kindKey(ModelProviderKind.OPENAI_COMPATIBLE)).toBe('providers.kind_openai_compatible');
  });

  it('opens More prices only for a row with a cache price', () => {
    const prices = { ...blank, input: '1', output: '2' };
    expect(hasMorePrices({ key: 1, id: 'a', vision: false, embedding: false, transcription: false, audioPrice: '', prices })).toBe(false);
    expect(hasMorePrices({ key: 1, id: 'a', vision: false, embedding: false, transcription: false, audioPrice: '', prices: { ...prices, cacheWrite: '0.5' } })).toBe(true);
  });

  it('reads USD per 1M tokens into micro-USD: all empty is unpriced, an empty cache price is zero', () => {
    expect(toPrices(blank)).toEqual({ kind: 'unpriced' });
    expect(toPrices({ ...blank, input: '0.4', output: '$1.6' })).toEqual({ kind: 'priced', prices: { inputMicros: 400_000n, outputMicros: 1_600_000n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
    expect(toPrices({ ...blank, input: '0.4' })).toEqual({ kind: 'invalid' });
    expect(toPrices({ ...blank, input: '0.02' }, true)).toEqual({ kind: 'priced', prices: { inputMicros: 20_000n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
    expect(toPrices({ ...blank, input: '0.4', output: '1', cacheRead: '-1' })).toEqual({ kind: 'invalid' });
  });
});

describe('useProviderForm', () => {
  let clients: ReturnType<typeof fakeRuntimeClients>;

  beforeEach(async () => {
    setActivePinia(createPinia());
    clients = fakeRuntimeClients();
    await useModelProvidersStore().load(projectId);
  });

  it('fills the Anthropic URL only when empty and clears it on switch back only while it holds that default', () => {
    const form = useProviderForm(orgId, projectId);
    form.show();
    expect(form.kind.value).toBe(ModelProviderKind.OPENAI_COMPATIBLE);
    form.setKind(ModelProviderKind.ANTHROPIC);
    expect(form.baseUrl.value).toBe(ANTHROPIC_URL);
    form.setKind(ModelProviderKind.OPENAI_COMPATIBLE);
    expect(form.baseUrl.value).toBe('');

    form.baseUrl.value = 'https://proxy.example.com';
    form.setKind(ModelProviderKind.ANTHROPIC);
    expect(form.baseUrl.value).toBe('https://proxy.example.com');
    form.baseUrl.value = 'https://api.anthropic.com/v1';
    form.setKind(ModelProviderKind.OPENAI_COMPATIBLE);
    expect(form.baseUrl.value).toBe('https://api.anthropic.com/v1');
  });

  it('warns about a trailing /v1 only for Anthropic', () => {
    const form = useProviderForm(orgId, projectId);
    form.show();
    form.baseUrl.value = 'https://openrouter.ai/api/v1';
    expect(form.urlV1.value).toBe(false);
    form.setKind(ModelProviderKind.ANTHROPIC);
    expect(form.urlV1.value).toBe(true);
    form.baseUrl.value = ' https://api.anthropic.com/v1/ ';
    expect(form.urlV1.value).toBe(true);
    form.baseUrl.value = 'https://api.anthropic.com/v1beta';
    expect(form.urlV1.value).toBe(false);
  });

  it('keeps the stored kind on edit and sends it back', async () => {
    const store = useModelProvidersStore();
    const anthropic = store.items.find(p => p.kind === ModelProviderKind.ANTHROPIC)!;
    clients.provider.updateModelProvider.mockResolvedValue({ modelProvider: providerPBs()[2] });
    const form = useProviderForm(orgId, projectId);
    form.show(anthropic);
    expect(form.kind.value).toBe(ModelProviderKind.ANTHROPIC);
    form.setKind(ModelProviderKind.OPENAI_COMPATIBLE);
    expect(form.kind.value).toBe(ModelProviderKind.ANTHROPIC);
    expect(await form.submit()).toBe(true);
    expect(clients.provider.updateModelProvider).toHaveBeenCalledWith(expect.objectContaining({ kind: ModelProviderKind.ANTHROPIC }));
  });

  it('sends a transcription model back with its flag and audio price untouched, and other rows as before', async () => {
    const store = useModelProvidersStore();
    const base = store.items[0]!;
    const audio = { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n, audioMinuteMicros: 6_000n };
    const withStt = { ...base, models: [...base.models, { id: 'whisper-1', vision: false, embedding: false, transcription: true, prices: audio }] };
    clients.provider.updateModelProvider.mockResolvedValue({ modelProvider: providerPBs()[0] });
    const form = useProviderForm(orgId, projectId);
    form.show(withStt);
    form.name.value = 'Renamed';
    expect(await form.submit()).toBe(true);
    const sent = clients.provider.updateModelProvider.mock.calls[0]![0].models;
    expect(sent.at(-1)).toMatchObject({ id: 'whisper-1', transcription: true, prices: { audioMinuteMicros: 6_000n } });
    const plain = useProviderForm(orgId, projectId);
    plain.show(base);
    plain.name.value = 'Renamed';
    clients.provider.updateModelProvider.mockClear();
    expect(await plain.submit()).toBe(true);
    expect(sent.slice(0, -1)).toEqual(clients.provider.updateModelProvider.mock.calls[0]![0].models);
  });

  it('makes Transcription exclusive with Vision and Embedding, clearing them when it is turned on', () => {
    const form = useProviderForm(orgId, projectId);
    form.show();
    const row = form.models.value[0]!;
    expect(flagLocks(row, false)).toEqual({ vision: false, embedding: false, transcription: false });
    row.vision = true;
    expect(flagLocks(row, false)).toEqual({ vision: false, embedding: false, transcription: true });
    row.vision = false;
    row.embedding = true;
    expect(flagLocks(row, false).transcription).toBe(true);
    row.vision = true;
    form.setTranscription(row, true);
    expect([row.transcription, row.vision, row.embedding]).toEqual([true, false, false]);
    expect(flagLocks(row, false)).toEqual({ vision: true, embedding: true, transcription: false });
    expect(hasTranscriptionRow(form.models.value, false)).toBe(true);
    expect(hasTranscriptionRow(form.models.value, true)).toBe(false);
    form.setTranscription(row, false);
    expect(flagLocks(row, false)).toEqual({ vision: false, embedding: false, transcription: false });
    expect(hasTranscriptionRow(form.models.value, false)).toBe(false);
  });

  it('unlocks Vision once a ticked Transcription is hidden by switching to Anthropic, and saves Vision as ticked', async () => {
    clients.provider.createModelProvider.mockResolvedValue({ modelProvider: providerPBs()[2] });
    const form = useProviderForm(orgId, projectId);
    form.show();
    const row = form.models.value[0]!;
    form.setTranscription(row, true);
    form.setKind(ModelProviderKind.ANTHROPIC);
    expect(flagLocks(row, form.anthropic.value).vision).toBe(false);
    row.vision = true;
    row.id = 'claude';
    row.prices.input = '3';
    row.prices.output = '15';
    form.name.value = 'Claude';
    form.key.value = 'k';
    expect(await form.submit()).toBe(true);
    expect(clients.provider.createModelProvider.mock.calls[0]![0].models[0]).toMatchObject({ id: 'claude', vision: true, transcription: false });
  });

  it('prices a transcription model per minute only: required, zero or more, at most six decimals, sent with zero token prices', async () => {
    clients.provider.createModelProvider.mockResolvedValue({ modelProvider: providerPBs()[0] });
    const form = useProviderForm(orgId, projectId);
    form.show();
    form.name.value = 'STT';
    form.baseUrl.value = 'https://api.openai.com/v1';
    form.key.value = 'k';
    const row = form.models.value[0]!;
    row.id = 'whisper-1';
    row.prices.input = '9';
    row.prices.output = 'junk';
    form.setTranscription(row, true);
    expect(isTranscriptionRow(row, form.anthropic.value)).toBe(true);
    for (const bad of ['', '-1', '0.0000001', 'abc']) {
      row.audioPrice = bad;
      expect(await form.submit()).toBe(false);
      expect(form.modelErrors.value).toEqual({ [row.key]: 'providers.price_audio_minute_invalid' });
      expect(form.priceErrors.value).toEqual([row.key]);
    }
    form.setTranscription(row, false);
    expect(form.priceErrors.value).toEqual([]);
    expect(form.modelErrors.value).toEqual({});
    form.setTranscription(row, true);
    row.audioPrice = '0';
    expect(await form.submit()).toBe(true);
    expect(clients.provider.createModelProvider.mock.calls[0]![0].models[0]).toMatchObject({ id: 'whisper-1', vision: false, embedding: false, transcription: true, prices: { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n, audioMinuteMicros: 0n } });
    form.show();
    form.name.value = 'STT';
    form.baseUrl.value = 'https://api.openai.com/v1';
    form.key.value = 'k';
    form.models.value[0]!.id = 'whisper-1';
    form.setTranscription(form.models.value[0]!, true);
    form.models.value[0]!.audioPrice = '0.006';
    expect(await form.submit()).toBe(true);
    expect(clients.provider.createModelProvider.mock.calls[1]![0].models[0].prices).toMatchObject({ audioMinuteMicros: 6_000n });
  });

  it('shows a stored per-minute price on edit and sends it back with the flag', () => {
    const base = useModelProvidersStore().items[0]!;
    const form = useProviderForm(orgId, projectId);
    form.show({ ...base, models: [{ id: 'whisper-1', vision: false, embedding: false, transcription: true, prices: { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n, audioMinuteMicros: 6_000n } }, base.models[0]!] });
    expect(form.models.value[0]!.audioPrice).toBe('0.006');
    expect(form.models.value[1]!.audioPrice).toBe('');
  });

  it('treats a hidden Transcription flag on an Anthropic provider as a chat model, with token prices', async () => {
    const store = useModelProvidersStore();
    const anthropic = store.items.find(p => p.kind === ModelProviderKind.ANTHROPIC)!;
    clients.provider.updateModelProvider.mockResolvedValue({ modelProvider: providerPBs()[2] });
    const form = useProviderForm(orgId, projectId);
    form.show(anthropic);
    const row = form.models.value[0]!;
    row.transcription = true;
    row.audioPrice = '';
    row.prices.input = '3';
    row.prices.output = '15';
    expect(isTranscriptionRow(row, form.anthropic.value)).toBe(false);
    expect(await form.submit()).toBe(true);
    const sent = clients.provider.updateModelProvider.mock.calls[0]![0].models[0];
    expect(sent.transcription).toBe(false);
    expect(sent.prices).toMatchObject({ inputMicros: 3_000_000n, outputMicros: 15_000_000n, audioMinuteMicros: 0n });
    row.prices.output = 'bad';
    expect(await form.submit()).toBe(false);
    expect(form.modelErrors.value).toEqual({ [row.key]: 'providers.price_invalid' });
  });

  it('creates a provider with a trimmed key and priced models, then clears the key', async () => {
    const [openrouter] = providerPBs();
    clients.provider.createModelProvider.mockResolvedValue({ modelProvider: { ...openrouter!, id: 'PvNewQca2345678923xyz', name: 'Local' } });
    const form = useProviderForm(orgId, projectId);
    form.show();
    expect(form.keyRequired.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.key).toBe('providers.key_required');
    expect(form.errors.value.name).toBeTruthy();
    expect(Object.values(form.modelErrors.value)).toEqual(['providers.model_id_required']);
    expect(form.formError.value, 'no raw models[0].id summary next to the row error').toBeNull();
    form.name.value = ' Local ';
    form.baseUrl.value = 'http://127.0.0.1:5190/v1';
    form.key.value = '  sk-1  ';
    form.models.value[0]!.id = 'm1';
    form.models.value[0]!.prices.input = '0.4';
    form.models.value[0]!.prices.output = '1.6';
    expect(await form.submit()).toBe(true);
    const req = clients.provider.createModelProvider.mock.calls[0]![0];
    expect(req).toMatchObject({ orgId, projectId, name: 'Local', baseUrl: 'http://127.0.0.1:5190/v1', apiKey: 'sk-1' });
    expect(req.models[0]).toMatchObject({ id: 'm1', prices: { inputMicros: 400_000n, outputMicros: 1_600_000n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
    expect(form.open.value).toBe(false);
    expect(form.key.value).toBe('');
    expect(useModelProvidersStore().items.at(-1)!.name).toBe('Local');
    expect(useToast().toasts.at(-1)?.text).toBe('providers.saved|{"name":"Local"}');
  });

  it('refuses a duplicate model id and a bad price, and places a server refusal under its input', async () => {
    setKnownKeys('apperror.PRV002');
    clients.provider.createModelProvider.mockRejectedValue(connectError('PRV002', { name: 'OpenRouter' }));
    const form = useProviderForm(orgId, projectId);
    form.show();
    form.name.value = 'OpenRouter';
    form.baseUrl.value = 'https://x.test';
    form.key.value = 'k';
    form.models.value[0]!.id = 'a';
    form.addModel();
    form.models.value[1]!.id = 'a';
    expect(await form.submit()).toBe(false);
    expect(Object.values(form.modelErrors.value)).toEqual(['providers.model_id_taken', 'providers.model_id_taken']);
    form.removeModel(form.models.value[1]!.key);
    form.models.value[0]!.prices.cacheRead = '1';
    expect(await form.submit()).toBe(false);
    expect(form.modelErrors.value).toEqual({ [form.models.value[0]!.key]: 'providers.price_invalid' });
    form.models.value[0]!.prices.cacheRead = '';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.name).toBe('apperror.PRV002|{"name":"OpenRouter"}');
    expect(form.formError.value).toBeNull();
    expect(form.open.value).toBe(true);
  });

  it('edits without the key, keeps the stored one, and asks for it again when the URL changes', async () => {
    const providers = useModelProvidersStore();
    const openrouter = providers.items[0]!;
    clients.provider.updateModelProvider.mockImplementation(async req => ({ modelProvider: { ...providerPBs()[0]!, baseUrl: req.baseUrl } }));
    const form = useProviderForm(orgId, projectId);
    form.show(openrouter);
    expect(form.models.value.map(m => [m.id, m.prices.input, m.prices.cacheRead])).toEqual([['openai/gpt-4.1-mini', '0.40', '0.10'], ['google/gemini-2.5-flash', '0.30', '0.00']]);
    expect(form.keyShown.value).toBe(false);
    expect(await form.submit()).toBe(true);
    expect(clients.provider.updateModelProvider.mock.calls[0]![0].apiKey).toBeUndefined();
    expect(providers.items[0]!.keySet).toBe(true);
    form.show(providers.items[0]!);
    form.replacingKey.value = true;
    expect(form.keyShown.value).toBe(true);
    expect(form.keyRequired.value).toBe(false);
    form.baseUrl.value = 'https://other.test/v1';
    expect(form.urlChanged.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ key: 'providers.key_required' });
    form.key.value = 'new';
    expect(await form.submit()).toBe(true);
    expect(clients.provider.updateModelProvider.mock.calls[1]![0]).toMatchObject({ id: openrouter.id, baseUrl: 'https://other.test/v1', apiKey: 'new' });
  });

  it('shows PRV006 under the key when the server still wants one', async () => {
    setKnownKeys('apperror.PRV006_secret_required');
    clients.provider.updateModelProvider.mockRejectedValue(connectError('PRV006', { reason: 'secret_required' }));
    const form = useProviderForm(orgId, projectId);
    form.show(useModelProvidersStore().items[0]!);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.key).toBe('apperror.PRV006_secret_required');
  });

  it('refuses more models than the proto allows, ignores a second submit and keeps the form open while saving', async () => {
    let release!: () => void;
    clients.provider.createModelProvider.mockReturnValue(new Promise((resolve) => {
      release = () => resolve({ modelProvider: providerPBs()[1]! });
    }));
    const form = useProviderForm(orgId, projectId);
    form.show();
    form.name.value = 'Groq';
    form.baseUrl.value = 'https://api.groq.com/openai/v1';
    form.key.value = 'typed';
    for (let i = 0; i < 65; i++) {
      form.addModel();
    }
    form.models.value.forEach((m, i) => (m.id = `m${i}`));
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.models).toBe('validation.too_many|{"max":"64"}');
    form.models.value = form.models.value.slice(0, 1);
    form.baseUrl.value = 'https://api.groq.com/openai/v1';
    form.key.value = 'typed';
    form.models.value[0]!.id = 'llama';
    const saving = form.submit();
    expect(await form.submit()).toBe(false);
    form.hide();
    expect(form.open.value).toBe(true);
    release();
    expect(await saving).toBe(true);
    form.show();
    form.key.value = 'typed';
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.key.value).toBe('');
  });
});
