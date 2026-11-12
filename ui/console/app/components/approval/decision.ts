import type { ApprovalRequest, ApprovalStatus, DecidedVia } from '~/stores/approvals';
import { shownName } from '~/components/enduser/forget';

type Translate = (key: string, params?: Record<string, unknown>) => string;

// statusAt is the approval's status at a moment: a pending request past its expiry counts as expired.
export function statusAt(a: ApprovalRequest, now = new Date()): ApprovalStatus {
  return a.status === 'pending' && a.expiresAt && a.expiresAt <= now ? 'expired' : a.status;
}

// expiresIn is the countdown for a pending request at minute granularity, "soon" under an hour, or null when it has none or is no longer pending.
export function expiresIn(a: ApprovalRequest, now = new Date()): { key: string, n: number, soon: boolean } | null {
  if (!a.expiresAt || statusAt(a, now) !== 'pending') {
    return null;
  }
  const minutes = Math.max(1, Math.ceil((a.expiresAt.getTime() - now.getTime()) / 60_000));
  return minutes < 60 ? { key: 'approvals.expires_in_min', n: minutes, soon: true } : { key: 'approvals.expires_in_h', n: Math.floor(minutes / 60), soon: false };
}

// viaLabel names where a decision was made, such as a WhatsApp reply or the console.
export function viaLabel(via: DecidedVia | undefined, t: Translate): string {
  if (!via) {
    return '';
  }
  return via.reply ? t(`approvalcard.via_${via.channel}_reply`, { reply: via.reply }) : t(`approvalcard.via_${via.channel}`);
}

// decisionLabel is the one "decided by" line every screen shows for a decided or expired request, or '' otherwise.
export function decisionLabel(a: ApprovalRequest, t: Translate, time: (d: Date) => string, now = new Date()): string {
  const status = statusAt(a, now);
  if (status === 'expired') {
    return t('approvalcard.expired');
  }
  if (status !== 'approved' && status !== 'rejected') {
    return '';
  }
  const by = shownName(a.decidedBy, a.deciderDeleted, t);
  const head = by ? t(status === 'approved' ? 'approvalcard.approved' : 'approvalcard.rejected', { by }) : t(status === 'approved' ? 'approvalcard.approved_plain' : 'approvalcard.rejected_plain');
  return [head, viaLabel(a.decidedVia, t), a.decidedAt ? time(a.decidedAt) : ''].filter(Boolean).join(' · ');
}
