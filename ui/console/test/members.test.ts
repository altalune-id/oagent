// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { MemberSchema } from '#gen/member/v1/member_pb';
import { mountPage } from '#test/mount-page';
import { deferred, setClient, setRoute } from '#test/nuxt-globals';
import Members from '~/pages/o/[org]/settings/members.vue';
import { useOrgsStore } from '~/stores/orgs';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const owner = create(MemberSchema, { userId: 'FtGrdYskgDVDf8apKsMdn', email: 'ann@example.com', name: 'Ann Lee', role: 'owner' });

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useOrgsStore().loaded = true;
  setRoute({ path: '/o/acme/settings/members', params: { org: 'acme' } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { userId: owner.userId, permissions: ['member:read', 'member:write'] })) });
  setClient('$inviteClient', { listInvites: vi.fn().mockResolvedValue({ invites: [], drafts: [] }) });
});

describe('members page', () => {
  it('shows the page and a loading status before the members arrive', async () => {
    const pending = deferred<{ members: typeof owner[] }>();
    setClient('$memberClient', { listMembers: vi.fn().mockReturnValue(pending.promise) });
    const w = await mountPage(Members);
    expect(w.get('h1').text()).toBe('members.title');
    expect(w.find('[role="status"]').text()).toContain('common.loading');
    pending.resolve({ members: [owner] });
    await flushPromises();
    expect(w.text()).toContain('ann@example.com');
  });

  it('offers Retry when the members fail to load, and loads them again', async () => {
    const listMembers = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ members: [owner] });
    setClient('$memberClient', { listMembers });
    const w = await mountPage(Members);
    await flushPromises();
    const retry = w.findAll('button').find(b => b.text().includes('common.retry'))!;
    await retry.trigger('click');
    await flushPromises();
    expect(listMembers).toHaveBeenCalledTimes(2);
    expect(w.text()).toContain('ann@example.com');
  });
});
