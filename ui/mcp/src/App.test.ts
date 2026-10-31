// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

const bridge = vi.hoisted(() => ({
  current: null as null | Record<string, any>,
  context: { theme: 'light', toolInfo: { tool: { name: 'org_list' } } } as Record<string, any>,
  failConnect: false,
}));

vi.mock('@modelcontextprotocol/ext-apps', () => ({
  App: class {
    constructor() {
      bridge.current = this as unknown as Record<string, any>;
    }

    connect = vi.fn(async () => {
      if (bridge.failConnect) {
        throw new Error('no host');
      }
    });

    getHostContext = () => bridge.context;
    callServerTool = vi.fn();
  },
}));

const { default: App } = await import('./App.vue');

describe('app shell', () => {
  it('follows the host theme as it changes', async () => {
    const w = mount(App);
    await flushPromises();
    expect(document.documentElement.dataset.theme).toBe('oagent-light');
    bridge.current!.onhostcontextchanged({ theme: 'dark' });
    expect(document.documentElement.dataset.theme).toBe('oagent-dark');
    w.unmount();
  });

  it('shows a refused tool call as its text, not raw JSON', async () => {
    const w = mount(App);
    await flushPromises();
    bridge.current!.ontoolresult({ isError: true, content: [{ type: 'text', text: 'PRJ001 Project not found' }] });
    await flushPromises();
    expect(w.text()).toContain('PRJ001 Project not found');
    expect(w.text()).not.toContain('"type"');
    w.unmount();
  });

  it('renders the tool\'s view for each result, and the next result replaces the rows', async () => {
    const w = mount(App);
    await flushPromises();
    bridge.current!.ontoolinput({ arguments: {} });
    bridge.current!.ontoolresult({ structuredContent: { orgs: [{ id: 'o1', name: 'Acme', slug: 'acme' }] } });
    await flushPromises();
    expect(w.text()).toContain('Acme');
    bridge.current!.ontoolresult({ structuredContent: { orgs: [{ id: 'o2', name: 'Beta', slug: 'beta' }] } });
    await flushPromises();
    expect(w.text()).toContain('Beta');
    expect(w.text()).not.toContain('Acme');
    w.unmount();
  });

  it('says so when the host cannot connect', async () => {
    bridge.failConnect = true;
    const w = mount(App);
    await flushPromises();
    expect(w.text()).toContain('no host');
    w.unmount();
    bridge.failConnect = false;
  });
});
