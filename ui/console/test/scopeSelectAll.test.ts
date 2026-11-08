// @vitest-environment happy-dom
import type { VueWrapper } from '@vue/test-utils';
import scopes from '@shared/scope/scopes.json';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setClient } from '#test/nuxt-globals';
import APIKeyCreateModal from '~/components/apikey/create/APIKeyCreateModal.vue';
import PersonalKeyCreateForm from '~/components/apikey/personal/PersonalKeyCreateForm.vue';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const orgScopes = scopes.filter(s => !s.holder).map(s => s.name);
const projectScopes = scopes.filter(s => !s.holder && s.level === 'project').map(s => s.name);

function picker(w: VueWrapper, label: string) {
  const all = () => w.findAll('label').find(l => l.text() === label)!.get('input');
  const boxes = () => w.findAll('input[type="checkbox"]').filter(b => scopes.some(s => s.name === (b.element as HTMLInputElement).value));
  const state = () => {
    const el = all().element as HTMLInputElement;
    return el.indeterminate ? 'some' : el.checked ? 'all' : 'none';
  };
  return { all, boxes, state };
}

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('api key scopes: select all', () => {
  it('ticks every scope, shows mixed after one is unticked, and clears them all', async () => {
    const w = mount(APIKeyCreateModal, { props: { orgId }, attachTo: document.body });
    await w.get('button').trigger('click');
    const p = picker(w, 'apikeys.select_all_scopes');
    expect(p.state()).toBe('none');
    await p.all().setValue(true);
    expect(p.boxes().map(b => (b.element as HTMLInputElement).checked)).toEqual(orgScopes.map(() => true));
    expect(p.state()).toBe('all');
    await p.boxes()[0]!.setValue(false);
    expect([p.state(), p.all().attributes('aria-checked')]).toEqual(['some', 'mixed']);
    await p.all().setValue(true);
    await p.all().setValue(false);
    expect(p.boxes().some(b => (b.element as HTMLInputElement).checked)).toBe(false);
  });

  it('sends only the scopes a project key can hold after switching kind', async () => {
    const createAPIKey = vi.fn().mockResolvedValue({ plaintext: 'oak_once' });
    setClient('$apiKeyClient', { createAPIKey });
    const w = mount(APIKeyCreateModal, { props: { orgId }, attachTo: document.body });
    await w.get('button').trigger('click');
    const p = picker(w, 'apikeys.select_all_scopes');
    await p.all().setValue(true);
    await w.get('dialog input[type="radio"][value="project"]').setValue(true);
    expect(p.state()).toBe('all');
    await w.get('dialog input[type="text"]').setValue('deploy');
    await w.findAll('dialog button').find(b => b.text() === 'common.create')!.trigger('click');
    await flushPromises();
    expect([...createAPIKey.mock.calls[0]![0].scopes].sort()).toEqual([...projectScopes].sort());
  });
});

describe('personal key scopes: select all', () => {
  const scopeOptions = [{ name: 'agent:read', label: 'Read agents' }, { name: 'agent:write', label: 'Edit agents' }];

  function form(initial: string[]) {
    const w = mount(PersonalKeyCreateForm, { props: { 'expiryOptions': [{ days: 30, label: '30 days' }], 'projects': [], scopeOptions, 'scopes': initial, 'onUpdate:scopes': (next: string[]) => w.setProps({ scopes: next }) } });
    return { w, p: picker(w, 'Select all scopes'), value: () => w.props('scopes') as string[] };
  }

  it('shows mixed for some, grants the rest, and unticking keeps values outside the list', async () => {
    const { p, value } = form(['agent:read', 'other:read']);
    expect(p.state()).toBe('some');
    await p.all().setValue(true);
    expect(value()).toEqual(['agent:read', 'other:read', 'agent:write']);
    expect(p.state()).toBe('all');
    await p.all().setValue(false);
    expect(value()).toEqual(['other:read']);
    expect(p.state()).toBe('none');
  });

  it('is disabled while the form is busy', () => {
    const w = mount(PersonalKeyCreateForm, { props: { expiryOptions: [], projects: [], scopeOptions, busy: true } });
    expect(picker(w, 'Select all scopes').all().attributes('disabled')).toBeDefined();
  });
});
