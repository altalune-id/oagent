import type { ApprovalItem, ApprovalRequest } from '~/stores/approvals';
import type { EndUser } from '~/stores/endUsers';
import type { RunSummary, ThreadDetail } from '~/stores/threads';

type Translate = (key: string, params?: Record<string, unknown>) => string;

export interface ForgottenPerson {
  id: string;
  threadIds: string[];
  ownThreadIds: string[];
}

// shownName shows a person's name, or "Deleted person" once their data was deleted.
export function shownName(name: string | undefined, deleted: boolean | undefined, t: Translate): string {
  return deleted ? t('endusers.deleted_person') : name ?? '';
}

// forgottenFrom records who was deleted and which conversations were theirs alone (DMs and API threads).
export function forgottenFrom(user: EndUser): ForgottenPerson {
  return {
    id: user.id,
    threadIds: user.threads.map(th => th.id),
    ownThreadIds: user.threads.filter(th => th.kind !== 'whatsapp_group').map(th => th.id),
  };
}

function forgetApproval(a: ApprovalRequest, p: ForgottenPerson): ApprovalRequest {
  const asked = a.requestedById === p.id;
  const decided = a.decidedById === p.id;
  return {
    ...a,
    ...(asked ? { requestedBy: '', requesterDeleted: true } : {}),
    ...(decided ? { decidedBy: '', deciderDeleted: true } : {}),
  };
}

// forgetInThreads drops the person's own conversations and their runs, and blanks their messages and name in shared ones.
export function forgetInThreads(threads: ThreadDetail[], runs: RunSummary[], p: ForgottenPerson): { threads: ThreadDetail[], runs: RunSummary[] } {
  const shared = (id: string) => p.threadIds.includes(id);
  const keptThreads = threads.filter(th => !p.ownThreadIds.includes(th.id)).map((th) => {
    if (!shared(th.id)) {
      return th;
    }
    const lastGone = th.lastSpeakerId === p.id;
    return {
      ...th,
      people: th.people.map(x => (x.endUserId === p.id ? { name: '', deleted: true } : x)),
      ...(lastGone ? { lastSpeaker: '', lastText: '', lastDeleted: true } : {}),
      items: th.items.map((i) => {
        if (i.kind === 'user' && i.speakerId === p.id) {
          return { id: i.id, kind: 'user' as const, speakerId: p.id, text: '', deleted: true, at: i.at };
        }
        return i.kind === 'approval' ? { ...i, approval: forgetApproval(i.approval, p) } : i;
      }),
    };
  });
  const keptRuns = runs.filter(r => !p.ownThreadIds.includes(r.threadId)).map(r => (r.personId === p.id ? { ...r, personDeleted: true, detail: r.detail && { ...r.detail, params: { ...r.detail.params, name: '' } } } : r));
  return { threads: keptThreads, runs: keptRuns };
}

// forgetInApprovals drops approvals from the person's own conversations and hides their name on shared ones.
export function forgetInApprovals(items: ApprovalItem[], p: ForgottenPerson): ApprovalItem[] {
  return items
    .filter(i => !p.ownThreadIds.includes(i.threadId))
    .map(i => ({ ...i, approval: forgetApproval(i.approval, p) }));
}
