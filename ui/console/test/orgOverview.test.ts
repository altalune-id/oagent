// @vitest-environment happy-dom
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it } from 'vitest';

import { ProjectEnvironment } from '#gen/project/v1/project_pb';
import { mountPage } from '#test/mount-page';
import { setRoute } from '#test/nuxt-globals';
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
    expect(w.find('.stat').exists()).toBe(false);
    const rows = w.findAll('[data-testid="org-projects"] li');
    expect(rows.map(r => r.classes())).toEqual([['flex', 'items-center', 'gap-2', 'px-4', 'py-2', 'text-ui'], ['flex', 'items-center', 'gap-2', 'px-4', 'py-2', 'text-ui']]);
    expect(rows[0]!.get('.font-mono').text()).toBe('acme-prod');
    expect(rows[0]!.get('.badge').text()).toBe('environment.production');
    expect(rows[1]!.find('.badge').exists()).toBe(false);
  });
});
