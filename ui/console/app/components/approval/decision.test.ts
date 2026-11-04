import type { ApprovalRequest } from '~/stores/approvals';
import { describe, expect, it } from 'vitest';
import { decideApproval, decisionLabel, statusAt, viaLabel } from './decision';

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}|${JSON.stringify(params)}` : key);
const time = () => '10:42';
const now = new Date(2026, 10, 4, 12, 0);
const pending: ApprovalRequest = { id: 'a1', tool: 'yasaku__record_expense', title: 'Simpan?', args: [], raw: {}, status: 'pending', requestedBy: 'Budi', expiresAt: new Date(2026, 10, 5, 12, 0) };

describe('approval decisions', () => {
  it('counts a pending request past its expiry as expired', () => {
    expect(statusAt(pending, now)).toBe('pending');
    expect(statusAt(pending, new Date(2026, 10, 5, 12, 0))).toBe('expired');
    expect(statusAt({ ...pending, expiresAt: undefined }, now)).toBe('pending');
    expect(statusAt({ ...pending, status: 'approved' }, new Date(2027, 0, 1))).toBe('approved');
  });

  it('records one decision on a pending request and refuses a second or an expired one', () => {
    const decided = decideApproval(pending, true, 'Budi', { channel: 'console' }, ' ', now)!;
    expect(decided).toMatchObject({ status: 'approved', decidedBy: 'Budi', decidedVia: { channel: 'console' }, decidedAt: now, reason: undefined });
    expect(decideApproval(decided, false, 'Sari', { channel: 'console' }, '', now)).toBeNull();
    expect(decideApproval(pending, false, 'Sari', { channel: 'try' }, ' salah ', now)!.reason).toBe('salah');
    expect(decideApproval(pending, true, 'Budi', { channel: 'console' }, '', new Date(2026, 10, 6))).toBeNull();
  });

  it('labels who decided, how and when, with one function for every screen', () => {
    expect(viaLabel(undefined, t)).toBe('');
    expect(viaLabel({ channel: 'whatsapp', reply: 'ya' }, t)).toBe('approvalcard.via_whatsapp_reply|{"reply":"ya"}');
    expect(viaLabel({ channel: 'console' }, t)).toBe('approvalcard.via_console');
    const approved = { ...pending, status: 'approved' as const, decidedBy: 'Sari', decidedVia: { channel: 'whatsapp' as const, reply: 'ya' }, decidedAt: now };
    expect(decisionLabel(approved, t, time, now)).toBe('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp_reply|{"reply":"ya"} · 10:42');
    expect(decisionLabel({ ...pending, status: 'rejected', decidedBy: 'Budi' }, t, time, now)).toBe('approvalcard.rejected|{"by":"Budi"}');
    expect(decisionLabel(pending, t, time, new Date(2026, 10, 6))).toBe('approvalcard.expired');
    expect(decisionLabel(pending, t, time, now)).toBe('');
  });
});
