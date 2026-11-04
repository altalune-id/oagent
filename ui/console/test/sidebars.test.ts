// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import AgentSidebar from '~/components/agent/AgentSidebar.vue';
import ProjectSidebar from '~/components/project/ProjectSidebar.vue';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const triage = { id: 'Q9vLm3XpT7kRw2NcY5bZd', projectId, slug: 'refund-triage', name: 'Refund Triage', description: '', publishedVersion: 12, unpublishedChanges: true };

beforeEach(async () => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner' }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'support', name: 'Support Copilot' }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { source: 'oidc', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', name: 'Sam Rivera', email: 'sam@acme.test' })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [triage] }) });
  await useAuthStore().load();
});

describe('project sidebar', () => {
  it('puts the org pages above the project row, the project pages below it, and the account row at the bottom', async () => {
    setRoute({ path: '/o/acme/projects/support/agents', params: { org: 'acme', project: 'support' } });
    const w = await mountPage(ProjectSidebar);
    const lists = w.findAll('nav > ul').map(ul => ul.findAll('li').map(li => li.text()));
    expect(lists).toEqual([
      ['nav.org_overview', 'nav.projects', 'members.title', 'apikeys.title', 'nav.org_settings'],
      ['nav.overview', 'nav.agents', 'webhooks.title', 'nav.settings'],
    ]);
    expect(w.get('nav > ul:last-child a.menu-active').text()).toBe('nav.agents');
    expect(w.get('[data-sidebar-footer]').text()).toContain('Sam Rivera');
  });
});

describe('agent sidebar', () => {
  it('leads with a back link to Agents, names the agent and its version state, then Configure and Runs', async () => {
    setRoute({ path: '/o/acme/projects/support/agents/refund-triage', params: { org: 'acme', project: 'support', agent: 'refund-triage' } });
    const w = await mountPage(AgentSidebar);
    const [back, pages] = w.findAll('nav > ul');
    expect(back!.getComponent(RouterLinkStub).props('to')).toBe('/o/acme/projects/support/agents');
    expect(back!.get('[data-icon="lucide:arrow-left"]').exists()).toBe(true);
    expect(w.get('nav > div').text()).toContain('Refund Triage');
    expect(w.get('nav > div').text()).toContain('agent.version_draft|{"n":12}');
    expect(pages!.findAll('li').map(li => li.text())).toEqual(['agent.configure', 'agent.runs']);
    expect(pages!.get('a.menu-active').text()).toBe('agent.configure');
  });

  it('opens a run as the icon rail and other agent pages in full', async () => {
    setRoute({ path: '/o/acme/projects/support/agents/refund-triage/runs/r7X', params: { org: 'acme', project: 'support', agent: 'refund-triage', run: 'r7X' } });
    const w = await mountPage(AgentSidebar);
    expect(useUIStore().rail).toBe(true);
    expect(w.get('aside').classes()).toContain('lg:w-16');
    expect(w.get('nav > ul:last-child li').attributes('data-tip')).toBe('agent.configure');
  });

  it('shows the org switcher as the plain full-width row, with no role marker', async () => {
    setRoute({ path: '/o/acme/projects/support/agents', params: { org: 'acme', project: 'support' } });
    const w = await mountPage(ProjectSidebar);
    expect(w.find('[data-testid="org-role"]').exists()).toBe(false);
    expect(w.get('aside > div').element.children).toHaveLength(1);
  });
});
