import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { ApprovalSchema } from '#gen/approval/v1/approval_pb';
import { previewCalls, toApprovalItem } from './fromWire';

const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 9, 5, min)));

describe('approval preview', () => {
  it('splits the plain-text preview into one titled call per header with its key: value rows', () => {
    expect(previewCalls('yasaku · record_expense\namount: 25000\nnote: a: b\nsheets · append_row\na: 1\n…')).toEqual({
      calls: [
        { title: 'yasaku · record_expense', args: [{ label: 'amount', value: '25000' }, { label: 'note', value: 'a: b' }] },
        { title: 'sheets · append_row', args: [{ label: 'a', value: '1' }] },
      ],
      more: true,
    });
    expect(previewCalls('')).toEqual({ calls: [], more: false });
    expect(previewCalls('bank · transfer\nto-number: 0812\nuser.id: 7\nnote: a')).toEqual({
      calls: [{ title: 'bank · transfer', args: [{ label: 'to-number', value: '0812' }, { label: 'user.id', value: '7' }, { label: 'note', value: 'a' }] }],
      more: false,
    });
    expect(previewCalls('amount: 1')).toEqual({ calls: [{ title: '', args: [{ label: 'amount', value: '1' }] }], more: false });
  });
});

describe('approval from the wire', () => {
  const agents = (id: string) => (id === 'Ag1' ? { name: 'Yasaku', slug: 'yasaku' } : undefined);

  it('maps a decided approval with its people, times, channel and where it was decided', () => {
    const item = toApprovalItem(create(ApprovalSchema, {
      id: 'Ap1',
      projectId: 'P1',
      agentId: 'Ag1',
      threadId: 'Th1',
      runId: 'Rn1',
      status: 'approved',
      channel: 'whatsapp',
      tools: ['yasaku__record_expense'],
      preview: 'yasaku · record_expense\namount: 25000',
      requesterEndUserId: 'Eu1',
      requesterLabel: 'Sari',
      decidedBy: 'Us1',
      decidedByLabel: 'Budi',
      decisionText: 'ok',
      decidedVia: 'console',
      createdAt: at(1),
      expiresAt: at(60),
      decidedAt: at(2),
    }), agents);
    expect(item).toEqual({
      approval: {
        id: 'Ap1',
        tool: 'yasaku__record_expense',
        title: 'yasaku · record_expense',
        args: [{ label: 'amount', value: '25000' }],
        calls: [{ title: 'yasaku · record_expense', args: [{ label: 'amount', value: '25000' }] }],
        more: false,
        preview: 'yasaku · record_expense\namount: 25000',
        status: 'approved',
        requestedBy: 'Sari',
        requestedById: 'Eu1',
        decidedBy: 'Budi',
        decidedById: 'Us1',
        decidedVia: { channel: 'console' },
        reason: 'ok',
        expiresAt: new Date(Date.UTC(2026, 10, 9, 6, 0)),
        decidedAt: new Date(Date.UTC(2026, 10, 9, 5, 2)),
      },
      agentId: 'Ag1',
      agentName: 'Yasaku',
      agentSlug: 'yasaku',
      threadId: 'Th1',
      runId: 'Rn1',
      channel: 'whatsapp',
      askedAt: new Date(Date.UTC(2026, 10, 9, 5, 1)),
    });
  });

  it('leaves a pending approval undecided, falls back on the tool for the title, names the agent by its id and leaves the slug unset when the agent is unknown', () => {
    const item = toApprovalItem(create(ApprovalSchema, { id: 'Ap2', agentId: 'Ag9', status: 'pending', channel: 'api', tools: ['t__x'], requesterLabel: '' }), agents);
    expect(item.approval).toMatchObject({ title: 't__x', status: 'pending', decidedVia: undefined, decidedBy: undefined, reason: undefined, expiresAt: undefined });
    expect([item.agentName, item.agentSlug, item.channel]).toEqual(['Ag9', undefined, 'api']);
    expect(toApprovalItem(create(ApprovalSchema, { id: 'x', status: 'odd', channel: 'odd' }), agents).approval.status).toBe('pending');
    expect(toApprovalItem(create(ApprovalSchema, { id: 'x', status: 'expired', channel: 'odd' }), agents).channel).toBe('console');
  });
});
