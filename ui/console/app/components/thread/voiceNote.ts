import type { VoiceNote } from '~/stores/threads';

const REASONS = new Set(['too_long', 'too_large', 'unsupported', 'unreadable', 'over_budget', 'unpriced', 'rate_limited', 'failed']);
const OWNER_CAN_FIX = new Set(['over_budget', 'unpriced', 'rate_limited']);

export interface VoiceBadge {
  key: string;
  warn: boolean;
}

// voiceBadge says why a voice note was not transcribed; a transcribed note, or one with no transcript state yet, gets none.
export function voiceBadge(v?: VoiceNote): VoiceBadge | undefined {
  if (!v?.status || v.status === 'done') {
    return undefined;
  }
  const reason = v.reason ?? '';
  return { key: REASONS.has(reason) ? `thread.voice_${reason}` : 'thread.voice_unknown', warn: OWNER_CAN_FIX.has(reason) };
}

// voiceStrip picks the voice note strip label, with the note length as a clock when it is known.
export function voiceStrip(v: VoiceNote, clock: (s: number) => string): { key: string, params?: Record<string, string> } {
  return v.seconds === undefined ? { key: 'thread.voice_placeholder' } : { key: 'thread.voice_label', params: { duration: clock(v.seconds) } };
}
