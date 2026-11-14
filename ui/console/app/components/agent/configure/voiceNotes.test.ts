import type { ModelProvider } from '~/stores/modelProviders';
import { describe, expect, it } from 'vitest';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { chatModelsOf, pickVoice, voiceChannelNotes, voiceConnectionHint, voiceHint, voiceOn, voiceOptions, voicePrice, voiceStale, voiceStaleLabel, voiceStaleOption, voiceValue } from './voiceNotes';

const zero = { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n };
const provider = (id: string, name: string, models: ModelProvider['models'], kind = ModelProviderKind.OPENAI_COMPATIBLE): ModelProvider => ({ id, orgId: 'o', projectId: 'p', name, kind, baseUrl: 'https://x.test', keySet: true, models });
const groq = provider('PvGroq', 'Groq', [{ id: 'whisper-large-v3', vision: false, embedding: false, transcription: true, prices: { ...zero, audioMinuteMicros: 1_850n } }, { id: 'llama', vision: false, embedding: false, prices: zero }]);
const openai = provider('PvOpenai', 'OpenAI', [{ id: 'whisper-1', vision: false, embedding: false, transcription: true, prices: { ...zero, audioMinuteMicros: 6_000n } }, { id: 'gpt-4o-transcribe', vision: false, embedding: false, transcription: true, prices: { ...zero, audioMinuteMicros: 0n } }]);
const claude = provider('PvClaude', 'Anthropic', [{ id: 'claude', vision: true, embedding: false, transcription: true, prices: zero }], ModelProviderKind.ANTHROPIC);
const off = { providerId: '', model: '', providerMissing: false };

