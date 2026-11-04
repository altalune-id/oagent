// @vitest-environment happy-dom
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import Overview from '~/pages/o/[org]/projects/[project]/index.vue';
import { useChannelsStore } from '~/stores/channels';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useTryItStore } from '~/stores/tryIt';

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

  it('leads with the set-up checklist, done from the sample providers and servers up to the next step', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'A1', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan' }] }) });
    const w = await mountPage(Overview);
    const steps = w.findAll('[data-testid="setup-checklist"] li');
    expect(steps).toHaveLength(6);
    expect(steps.map(s => s.text().includes('setup.done'))).toEqual([true, true, true, false, false, false]);
    expect(steps[3]!.attributes('aria-current')).toBe('step');
    expect(steps[3]!.get('a.btn-primary').text()).toContain('setup.start');
    expect(steps[3]!.get('a.btn-primary').attributes('aria-label')).toBe('setup.start_label|{"step":"setup.step_try"}');
    const bar = w.get('[data-testid="setup-checklist"] progress');
    expect([bar.attributes('value'), bar.attributes('max'), bar.attributes('aria-valuetext')]).toEqual(['3', '6', 'setup.progress|{"done":3,"total":6}']);
    expect(w.findAll('[data-testid="setup-checklist"] a.btn-primary')).toHaveLength(1);
    expect(steps.map(s => s.find('a.link').exists())).toEqual([true, true, true, true, true, true]);
    expect(steps[4]!.text()).toContain('setup.not_started');
  });

  it('marks Try it and the channel done from what happened in this session', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'A1', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', publishedVersion: 4 }] }) });
    useTryItStore().markTried('A1');
    useChannelsStore().save('A1', { id: 'chn_1', provider: 'openwa', status: 'connected', baseUrl: 'https://openwa.example.id', deviceId: 'dev_1', webhookUrl: '', rejected24h: 0, rejectedLastHour: 0 });
    const w = await mountPage(Overview);
    const steps = w.findAll('[data-testid="setup-checklist"] li');
    expect(steps.map(s => s.text().includes('setup.done'))).toEqual([true, true, true, true, true, true]);
  });

  it('leaves the agent unconfigured when its runtime has no provider', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'A2', projectId, slug: 'other', name: 'Other' }] }) });
    const w = await mountPage(Overview);
    const steps = w.findAll('[data-testid="setup-checklist"] li');
    expect(steps.map(s => s.text().includes('setup.done'))).toEqual([true, true, false, false, false, false]);
  });

  it('shows a load error with Retry instead of an empty list', async () => {
    const listAgents = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValue({ agents: [] });
    setClient('$agentClient', { listAgents });
    const w = await mountPage(Overview);
    expect(w.text()).not.toContain('setup.agents_first');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(listAgents).toHaveBeenCalledTimes(2);
    expect(w.text()).toContain('setup.agents_first');
  });
});
