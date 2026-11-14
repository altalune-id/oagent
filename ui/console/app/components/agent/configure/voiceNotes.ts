import type { AgentConfig, AgentLimits, TranscriptionModel } from './agentConfig';
import type { ModelProvider, ProviderModel } from '~/stores/modelProviders';
import { formatBytes } from '#base/lib/format/bytes';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { staleVoiceReply } from '~/components/channels/replies';
import { usdText } from '~/lib/money/usd';
import { keyOf } from '~/stores/modelProviders';

export interface VoiceOption {
  value: string;
  providerId: string;
  providerName: string;
  model: string;
  audioMinuteMicros?: bigint;
}

export type VoiceStale = 'provider_missing' | 'not_listed';

// voiceOptions lists the transcription models on OpenAI-compatible providers, sorted by provider name, then model id.
export function voiceOptions(providers: ModelProvider[]): VoiceOption[] {
  return providers
    .filter(p => p.kind === ModelProviderKind.OPENAI_COMPATIBLE)
    .flatMap(p => p.models.filter(m => m.transcription).map(m => ({ value: keyOf({ providerId: p.id, model: m.id }), providerId: p.id, providerName: p.name, model: m.id, audioMinuteMicros: m.prices?.audioMinuteMicros })))
    .sort((a, b) => a.providerName.localeCompare(b.providerName) || a.model.localeCompare(b.model));
}

// voiceValue is the select value of a transcription model setting, empty when voice notes are off.
export function voiceValue(tm: TranscriptionModel): string {
  return tm.model ? keyOf(tm) : '';
}

// voiceStale says why the set model is not among the options: its provider was deleted, or it is no longer marked Transcription.
export function voiceStale(tm: TranscriptionModel, options: VoiceOption[]): VoiceStale | undefined {
  if (!tm.model) {
    return undefined;
  }
  if (tm.providerMissing) {
    return 'provider_missing';
  }
  return options.some(o => o.value === keyOf(tm)) ? undefined : 'not_listed';
}

// pickVoice turns a select value into the setting; picking the stale value again keeps it as it is.
export function pickVoice(value: string, options: VoiceOption[], current: TranscriptionModel): TranscriptionModel {
  if (!value) {
    return { providerId: '', model: '', providerMissing: false };
  }
  const o = options.find(x => x.value === value);
  return o ? { providerId: o.providerId, model: o.model, providerMissing: false } : current;
}

// voiceStaleLabel names a stale value for its warning: its model, and its provider or a dash when that provider is gone.
export function voiceStaleLabel(tm: TranscriptionModel, providers: ModelProvider[]): { model: string, provider: string } {
  return { model: tm.model, provider: providers.find(p => p.id === tm.providerId)?.name || '—' };
}

// voiceStaleOption labels the stale select option: model and provider, or the bare model when its provider is gone.
export function voiceStaleOption(tm: TranscriptionModel, providers: ModelProvider[], t: (key: string, params?: Record<string, unknown>) => string): string {
  const provider = providers.find(p => p.id === tm.providerId)?.name;
  return provider ? t('configure.voice_option', { model: tm.model, provider }) : tm.model;
}

// voicePrice is the price line of the chosen model, per audio minute or free.
export function voicePrice(o?: VoiceOption): { key: string, params?: Record<string, string> } | undefined {
  if (!o) {
    return undefined;
  }
  return o.audioMinuteMicros ? { key: 'configure.voice_price', params: { price: usdText(o.audioMinuteMicros) } } : { key: 'configure.voice_free' };
}

// voiceHint fills the voice note limits of the hint from the agent defaults.
export function voiceHint(limits: AgentLimits, count: (n: number) => string, locale: string): { max_minutes: string, max_size: string } {
  return {
    max_minutes: limits.voiceNoteMaxSeconds ? count(limits.voiceNoteMaxSeconds / 60) : '—',
    max_size: limits.voiceNoteMaxBytes ? formatBytes(limits.voiceNoteMaxBytes, locale) : '—',
  };
}

// chatModelsOf lists the models a chat or helper select offers: no embedding or transcription model, except one already chosen.
export function chatModelsOf(models: ProviderModel[], config: Pick<AgentConfig, 'model' | 'helperModel'>): ProviderModel[] {
  return models.filter(m => (!m.embedding && !m.transcription) || m.id === config.model || m.id === config.helperModel);
}

// voiceOn reports whether an agent setting transcribes voice notes; a model whose provider was deleted counts as off.
export function voiceOn(tm?: TranscriptionModel): tm is TranscriptionModel {
  return !!tm?.model && !tm.providerMissing;
}

// voiceConnectionHint says whether the agent transcribes voice notes, naming the set reply when off; a model whose provider was deleted counts as off.
export function voiceConnectionHint(tm: TranscriptionModel | undefined, reply: string): { key: string, params: Record<string, string> } {
  return voiceOn(tm) ? { key: 'openwa.voice_on', params: { model: tm.model } } : { key: 'openwa.voice_off', params: { reply } };
}

// voiceChannelNotes decides the WhatsApp page's voice notes: the hint, whether it waits for a publish, and whether to offer the new set reply; an unpublished draft never prompts the reply switch.
export function voiceChannelNotes(tm: TranscriptionModel | undefined, unpublished: boolean, storedReply: string, reply: string): { hint: { key: string, params: Record<string, string> }, afterPublish: boolean, staleReply: boolean } {
  return { hint: voiceConnectionHint(tm, reply), afterPublish: unpublished, staleReply: !unpublished && staleVoiceReply(storedReply, voiceOn(tm)) };
}
