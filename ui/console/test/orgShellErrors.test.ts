// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { nuxtError, setClient, setRoute } from '#test/nuxt-globals';
import AppNavbar from '~/components/layout/AppNavbar.vue';
import OrgShell from '~/pages/o/[org].vue';
import ConfigurePage from '~/pages/o/[org]/projects/[project]/agents/[agent]/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useOrgsStore().loaded = true;
  vi.stubGlobal('definePageMeta', vi.fn());
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions: ['agent:read'] })) });
  setRoute({ path: '/o/acme/projects/web', params: { org: 'acme', project: 'web' } });
});

describe('errors inside an org', () => {
  it('keep the sidebar and top bar, with the error page in place of the page', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [] }) });
    const Missing = defineComponent({
      setup() {
        throw Object.assign(new Error('Agent not found'), { statusCode: 404, statusMessage: 'Agent not found' });
      },
      render: () => null,
    });
    const w = await mountPage(OrgShell, { global: { stubs: { NuxtPage: Missing, ProjectSidebar: { template: '<nav data-testid="sidebar" />' }, AppSidebar: true, ConsoleTopBar: true } } });
    expect(w.find('[data-testid="sidebar"]').exists()).toBe(true);
    expect(w.find('console-top-bar-stub').exists()).toBe(true);
    expect(w.get('h1').text()).toBe('error_page.not_found_title');
    expect(w.text()).toContain('Agent not found');
    expect(w.findAll('main')).toHaveLength(1);
  });

  const stubs = { ProjectSidebar: true, AppSidebar: true, AgentSidebar: true, ConsoleTopBar: true };

  function failsOnce() {
    let throws = 1;
    return defineComponent({
      setup() {
        if (throws-- > 0) {
          throw Object.assign(new Error('Failed to load agents'), { statusCode: 502, statusMessage: 'Failed to load agents' });
        }
      },
      render: () => h('p', { 'data-testid': 'page' }, 'page'),
    });
  }

  it('mount the page again on Retry', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [] }) });
    const w = await mountPage(OrgShell, { global: { stubs: { ...stubs, NuxtPage: failsOnce() } } });
    expect(w.find('[data-testid="page"]').exists()).toBe(false);
    await w.findAll('button').find(b => b.text().includes('common.retry'))!.trigger('click');
    await flushPromises();
    expect(w.find('[data-testid="page"]').exists()).toBe(true);
    expect(w.text()).not.toContain('error_page.failed_title');
  });

  it('clear the error when the route changes', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [] }) });
    const w = await mountPage(OrgShell, { global: { stubs: { ...stubs, NuxtPage: failsOnce() } } });
    expect(w.text()).toContain('error_page.failed_title');
    setRoute({ path: '/o/acme/projects/web/agents', params: { org: 'acme', project: 'web' } });
    await flushPromises();
    expect(w.find('[data-testid="page"]').exists()).toBe(true);
    expect(w.text()).not.toContain('error_page.failed_title');
  });

  it('say a failed project load is a failure with Retry, not "not found"', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockRejectedValue(new Error('offline')) });
    const thrown = await new Promise((resolve) => {
      mount(defineComponent({
        components: { OrgShell },
        errorCaptured: (e) => {
          resolve(e);
          return false;
        },
        template: '<Suspense><OrgShell /></Suspense>',
      }), { global: { stubs: { NuxtPage: true, ProjectSidebar: true, AppSidebar: true, ConsoleTopBar: true } } });
    });
    expect(thrown).toMatchObject({ statusCode: 502, statusMessage: 'error.load_projects' });
  });

  it('say a failed agents load is a failure, not "agent not found"', async () => {
    useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }];
    setRoute({ path: '/o/acme/projects/web/agents/support', params: { org: 'acme', project: 'web', agent: 'support' } });
    setClient('$agentClient', { listAgents: vi.fn().mockRejectedValue(new Error('offline')) });
    const thrown = await new Promise((resolve) => {
      mount(defineComponent({
        components: { ConfigurePage },
        errorCaptured: (e) => {
          resolve(e);
          return false;
        },
        template: '<Suspense><ConfigurePage /></Suspense>',
      }));
    });
    expect(thrown).toMatchObject({ statusCode: 502, statusMessage: 'error.load_agents' });
  });
});

describe('the slim navbar', () => {
  it('shows on an org route when the org itself failed, so the error page keeps a way out', () => {
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    const stubs = { ThemeFamilySwitcher: true, ThemeSwitcher: true, UserMenu: true };
    expect(mount(AppNavbar, { global: { stubs } }).html()).toBe('<!--v-if-->');
    nuxtError.value = { statusCode: 502 };
    expect(mount(AppNavbar, { global: { stubs } }).find('a').exists()).toBe(true);
  });
});
