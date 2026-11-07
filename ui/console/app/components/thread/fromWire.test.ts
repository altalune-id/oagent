import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { RunSchema, RunStatus, ThreadMessageSchema, ThreadSchema } from '#gen/thread/v1/thread_pb';
import { toItems as itemsOf, maskedLabel, runStatus, skipReason, toThread as threadOf, toRunSummary, wireLabel } from './fromWire';

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}|${JSON.stringify(params)}` : key);
const toThread = (th: Parameters<typeof threadOf>[0]) => threadOf(th, t);
const toItems = (messages: Parameters<typeof itemsOf>[0], runs: Parameters<typeof itemsOf>[1]) => itemsOf(messages, runs, t);

describe('thread wire mapping', () => {
  it('titles a thread with no person by its channel, so a group never has an empty title', () => {
    expect(threadOf(create(ThreadSchema, { id: 't', kind: 'group' }), t).title).toBe('threads.kind_whatsapp_group');
  });

  it('labels a person from the structured fields in the viewer\'s locale, else the server label, never a full number', () => {
    expect(wireLabel({ displayName: 'Sari', identityKind: IdentityKind.WA_PHONE, identityHint: '90' }, 'ignored', t)).toBe('Sari');
    expect(wireLabel({ displayName: '', identityKind: IdentityKind.WA_PHONE, identityHint: '90' }, '…90', t)).toBe('person.masked|{"kind":"person.kind_wa_phone","hint":"90"}');
    expect(wireLabel({ displayName: '', identityKind: IdentityKind.UNSPECIFIED, identityHint: '' }, 'WhatsApp +62 812-3456-5521', t)).toBe('WhatsApp …5521');
    expect(wireLabel({ displayName: 'Budi 0812 3456 7890', identityKind: IdentityKind.WA_PHONE, identityHint: '90' }, '', t)).toBe('Budi …7890');
    const th = threadOf(create(ThreadSchema, { id: 't', kind: 'dm', endUserLabel: '…90', endUserIdentityKind: IdentityKind.WA_PHONE, endUserIdentityHint: '90' }), t);
    expect(th.title).toBe('person.masked|{"kind":"person.kind_wa_phone","hint":"90"}');
    const [said] = itemsOf([create(ThreadMessageSchema, { seq: 1n, role: 'user', speakerLabel: 'Sari', speakerDisplayName: 'Sari', speakerIdentityKind: IdentityKind.WA_LID, message: { content: 'hai' } })], [], t);
    expect(said).toMatchObject({ kind: 'user', speaker: 'Sari' });
  });

  it('masks any phone number in a label, keeping its last four digits', () => {
    expect(maskedLabel('WhatsApp · +62 812-3456-5521')).toBe('WhatsApp · …5521');
    expect(maskedLabel('WhatsApp ·…5521')).toBe('WhatsApp ·…5521');
  });

  it('maps statuses, with unspecified meaning no open run', () => {
    expect(runStatus(RunStatus.UNSPECIFIED)).toBeUndefined();
    expect(runStatus(RunStatus.LIMIT_REACHED)).toBe('limit_reached');
  });

  it('maps a thread with its kind, masked label and no transcript yet, and an unknown kind as API', () => {
    expect(toThread(create(ThreadSchema, { id: 't', kind: 'group', endUserLabel: '', runCount: 2n }))).toMatchObject({ kind: 'whatsapp_group', title: 'threads.kind_whatsapp_group', people: [], status: undefined, runs: 2, items: [], createdAt: new Date(0) });
    expect(toThread(create(ThreadSchema, { id: 't', kind: 'other' })).kind).toBe('api');
  });

  it('keeps a run unpriced without cost micros, and its error code', () => {
    expect(toRunSummary(create(RunSchema, { id: 'r', status: RunStatus.FAILED, errorCode: 'RUN005' }), 'T')).toMatchObject({ status: 'failed', unpriced: true, estimated: false, tokens: 0, seconds: 0, costCents: 0, detail: { key: 'runstatus.detail_error', params: { code: 'RUN005' } }, startedAt: new Date(0) });
    expect(toRunSummary(create(RunSchema, { id: 'r', status: RunStatus.UNSPECIFIED }), 'T').status).toBe('running');
  });

  it('maps why a channel message was skipped, keeping an unknown reason as skipped', () => {
    expect(['', 'stale', 'sender_rate', 'quota'].map(skipReason)).toEqual([undefined, 'stale', 'sender_rate', 'other']);
    expect(toItems([create(ThreadMessageSchema, { seq: 1n, role: 'user', skipReason: 'stale', message: { content: 'late' } })], [])).toEqual([{ id: 0, kind: 'user', text: 'late', speaker: undefined, skipped: 'stale', at: new Date(0) }]);
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