describe('voice note picker', () => {
  it('offers every transcription model on an OpenAI-compatible provider, sorted by provider then model', () => {
    expect(voiceOptions([openai, claude, groq]).map(o => [o.value, o.providerName, o.model])).toEqual([
      ['PvGroq|whisper-large-v3', 'Groq', 'whisper-large-v3'],
      ['PvOpenai|gpt-4o-transcribe', 'OpenAI', 'gpt-4o-transcribe'],
      ['PvOpenai|whisper-1', 'OpenAI', 'whisper-1'],
    ]);
    expect(voiceOptions([])).toEqual([]);
  });

  it('reads the chosen value, Off when unset', () => {
    expect(voiceValue(off)).toBe('');
    expect(voiceValue({ providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false })).toBe('PvOpenai|whisper-1');
  });

  it('flags a deleted provider and a model no longer marked Transcription, and nothing for Off or a listed model', () => {
    const options = voiceOptions([openai, groq]);
    expect(voiceStale(off, options)).toBeUndefined();
    expect(voiceStale({ providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false }, options)).toBeUndefined();
    expect(voiceStale({ providerId: '', model: 'whisper-1', providerMissing: true }, options)).toBe('provider_missing');
    expect(voiceStale({ providerId: 'PvGroq', model: 'llama', providerMissing: false }, options)).toBe('not_listed');
  });

  it('writes the picked model, clears it for Off, and keeps a stale value when it is picked again', () => {
    const options = voiceOptions([openai, groq]);
    expect(pickVoice('PvOpenai|whisper-1', options, off)).toEqual({ providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false });
    expect(pickVoice('', options, { providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false })).toEqual(off);
    const stale = { providerId: '', model: 'whisper-1', providerMissing: true };
    expect(pickVoice('|whisper-1', options, stale)).toBe(stale);
  });

  it('prices the chosen model per audio minute, or as free', () => {
    const [groqOption, freeOption, paidOption] = voiceOptions([openai, groq]);
    expect(voicePrice(paidOption)).toEqual({ key: 'configure.voice_price', params: { price: '0.006' } });
    expect(voicePrice(groqOption)).toEqual({ key: 'configure.voice_price', params: { price: '0.00185' } });
    expect(voicePrice(freeOption)).toEqual({ key: 'configure.voice_free' });
    expect(voicePrice(undefined)).toBeUndefined();
  });

  it('fills the hint limits from the agent defaults, never a number of its own', () => {
    const count = (n: number) => `n${n}`;
    expect(voiceHint({ voiceNoteMaxSeconds: 300, voiceNoteMaxBytes: 5 * 1024 * 1024 }, count, 'en-US')).toEqual({ max_minutes: 'n5', max_size: '5 MiB' });
    expect(voiceHint({}, count, 'en-US')).toEqual({ max_minutes: '—', max_size: '—' });
  });

  it('leaves transcription models out of the chat model list, except the one already chosen', () => {
    const models = [...openai.models, ...groq.models, { id: 'embed', vision: false, embedding: true, prices: zero }];
    expect(chatModelsOf(models, { model: '', helperModel: '' }).map(m => m.id)).toEqual(['llama']);
    expect(chatModelsOf(models, { model: 'whisper-1', helperModel: 'embed' }).map(m => m.id)).toEqual(['whisper-1', 'llama', 'embed']);
  });

  it('tells the WhatsApp page whether voice notes are transcribed, treating a deleted provider as off', () => {
    const offHint = { key: 'openwa.voice_off', params: { reply: 'R' } };
    expect(voiceConnectionHint(undefined, 'R')).toEqual(offHint);
    expect(voiceConnectionHint(off, 'R')).toEqual(offHint);
    expect(voiceConnectionHint({ providerId: '', model: 'whisper-1', providerMissing: true }, 'R')).toEqual(offHint);
    expect(voiceConnectionHint({ providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false }, 'R')).toEqual({ key: 'openwa.voice_on', params: { model: 'whisper-1' } });
  });

  it('labels a stale value: the bare model when its provider is gone, else model and provider name', () => {
    expect(voiceStaleLabel({ providerId: '', model: 'whisper-1', providerMissing: true }, [openai, groq])).toEqual({ model: 'whisper-1', provider: '—' });
    expect(voiceStaleLabel({ providerId: 'PvGroq', model: 'llama', providerMissing: false }, [openai, groq])).toEqual({ model: 'llama', provider: 'Groq' });
    expect(voiceStaleLabel({ providerId: 'PvGone', model: 'x', providerMissing: false }, [openai])).toEqual({ model: 'x', provider: '—' });
    const t = (key: string, params?: Record<string, unknown>) => `${key}|${JSON.stringify(params)}`;
    expect(voiceStaleOption({ providerId: 'PvGroq', model: 'llama', providerMissing: false }, [groq], t)).toBe('configure.voice_option|{"model":"llama","provider":"Groq"}');
    expect(voiceStaleOption({ providerId: '', model: 'whisper-1', providerMissing: true }, [groq], t)).toBe('whisper-1');
  });

  it('reads voice notes as off for no setting', () => {
    expect(voiceOn(undefined)).toBe(false);
  });

  it('qualifies the WhatsApp voice hint and hides the old-reply notice until the draft is published', () => {
    const on = { providerId: 'PvOpenai', model: 'whisper-1', providerMissing: false };
    const old = 'Sorry, I can\'t listen to voice messages yet. Please type your message.';
    expect(voiceChannelNotes(on, true, old, 'R')).toEqual({ hint: { key: 'openwa.voice_on', params: { model: 'whisper-1' } }, afterPublish: true, staleReply: false });
    expect(voiceChannelNotes(on, false, old, 'R')).toEqual({ hint: { key: 'openwa.voice_on', params: { model: 'whisper-1' } }, afterPublish: false, staleReply: true });
    expect(voiceChannelNotes(off, false, old, 'R')).toEqual({ hint: { key: 'openwa.voice_off', params: { reply: 'R' } }, afterPublish: false, staleReply: false });
    expect(voiceChannelNotes(on, false, 'Custom.', 'R').staleReply).toBe(false);
  });
});
