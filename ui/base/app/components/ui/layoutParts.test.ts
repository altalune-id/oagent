// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import Breadcrumbs from './Breadcrumbs.vue';
import EmptyState from './EmptyState.vue';
import KeyValue from './KeyValue.vue';
import PageHeader from './PageHeader.vue';
import Section from './Section.vue';

describe('breadcrumbs', () => {
  it('is a named dense trail: links, optional mono slugs and the current page as plain text', () => {
    const w = mount(Breadcrumbs, { props: { crumbs: [{ label: 'Acme', to: '/o/acme' }, { label: 'refund-triage', to: '/o/acme/p', mono: true }, { label: 'Agents' }] } });
    expect(w.get('nav').classes()).toEqual(['breadcrumbs', 'min-w-0', 'text-ui']);
    expect(w.get('nav').attributes('aria-label')).toBe('Breadcrumb');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toEqual(['/o/acme', '/o/acme/p']);
    expect(w.findAll('li')[1]!.get('span').classes()).toEqual(['font-mono', 'text-xs']);
    expect(w.get('[aria-current="page"]').text()).toBe('Agents');
    expect(w.get('[aria-current="page"]').element.tagName).toBe('SPAN');
  });
});

describe('pageHeader', () => {
  it('has one h1, a description and an actions area', () => {
    const w = mount(PageHeader, { props: { title: 'Agents', description: 'Agents in Support Copilot' }, slots: { actions: '<button>New agent</button>' } });
    expect(w.findAll('h1').map(h => h.text())).toEqual(['Agents']);
    expect(w.get('p').text()).toBe('Agents in Support Copilot');
    expect(w.text()).toContain('New agent');
    expect(w.find('nav').exists()).toBe(false);
    expect(w.find('.badge').exists()).toBe(false);
  });

  it('shows a mono title, a preview badge, status and breadcrumbs with the current page marked', () => {
    const w = mount(PageHeader, {
      props: { title: 'r7Xp2KqLvN9sT4bWzY1cD', mono: true, preview: 'Preview', crumbs: [{ label: 'Agents', to: '/a' }, { label: 'Run' }], crumbsLabel: 'You are here' },
      slots: { status: '<p>Waiting</p>' },
    });
    expect(w.get('h1').classes()).toContain('font-mono');
    expect(w.get('.badge').text()).toBe('Preview');
    expect(w.get('nav').attributes('aria-label')).toBe('You are here');
    expect(w.getComponent(RouterLinkStub).props('to')).toBe('/a');
    expect(w.get('[aria-current="page"]').text()).toBe('Run');
    expect(w.text()).toContain('Waiting');
  });
});

describe('section', () => {
  it('is a section named by its h2', () => {
    const w = mount(Section, { props: { title: 'Model', description: 'Which model runs' }, slots: { default: '<p>body</p>', actions: '<button>Edit</button>' } });
    expect(w.attributes('aria-labelledby')).toBe(w.get('h2').attributes('id'));
    expect(w.text()).toContain('Which model runs');
    expect(w.text()).toContain('Edit');
    expect(w.classes()).not.toContain('border-error');
  });

  it('marks a danger zone', () => {
    const w = mount(Section, { props: { title: 'Delete', tone: 'danger' } });
    expect(w.classes()).toContain('border-error');
    expect(w.classes()).not.toContain('border-base-300');
  });

  it('pads the body, or draws the design header bar over a flush body for rows', () => {
    const padded = mount(Section, { props: { title: 'Limits' }, slots: { default: '<p>body</p>' } });
    expect(padded.get('h2').classes()).toEqual(['text-ui', 'font-semibold']);
    expect(padded.get('p').element.parentElement!.className).toBe('flex flex-col gap-4 px-4 pb-4');
    const flush = mount(Section, { props: { title: 'Run stats', flush: true }, slots: { default: '<p>body</p>' } });
    expect(flush.get('h2').element.parentElement!.parentElement!.classList).toContain('border-b');
    expect(flush.get('p').element.parentElement!.className).toBe('');
  });
});

describe('emptyState', () => {
  it('shows a heading at the asked level, the body and actions', () => {
    const w = mount(EmptyState, { props: { title: 'No agents yet', body: 'Create one', headingLevel: 3 }, slots: { actions: '<button>New agent</button>' } });
    expect(w.get('h3').text()).toBe('No agents yet');
    expect(w.text()).toContain('Create one');
    expect(w.text()).toContain('New agent');
  });

  it('defaults to h2 with no body', () => {
    const w = mount(EmptyState, { props: { title: 'Nothing' } });
    expect(w.get('h2').text()).toBe('Nothing');
    expect(w.find('p').exists()).toBe(false);
  });
});

describe('keyValue', () => {
  it('is a description list with mono values and copy buttons', () => {
    const w = mount(KeyValue, { props: { items: [{ label: 'Model', value: 'large-2' }, { label: 'ID', value: 'kT4v', mono: true, copy: true }], copyLabel: 'Copy {label}' } });
    expect(w.findAll('dt').map(d => d.text())).toEqual(['Model', 'ID']);
    expect(w.findAll('dd')[1]!.get('span').classes()).toContain('font-mono');
    expect(w.get('button').attributes('aria-label')).toBe('Copy ID');
  });

  it('draws divided status rows with a fixed label column that stacks on phones', () => {
    const w = mount(KeyValue, { props: { items: [{ label: 'Status', value: 'Waiting for approval' }] } });
    expect(w.classes()).toEqual(['divide-y', 'divide-base-300']);
    expect(w.get('dl > div').classes()).toEqual(['grid', 'grid-cols-[9.5rem_minmax(0,1fr)]', 'gap-3', 'px-4', 'py-2', 'max-sm:grid-cols-1', 'max-sm:gap-0']);
    expect(w.get('dt').classes()).toEqual(['text-ui', 'text-base-content/70']);
  });

  it('lets a slot replace one value', () => {
    const w = mount(KeyValue, { props: { items: [{ label: 'Status' }] }, slots: { 'value-0': '<b>Live</b>' } });
    expect(w.get('dd').text()).toBe('Live');
  });
});
