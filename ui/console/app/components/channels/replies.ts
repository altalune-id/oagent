export interface ReplyTexts {
  failure: string;
  voice: string;
  photo: string;
  approvalPrompt: string;
  approvalExpired: string;
}

export const replyKeys = ['failure', 'voice', 'photo', 'approvalPrompt', 'approvalExpired'] as const;
export const maxReplyLength = 1000;
export type ReplyKey = typeof replyKeys[number];

export const previousVoiceReply = 'Sorry, I can\'t listen to voice messages yet. Please type your message.';

// staleVoiceReply reports whether a connection still sends the old voice note default, which is wrong once voice notes are on.
export function staleVoiceReply(stored: string, voiceOn: boolean): boolean {
  return voiceOn && stored.trim() === previousVoiceReply;
}

export interface ChannelTiming {
  quietMs: number;
  quietMaxMs: number;
  maxMessageAgeMinutes: number;
  maxQuietMs: number;
}

// replyErrors flags each set reply left empty, since the agent would then send a blank message.
export function replyErrors(replies: ReplyTexts): Partial<Record<ReplyKey, string>> {
  return Object.fromEntries(replyKeys.flatMap((k) => {
    const text = replies[k].trim();
    if (!text) {
      return [[k, 'channels.error_reply_required']];
    }
    if ([...text].length > maxReplyLength) {
      return [[k, 'channels.error_reply_too_long']];
    }
    return k === 'approvalPrompt' && !text.includes('{preview}') ? [[k, 'channels.error_reply_preview']] : [];
  }));
}

// seconds turns milliseconds into whole or one-decimal seconds for the read-only timing values.
export function seconds(ms: number): number {
  return Math.round(ms / 100) / 10;
}

// toMs reads a typed number of seconds, one decimal at most, into milliseconds, or null when it isn't one.
export function toMs(text: string): number | null {
  const clean = text.trim().replace(',', '.');
  return /^\d+(?:\.\d)?$/.test(clean) ? Math.round(Number(clean) * 1000) : null;
}

// timingErrors flags a quiet time that is not a number or is over the server's limit (none when 0), or a window longer than its maximum.
export function timingErrors(quietMs: number | null, quietMaxMs: number | null, limitMs: number): Partial<Record<'quiet' | 'quietMax', string>> {
  const out: Partial<Record<'quiet' | 'quietMax', string>> = {};
  const range = limitMs > 0 ? 'channels.error_quiet_range' : 'channels.error_quiet_number';
  const over = (ms: number) => limitMs > 0 && ms > limitMs;
  if (quietMs === null || over(quietMs)) {
    out.quiet = range;
  }
  if (quietMaxMs === null || over(quietMaxMs)) {
    out.quietMax = range;
  } else if (quietMs !== null && quietMs > quietMaxMs) {
    out.quietMax = 'channels.error_quiet_order';
  }
  return out;
}
