import type { ApprovalRequest, ApprovalStatus, DecidedVia } from '~/stores/approvals';
import { shownName } from '~/components/enduser/forget';

type Translate = (key: string, params?: Record<string, unknown>) => string;

// statusAt is the approval's status at a moment: a pending request past its expiry counts as expired.
export function statusAt(a: ApprovalRequest, now = new Date()): ApprovalStatus {
  return a.status === 'pending' && a.expiresAt && a.expiresAt <= now ? 'expired' : a.status;
}

// decideApproval records one decision on a pending request, or returns null when it was already decided or expired.
export function decideApproval(a: ApprovalRequest, approve: boolean, by: string, via: DecidedVia, reason = '', at = new Date()): ApprovalRequest | null {
  if (statusAt(a, at) !== 'pending') {
    return null;
  }
  return { ...a, status: approve ? 'approved' : 'rejected', decidedBy: by, decidedVia: via, decidedAt: at, reason: reason.trim() || undefined };
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
  const head = t(status === 'approved' ? 'approvalcard.approved' : 'approvalcard.rejected', { by: shownName(a.decidedBy, a.deciderDeleted, t) });
  return [head, viaLabel(a.decidedVia, t), a.decidedAt ? time(a.decidedAt) : ''].filter(Boolean).join(' · ');
}
