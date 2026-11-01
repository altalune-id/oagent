// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import EmptyState from './EmptyState.vue';
import KeyValue from './KeyValue.vue';
import PageHeader from './PageHeader.vue';
import Section from './Section.vue';

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
    expect(mount(Section, { props: { title: 'Delete', tone: 'danger' } }).classes()).toContain('border-error');
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

  it('lets a slot replace one value', () => {
    const w = mount(KeyValue, { props: { items: [{ label: 'Status' }] }, slots: { 'value-0': '<b>Live</b>' } });
    expect(w.get('dd').text()).toBe('Live');
  });
});
