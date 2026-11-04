// @vitest-environment happy-dom
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import Overview from '~/pages/o/[org]/projects/[project]/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }] as never;
  setRoute({ path: '/o/acme/projects/web', params: { org: 'acme', project: 'web' } });
});

describe('project overview', () => {
  it('heads the page with the project and its mono id, and lists agents as dense rows with a draft badge', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [
      { id: 'A1', projectId, slug: 'support', name: 'Support', description: 'Answers tickets', publishedVersion: 3, unpublishedChanges: true },
      { id: 'A2', projectId, slug: 'triage', name: 'Triage', publishedVersion: 1 },
    ] }) });
    const w = await mountPage(Overview);
    expect(w.get('h1').classes()).toEqual(['text-xl', 'font-semibold', 'break-words']);
    expect(w.get('header .font-mono').text()).toBe(projectId);
    const rows = w.findAll('[data-testid="project-agents"] li');
    expect(rows.map(r => r.get('.font-mono').text())).toEqual(['support', 'triage']);
    expect(rows[0]!.get('.badge').text()).toBe('agent.draft_changes');
    expect(rows[0]!.get('.badge').classes()).toEqual(expect.arrayContaining(['badge', 'badge-sm', 'badge-outline']));
    expect(rows[0]!.get('p').text()).toBe('Answers tickets');
    expect(rows[0]!.get('p').classes()).toEqual(['truncate', 'text-xs', 'text-base-content/70']);
    expect(rows[1]!.find('p').exists()).toBe(false);
    expect(rows[1]!.find('.badge').exists()).toBe(false);
  });

  it('shows a load error with Retry instead of an empty list', async () => {
    const listAgents = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValue({ agents: [] });
    setClient('$agentClient', { listAgents });
    const w = await mountPage(Overview);
    expect(w.text()).not.toContain('agent.empty');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(listAgents).toHaveBeenCalledTimes(2);
    expect(w.text()).toContain('agent.empty');
  });
});
