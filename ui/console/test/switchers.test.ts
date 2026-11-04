// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProjectEnvironment } from '#gen/project/v1/project_pb';
import { fakeStorage, setRoute } from '#test/nuxt-globals';
import OrgSelector from '~/components/org/OrgSelector.vue';
import ProjectSelector from '~/components/project/ProjectSelector.vue';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useRecentStore } from '~/stores/recent';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const rows = (w: ReturnType<typeof mount>) => w.findAll('ul.menu li > button').map(b => b.text());

beforeEach(() => {
  setActivePinia(createPinia());
  vi.stubGlobal('localStorage', fakeStorage());
  useAuthStore().me = { permissions: ['project:read'] } as never;
  useOrgsStore().items = [
    { id: orgId, slug: 'acme', name: 'Acme', role: 'admin' },
    { id: 'Z1', slug: 'north', name: 'Northwind Labs', role: 'member' },
    { id: 'Z2', slug: 'globex', name: 'Globex', role: 'member' },
  ] as never;
  useProjectsStore().items = [
    { id: 'P1', orgId, slug: 'support', name: 'Support Copilot', environment: ProjectEnvironment.PRODUCTION },
    { id: 'P2', orgId, slug: 'invoices', name: 'Invoice Review' },
  ] as never;
});

describe('org switcher', () => {
  it('reads OAgent / org, lists orgs last opened first in the design card, with no search and no title', () => {
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    useRecentStore().visit('globex');
    useRecentStore().visit('acme');
    const w = mount(OrgSelector);
    const trigger = w.get('button[popovertarget]');
    expect(trigger.text().replace(/\s+/g, '')).toContain('OAgent/Acme');
    const logo = trigger.get('img');
    expect(logo.attributes('src')).toMatch(/\/static\/oagent-mark\.svg$/);
    expect(logo.attributes('alt')).toBe('');
    expect(trigger.classes()).toEqual(expect.arrayContaining(['btn', 'btn-ghost', 'btn-sm', 'w-full', 'justify-between', 'text-ui']));
    expect(w.get('[popover]').classes()).toEqual(expect.arrayContaining(['dropdown', 'card', 'card-sm', 'z-1', 'w-64', 'border', 'border-base-300', 'bg-base-100', 'shadow-sm']));
    expect(w.find('input[type="search"]').exists()).toBe(false);
    expect(w.find('.menu-title').exists()).toBe(false);
    expect(rows(w).slice(0, 3)).toEqual(['Acme', 'Globex', 'Northwind Labs']);
    expect(w.get('.menu-active').text()).toBe('Acme');
  });
});

describe('org switcher creation', () => {
  it('keeps New organization at the end when the person may create one', () => {
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    expect(rows(mount(OrgSelector))).not.toContain('org.new');
    useAuthStore().me = { permissions: [], orgCreation: true } as never;
    expect(rows(mount(OrgSelector)).at(-1)).toBe('org.new');
  });
});

describe('project switcher', () => {
  it('shows the name over its environment badge, titles the list and orders it last opened first', () => {
    setRoute({ path: '/o/acme/projects/support', params: { org: 'acme', project: 'support' } });
    useRecentStore().visit('acme', 'invoices');
    const w = mount(ProjectSelector);
    const trigger = w.get('button[popovertarget]');
    expect(trigger.classes()).toEqual(expect.arrayContaining(['h-auto', 'min-h-8', 'py-1']));
    expect(trigger.get('.font-medium').text()).toBe('Support Copilot');
    expect(trigger.get('.badge').classes()).toEqual(['badge', 'badge-xs']);
    expect(trigger.get('.badge').text()).toBe('environment.production');
    expect(w.get('.menu-title').text()).toBe('project.group_title');
    expect(rows(w)).toEqual(['Invoice Review', 'Support Copilot']);
  });

  it('shows no badge for a project without a tag', () => {
    setRoute({ path: '/o/acme/projects/invoices', params: { org: 'acme', project: 'invoices' } });
    const trigger = mount(ProjectSelector).get('button[popovertarget]');
    expect(trigger.get('.font-medium').text()).toBe('Invoice Review');
    expect(trigger.find('.badge').exists()).toBe(false);
  });

  it('offers New project only with project:write', () => {
    setRoute({ path: '/o/acme/projects/support', params: { org: 'acme', project: 'support' } });
    useAuthStore().me = { permissions: ['project:read', 'project:write'] } as never;
    expect(rows(mount(ProjectSelector))).toContain('project.new');
  });
});
