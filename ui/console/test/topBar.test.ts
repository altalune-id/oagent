// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent, h } from 'vue';

import { setRoute } from '#test/nuxt-globals';
import ConsoleTopBar from '~/components/layout/ConsoleTopBar.vue';
import { useNavCrumbs } from '~/components/layout/useNavCrumbs';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

beforeEach(() => {
  setActivePinia(createPinia());
  useAuthStore().me = { permissions: ['end_user:read'] } as never;
  useOrgsStore().items = [{ id: 'B6rTn1YkM8wQz3XpL5vGd', slug: 'acme', name: 'Acme', role: 'admin' }] as never;
  useProjectsStore().items = [{ id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'B6rTn1YkM8wQz3XpL5vGd', slug: 'support', name: 'Support Copilot' }] as never;
  useAgentsStore().items = [{ id: 'Q9vLm3XpT7kRw2NcY5bZd', projectId: 'x6MH9WPWjktMwCBKHy4pF', slug: 'refund-triage', name: 'Refund Triage', description: '' }];
});

function crumbsAt(path: string, params: Record<string, string>) {
  setRoute({ path, params });
  let out!: ReturnType<typeof useNavCrumbs>;
  const probe = defineComponent({
    setup() {
      out = useNavCrumbs();
      return () => h('i');
    },
  });
  mount(probe);
  return out;
}

describe('nav crumbs', () => {
  const p = '/o/acme/projects/support';
  const params = { org: 'acme', project: 'support' };

  it('trails org, project name and the active page, and lists both navs in the palette', () => {
    const { crumbs, palette } = crumbsAt(`${p}/agents`, params);
    expect(crumbs.value).toEqual([
      { label: 'Acme', to: '/o/acme' },
      { label: 'Support Copilot', to: p },
      { label: 'nav.agents' },
    ]);
    expect(palette.value.map(i => i.label)).toContain('Acme / members.title');
    expect(palette.value.map(i => i.label)).toContain('Support Copilot / nav.agents');
  });

  it('ends at the agent on Configure, then names Runs or the run inside it', () => {
    const agent = { ...params, agent: 'refund-triage' };
    const head = [{ label: 'Acme', to: '/o/acme' }, { label: 'Support Copilot', to: p }, { label: 'nav.agents', to: `${p}/agents` }];
    expect(crumbsAt(`${p}/agents/refund-triage`, agent).crumbs.value).toEqual([...head, { label: 'Refund Triage' }]);
    const named = { label: 'Refund Triage', to: `${p}/agents/refund-triage` };
    expect(crumbsAt(`${p}/agents/refund-triage/runs`, agent).crumbs.value).toEqual([...head, named, { label: 'agent.runs' }]);
    expect(crumbsAt(`${p}/agents/refund-triage/runs/r7X`, { ...agent, run: 'r7X' }).crumbs.value).toEqual([...head, named, { label: 'run.col_run' }]);
    expect(crumbsAt(`${p}/agents/refund-triage`, agent).palette.value.map(i => i.label)).toContain('Refund Triage / agent.runs');
  });

  it('trails only the org and its page outside a project', () => {
    const { crumbs, palette } = crumbsAt('/o/acme/settings/members', { org: 'acme' });
    expect(crumbs.value).toEqual([{ label: 'Acme', to: '/o/acme' }, { label: 'members.title' }]);
    expect(palette.value.every(i => i.label.startsWith('Acme / '))).toBe(true);
  });
});

describe('console top bar', () => {
  it('opens and closes the palette with Command or Control K', async () => {
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    const w = mount(ConsoleTopBar, { attachTo: document.body, global: { stubs: { ThemeFamilySwitcher: true, ThemeSwitcher: true } } });
    const dialog = () => w.get('dialog').element as HTMLDialogElement;
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }));
    await w.vm.$nextTick();
    await w.vm.$nextTick();
    expect(dialog().open).toBe(true);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', ctrlKey: true }));
    await w.vm.$nextTick();
    await w.vm.$nextTick();
    expect(dialog().open).toBe(false);
    w.unmount();
  });
});
