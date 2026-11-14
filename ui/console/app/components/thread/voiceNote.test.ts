import { describe, expect, it } from 'vitest';
import { voiceBadge, voiceStrip } from './voiceNote';

const reasons = ['too_long', 'too_large', 'unsupported', 'unreadable', 'over_budget', 'unpriced', 'rate_limited', 'failed'];

describe('voice note labels', () => {
  it('shows no badge for a transcribed note or one with no transcript state yet', () => {
    expect(voiceBadge(undefined)).toBeUndefined();
    expect(voiceBadge({})).toBeUndefined();
    expect(voiceBadge({ status: 'done', text: 'oke', seconds: 2 })).toBeUndefined();
  });

  it('names each reason a note was not transcribed, and warns only for the ones an owner can fix', () => {
    for (const reason of reasons) {
      expect(voiceBadge({ status: reason === 'failed' ? 'failed' : 'skipped', reason })).toEqual({ key: `thread.voice_${reason}`, warn: ['over_budget', 'unpriced', 'rate_limited'].includes(reason) });
    }
  });

  it('falls back to a plain label for a reason this console does not know, or none', () => {
    expect(voiceBadge({ status: 'skipped', reason: 'new_reason' })).toEqual({ key: 'thread.voice_unknown', warn: false });
    expect(voiceBadge({ status: 'failed' })).toEqual({ key: 'thread.voice_unknown', warn: false });
    expect(voiceBadge({ status: 'skipped', reason: 'toString' })).toEqual({ key: 'thread.voice_unknown', warn: false });
  });

  it('labels the strip with the note length when it is known', () => {
    const clock = (s: number) => `c${s}`;
    expect(voiceStrip({ status: 'done', seconds: 14 }, clock)).toEqual({ key: 'thread.voice_label', params: { duration: 'c14' } });
    expect(voiceStrip({ status: 'done', seconds: 0 }, clock)).toEqual({ key: 'thread.voice_label', params: { duration: 'c0' } });
    expect(voiceStrip({ status: 'skipped', reason: 'too_long' }, clock)).toEqual({ key: 'thread.voice_placeholder' });
  });
});
