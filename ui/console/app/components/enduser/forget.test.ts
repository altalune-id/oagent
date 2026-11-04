import { describe, expect, it } from 'vitest';
import { sampleApprovals, sampleEndUsers, sampleRuns, sampleThreads } from '~/preview/samples';
import { forgetInApprovals, forgetInThreads, forgottenFrom, shownName } from './forget';

const t = (key: string) => key;
const [budi, sari] = sampleEndUsers().items;

describe('forgetting a deleted person', () => {
  it('shows "Deleted person" in place of the name once deleted', () => {
    expect(shownName('', true, t)).toBe('endusers.deleted_person');
    expect(shownName('Sari', false, t)).toBe('Sari');
    expect(shownName(undefined, undefined, t)).toBe('');
  });

  it('treats DMs and API threads as their own, and groups as shared', () => {
    expect(forgottenFrom(budi!)).toEqual({ id: 'eu_budi1', threadIds: ['thr_dm8Kq2Wx', 'thr_grp3Kd9Q'], ownThreadIds: ['thr_dm8Kq2Wx'] });
  });

  it('drops their DM and its runs, and blanks their group messages and name, matching by id', () => {
    const { threads, runs } = forgetInThreads(sampleThreads(), sampleRuns(), forgottenFrom(budi!));
    expect(threads.map(th => th.id)).toEqual(['thr_grp3Kd9Q', 'thr_api5Tn1R', 'thr_try2Hx6P']);
    expect(runs.some(r => r.threadId === 'thr_dm8Kq2Wx')).toBe(false);
    const group = threads[0]!;
    expect(group.people).toEqual([{ name: '', deleted: true }, { name: 'Sari', endUserId: 'eu_sari2' }]);
    const theirs = group.items.filter(i => i.kind === 'user' && i.deleted);
    expect(theirs).toHaveLength(1);
    expect(theirs[0]).toMatchObject({ text: '', speakerId: 'eu_budi1' });
    expect(theirs[0]).not.toHaveProperty('speaker');
    const expired = group.items.find(i => i.kind === 'approval' && i.approval.id === 'apr_del1');
    expect(expired?.kind === 'approval' && expired.approval).toMatchObject({ requestedBy: '', requesterDeleted: true });
    const tryIt = threads.find(th => th.id === 'thr_try2Hx6P')!;
    expect(tryIt.items.some(i => i.kind === 'user' && i.speaker === 'Budi' && !i.deleted)).toBe(true);
    expect(runs.find(r => r.threadId === 'thr_try2Hx6P')!.detail!.params!.name).toBe('Budi');
  });

  it('clears the last line of a shared conversation when it was theirs, and marks their runs', () => {
    const runs = sampleRuns().map(r => (r.threadId === 'thr_grp3Kd9Q' ? { ...r, personId: 'eu_sari2', detail: { key: 'runstatus.detail_awaiting', params: { name: 'Sari' } } } : r));
    const out = forgetInThreads(sampleThreads(), runs, forgottenFrom(sari!));
    const group = out.threads.find(th => th.id === 'thr_grp3Kd9Q')!;
    expect(group).toMatchObject({ lastSpeaker: '', lastText: '', lastDeleted: true });
    expect(out.runs.filter(r => r.threadId === 'thr_grp3Kd9Q').every(r => r.personDeleted && r.detail!.params!.name === '')).toBe(true);
    expect(out.runs.find(r => r.threadId === 'thr_dm8Kq2Wx')!.detail!.params!.name).toBe('Budi');
  });

  it('drops approvals from their own conversations and hides their name on shared ones', () => {
    const items = forgetInApprovals(sampleApprovals(), forgottenFrom(sari!));
    expect(items.find(i => i.approval.id === 'apr_ps2')!.approval).toMatchObject({ requestedBy: '', requesterDeleted: true, decidedBy: '', deciderDeleted: true });
    expect(items.find(i => i.approval.id === 'apr_kp1')!.approval.requestedBy).toBe('Budi');
    expect(items.find(i => i.approval.id === 'apr_tr3')!.approval.decidedBy).toBe('Budi');
    expect(forgetInApprovals(sampleApprovals(), forgottenFrom(budi!)).some(i => i.approval.id === 'apr_kp1')).toBe(false);
  });
});
