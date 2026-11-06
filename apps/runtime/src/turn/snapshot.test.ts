import type { Snapshot } from '@strands-agents/sdk';
import { describe, expect, it } from 'vitest';

import { cleanSnapshot, IMAGE_NOTE, SDK_VERSION, tagsMatch } from './snapshot.ts';

function snap(messages: unknown[]): Snapshot {
  return { scope: 'agent', schemaVersion: '1.0', createdAt: '2026-11-06T00:00:00.000Z', data: { messages: messages as never }, appData: {} };
}

describe('cleanSnapshot', () => {
  it('replaces image blocks, also inside a tool result', () => {
    const image = { image: { format: 'png', source: { bytes: 'AAAA' } } };
    const out = cleanSnapshot(snap([
      { role: 'user', content: [{ text: 'what is this?' }, image] },
      { role: 'user', content: [{ toolResult: { toolUseId: 't1', status: 'success', content: [{ text: 'ok' }, image] } }] },
    ]));
    expect(out.data.messages).toEqual([
      { role: 'user', content: [{ text: 'what is this?' }, { text: IMAGE_NOTE }] },
      { role: 'user', content: [{ toolResult: { toolUseId: 't1', status: 'success', content: [{ text: 'ok' }, { text: IMAGE_NOTE }] } }] },
    ]);
  });

  it('drops a trailing refusal only when it is exactly the given text', () => {
    const messages = [
      { role: 'user', content: [{ text: 'hi' }] },
      { role: 'assistant', content: [{ text: 'DENIED: budget exhausted' }] },
    ];
    expect(cleanSnapshot(snap(messages), 'DENIED: budget exhausted').data.messages).toEqual([messages[0]]);
    expect(cleanSnapshot(snap(messages)).data.messages).toEqual(messages);
  });

  it('keeps a real reply that starts with DENIED:', () => {
    const messages = [{ role: 'assistant', content: [{ text: 'DENIED: your card was declined' }] }];
    expect(cleanSnapshot(snap(messages), 'DENIED: budget exhausted').data.messages).toEqual(messages);
  });

  it('does not change the input snapshot', () => {
    const input = snap([{ role: 'user', content: [{ image: { format: 'png', source: { bytes: 'AAAA' } } }] }]);
    cleanSnapshot(input);
    expect(input.data.messages).toEqual([{ role: 'user', content: [{ image: { format: 'png', source: { bytes: 'AAAA' } } }] }]);
  });
});

describe('tagsMatch', () => {
  it('needs the thread, the agent version and the SDK version', () => {
    expect(SDK_VERSION).toBe('1.23.0');
    const tags = { thread: 't1', agentVersion: 3 };
    expect(tagsMatch({ appData: { thread: 't1', agentVersion: 3, sdk: '1.23.0' } }, tags)).toBe(true);
    expect(tagsMatch({ appData: { thread: 't2', agentVersion: 3, sdk: '1.23.0' } }, tags)).toBe(false);
    expect(tagsMatch({ appData: { thread: 't1', agentVersion: 2, sdk: '1.23.0' } }, tags)).toBe(false);
    expect(tagsMatch({ appData: { thread: 't1', agentVersion: 3, sdk: '1.22.0' } }, tags)).toBe(false);
    expect(tagsMatch({}, tags)).toBe(false);
  });
});
