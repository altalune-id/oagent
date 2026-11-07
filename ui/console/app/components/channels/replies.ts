export interface ReplyTexts {
  failure: string;
  voice: string;
  photo: string;
}

export const replyKeys = ['failure', 'voice', 'photo'] as const;
export type ReplyKey = typeof replyKeys[number];

export interface ChannelTiming {
  quietMs: number;
  quietMaxMs: number;
  maxMessageAgeMinutes: number;
}

// replyErrors flags each set reply left empty, since the agent would then send a blank message.
export function replyErrors(replies: ReplyTexts): Partial<Record<ReplyKey, string>> {
  return Object.fromEntries(replyKeys.filter(k => !replies[k].trim()).map(k => [k, 'channels.error_reply_required']));
}

// seconds turns milliseconds into whole or one-decimal seconds for the read-only timing values.
export function seconds(ms: number): number {
  return Math.round(ms / 100) / 10;
}

export const quietLimitMs = 10_000;

// toMs reads a typed number of seconds, one decimal at most, into milliseconds, or null when it isn't one.
export function toMs(text: string): number | null {
  const clean = text.trim().replace(',', '.');
  return /^\d+(?:\.\d)?$/.test(clean) ? Math.round(Number(clean) * 1000) : null;
}

// timingErrors flags a quiet window outside 0-10 s, or a window longer than its maximum.
export function timingErrors(quietMs: number | null, quietMaxMs: number | null): Partial<Record<'quiet' | 'quietMax', string>> {
  const out: Partial<Record<'quiet' | 'quietMax', string>> = {};
  if (quietMs === null || quietMs > quietLimitMs) {
    out.quiet = 'channels.error_quiet_range';
  }
  if (quietMaxMs === null || quietMaxMs > quietLimitMs) {
    out.quietMax = 'channels.error_quiet_range';
  } else if (quietMs !== null && quietMs > quietMaxMs) {
    out.quietMax = 'channels.error_quiet_order';
  }
  return out;
}
