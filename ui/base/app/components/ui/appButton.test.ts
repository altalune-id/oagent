// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import AppButton from './AppButton.vue';

describe('appButton', () => {
  it('is a primary button by default, typed so it never submits a form by accident', () => {
    const b = mount(AppButton, { slots: { default: 'Save' } }).get('button');
    expect(b.attributes('type')).toBe('button');
    expect(b.classes()).toEqual(expect.arrayContaining(['btn', 'btn-primary']));
    expect(b.text()).toBe('Save');
  });

  it('maps variant, size, soft and square onto daisyUI classes, plain adding no colour', () => {
    const b = mount(AppButton, { props: { variant: 'error', size: 'sm', soft: true, square: true } }).get('button');
    expect(b.classes()).toEqual(expect.arrayContaining(['btn-error', 'btn-sm', 'btn-soft', 'btn-square']));
    expect(mount(AppButton, { props: { variant: 'plain' } }).get('button').classes()).toEqual(['btn']);
  });

  it('disables itself and shows a hidden spinner while busy', () => {
    const b = mount(AppButton, { props: { busy: true } }).get('button');
    expect(b.attributes('disabled')).toBeDefined();
    expect(b.get('.loading').attributes('aria-hidden')).toBe('true');
  });

  it('renders an in-app link for to and a plain link for href', () => {
    const link = mount(AppButton, { props: { to: '/tokens', variant: 'ghost' } });
    expect(link.getComponent(RouterLinkStub).props('to')).toBe('/tokens');
    const a = mount(AppButton, { props: { href: '/p/acme/login' } }).get('a');
    expect(a.attributes('href')).toBe('/p/acme/login');
    expect(a.classes()).toContain('btn-primary');
  });
});
