// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it, vi } from 'vitest';

import { MemberSchema } from '#gen/member/v1/member_pb';
import { ProjectEnvironment } from '#gen/project/v1/project_pb';
import { mountPage } from '#test/mount-page';
import { deferred, setClient, setRoute } from '#test/nuxt-globals';
import OrgOverview from '~/pages/o/[org]/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

describe('org overview', () => {
  it('heads the page with the org and its mono id, and lists its projects as dense rows with the environment tag', async () => {
    setActivePinia(createPinia());
    const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
    useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
    useProjectsStore().items = [
      { id: 'P1', orgId, slug: 'acme-prod', name: 'Acme production', environment: ProjectEnvironment.PRODUCTION },
      { id: 'P2', orgId, slug: 'acme-misc', name: 'Misc' },
    ] as never;
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    const w = await mountPage(OrgOverview);
    expect(w.get('h1').classes()).toEqual(['text-xl', 'font-semibold', 'break-words']);
    expect(w.get('header .font-mono').text()).toBe(orgId);
    const rows = w.findAll('[data-testid="org-projects"] li');
    expect(rows.map(r => r.classes())).toEqual([['flex', 'items-center', 'gap-2', 'px-4', 'py-2', 'text-ui'], ['flex', 'items-center', 'gap-2', 'px-4', 'py-2', 'text-ui']]);
    expect(rows[0]!.get('.font-mono').text()).toBe('acme-prod');
    expect(rows[0]!.get('.badge').text()).toBe('environment.production');
    expect(rows[1]!.find('.badge').exists()).toBe(false);
  });
});

describe('org overview stats', () => {
  const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
  const seed = () => {
    setActivePinia(createPinia());
    useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
  };

  it('counts the real projects and members, with a dash while members load', async () => {
    seed();
    const members = ['FtGrdYskgDVDf8apKsMdn', '5jeWP8ut8V9AC9GBMMHfx', 'jzdsjzt2UHJ3Cmr3uN2VP'].map(userId => create(MemberSchema, { userId, role: 'member' }));
    const pending = deferred<{ members: typeof members }>();
    const listMembers = vi.fn().mockReturnValue(pending.promise);
    setClient('$memberClient', { listMembers });
    const w = await mountPage(OrgOverview);
    expect(w.findAll('.stat').map(s => s.text())).toEqual(['nav.projects0', 'stat.members—']);
    pending.resolve({ members });
    await flushPromises();
    expect(listMembers).toHaveBeenCalledWith({ orgId });
    expect(w.findAll('.stat').map(s => s.text())).toEqual(['nav.projects0', 'stat.members3']);
    expect(w.get('.stat-title').classes()).toEqual(expect.arrayContaining(['text-xs', 'text-base-content/70']));
  });

  it('shows a dash, not a number, when members or projects cannot be read', async () => {
    seed();
    setClient('$memberClient', { listMembers: vi.fn().mockRejectedValue(new Error('denied')) });
    useProjectsStore().error = { message: 'down' } as never;
    const w = await mountPage(OrgOverview);
    await flushPromises();
    expect(w.findAll('.stat').map(s => s.text())).toEqual(['nav.projects—', 'stat.members—']);
  });
});
