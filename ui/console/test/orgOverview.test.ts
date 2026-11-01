// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { MemberSchema } from '#gen/member/v1/member_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import OrgOverview from '~/pages/o/[org]/index.vue';
import { useOrgsStore } from '~/stores/orgs';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  setRoute({ path: '/o/acme', params: { org: 'acme' } });
});

describe('org overview stats', () => {
  it('counts the real members and shows no made-up agent count', async () => {
    const members = ['FtGrdYskgDVDf8apKsMdn', '5jeWP8ut8V9AC9GBMMHfx', 'jzdsjzt2UHJ3Cmr3uN2VP'].map(userId => create(MemberSchema, { userId, role: 'member' }));
    const listMembers = vi.fn().mockResolvedValue({ members });
    setClient('$memberClient', { listMembers });
    const w = await mountPage(OrgOverview);
    await flushPromises();
    expect(listMembers).toHaveBeenCalledWith({ orgId });
    const stats = w.findAll('.stat').map(s => s.text());
    expect(stats).toEqual(['nav.projects0', 'stat.members3']);
  });

  it('shows a dash, not a number, when members cannot be read', async () => {
    setClient('$memberClient', { listMembers: vi.fn().mockRejectedValue(new Error('denied')) });
    const w = await mountPage(OrgOverview);
    await flushPromises();
    expect(w.findAll('.stat').map(s => s.text())).toContain('stat.members—');
  });
});
