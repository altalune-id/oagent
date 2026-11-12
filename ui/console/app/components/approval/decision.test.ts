import type { ApprovalRequest } from '~/stores/approvals';
import { describe, expect, it } from 'vitest';
import { decisionLabel, expiresIn, statusAt, viaLabel } from './decision';

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

  it('counts down at minute granularity, hours from an hour up, soon under an hour', () => {
    expect(expiresIn(pending, now)).toEqual({ key: 'approvals.expires_in_h', n: 24, soon: false });
    expect(expiresIn(pending, new Date(2026, 10, 5, 11, 1))).toEqual({ key: 'approvals.expires_in_min', n: 59, soon: true });
    expect(expiresIn(pending, new Date(2026, 10, 5, 11, 59, 30))).toEqual({ key: 'approvals.expires_in_min', n: 1, soon: true });
    expect(expiresIn(pending, new Date(2026, 10, 5, 12, 0))).toBeNull();
    expect(expiresIn({ ...pending, expiresAt: undefined }, now)).toBeNull();
    expect(expiresIn({ ...pending, status: 'approved' }, now)).toBeNull();
  });

  it('labels who decided, how and when, with one function for every screen', () => {
    expect(viaLabel(undefined, t)).toBe('');
    expect(viaLabel({ channel: 'whatsapp', reply: 'ya' }, t)).toBe('approvalcard.via_whatsapp_reply|{"reply":"ya"}');
    expect(viaLabel({ channel: 'console' }, t)).toBe('approvalcard.via_console');
    expect(viaLabel({ channel: 'mcp' }, t)).toBe('approvalcard.via_mcp');
    const approved = { ...pending, status: 'approved' as const, decidedBy: 'Sari', decidedVia: { channel: 'whatsapp' as const, reply: 'ya' }, decidedAt: now };
    expect(decisionLabel(approved, t, time, now)).toBe('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp_reply|{"reply":"ya"} · 10:42');
    expect(decisionLabel({ ...pending, status: 'rejected', decidedBy: 'Budi' }, t, time, now)).toBe('approvalcard.rejected|{"by":"Budi"}');
    expect(decisionLabel(pending, t, time, new Date(2026, 10, 6))).toBe('approvalcard.expired');
    expect(decisionLabel({ ...pending, status: 'approved', decidedVia: { channel: 'whatsapp' } }, t, time, now)).toBe('approvalcard.approved_plain · approvalcard.via_whatsapp');
    expect(decisionLabel({ ...pending, status: 'rejected' }, t, time, now)).toBe('approvalcard.rejected_plain');
    expect(decisionLabel(pending, t, time, now)).toBe('');
  });
});
