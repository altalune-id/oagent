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
