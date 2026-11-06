import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { RunSchema, RunStatus, ThreadMessageSchema, ThreadSchema } from '#gen/thread/v1/thread_pb';
import { maskedLabel, runStatus, toItems, toRunSummary, toThread } from './fromWire';

describe('thread wire mapping', () => {
  it('masks any phone number in a label, keeping its last four digits', () => {
    expect(maskedLabel('WhatsApp · +62 812-3456-5521')).toBe('WhatsApp · …5521');
    expect(maskedLabel('WhatsApp ·…5521')).toBe('WhatsApp ·…5521');
  });

  it('maps statuses, with unspecified meaning no open run', () => {
    expect(runStatus(RunStatus.UNSPECIFIED)).toBeUndefined();
    expect(runStatus(RunStatus.LIMIT_REACHED)).toBe('limit_reached');
  });

  it('maps a thread with its kind, masked label and no transcript yet, and an unknown kind as API', () => {
    expect(toThread(create(ThreadSchema, { id: 't', kind: 'group', endUserLabel: '', runCount: 2n }))).toMatchObject({ kind: 'whatsapp_group', title: '', people: [], status: undefined, runs: 2, items: [], createdAt: new Date(0) });
    expect(toThread(create(ThreadSchema, { id: 't', kind: 'other' })).kind).toBe('api');
  });

  it('keeps a run unpriced without cost micros, and its error code', () => {
    expect(toRunSummary(create(RunSchema, { id: 'r', status: RunStatus.FAILED, errorCode: 'RUN005' }), 'T')).toMatchObject({ status: 'failed', unpriced: true, estimated: false, tokens: 0, seconds: 0, costCents: 0, detail: { key: 'runstatus.detail_error', params: { code: 'RUN005' } }, startedAt: new Date(0) });
    expect(toRunSummary(create(RunSchema, { id: 'r', status: RunStatus.UNSPECIFIED }), 'T').status).toBe('running');
  });

  it('reads text parts, keeps odd tool arguments, and shows an orphan tool result on its own', () => {
    const items = toItems([
      create(ThreadMessageSchema, { seq: 1n, role: 'user', message: { content: [{ type: 'text', text: 'Hi ' }, { type: 'image' }, { type: 'text', text: 'there' }] } }),
      create(ThreadMessageSchema, { seq: 2n, role: 'assistant', message: { toolCalls: [{ id: 'c1', function: { name: 'a', arguments: 'not json' } }, { id: 'c2', function: { name: 'b', arguments: '[1]' } }, 'junk'] } }),
      create(ThreadMessageSchema, { seq: 3n, role: 'tool', toolCallId: 'gone', message: { content: 'late' } }),
      create(ThreadMessageSchema, { seq: 4n, role: 'system', message: { content: 'ignored' } }),
      create(ThreadMessageSchema, { seq: 5n, role: 'assistant' }),
    ], []);
    expect(items).toEqual([
      { id: 0, kind: 'user', text: 'Hi there', speaker: undefined, at: new Date(0) },
      { id: 1, kind: 'tool', name: 'a', args: { arguments: 'not json' }, result: undefined, state: 'running' },
      { id: 2, kind: 'tool', name: 'b', args: {}, result: undefined, state: 'running' },
      { id: 3, kind: 'tool', name: 'gone', args: {}, result: 'late', state: 'done' },
    ]);
  });
});
