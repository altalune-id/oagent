// @vitest-environment happy-dom
import type { AgentConfig } from '~/components/agent/configure/agentConfig';
import type { McpServer } from '~/stores/mcpServers';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import { fakeRuntimeClients, servers as fixtureServers } from '~~/test/fixtures/runtime';
import { emptyConfig } from '~/components/agent/configure/agentConfig';
import McpGrants from '~/components/agent/configure/McpGrants.vue';
import { grantFor, withGrant } from '~/components/agent/configure/runtime';

const [yasaku, tagihan] = fixtureServers() as [McpServer, McpServer];
const grantable = yasaku.tools.filter(t => t.grantable).map(t => t.name).sort();

function picker(initial: AgentConfig, servers = [yasaku, tagihan]) {
  const w = mount(McpGrants, { attachTo: document.body, props: { servers, 'serversLink': '/servers', 'modelValue': initial, 'onUpdate:modelValue': (next: AgentConfig) => w.setProps({ modelValue: next }) } });
  const selectAll = (server: McpServer) => w.findAll('[data-testid="select-all"]').find(l => l.text().includes(`"server":"${server.name}"`))!;
  const box = (server: McpServer) => selectAll(server).get('input').element as HTMLInputElement;
  const config = () => w.props('modelValue') as AgentConfig;
  return { w, selectAll, box, config };
}

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
});

describe('mcp grants: select all', () => {
  it('is a labelled checkbox per server that starts unticked when no tool is on', () => {
    const { selectAll, box } = picker(emptyConfig());
    expect(selectAll(yasaku).text()).toBe(`grants.select_all_context|{"server":"yasaku"} grants.select_all|{"n":${grantable.length}}`);
    expect(selectAll(yasaku).get('input').attributes('type')).toBe('checkbox');
    expect([box(yasaku).checked, box(yasaku).indeterminate]).toEqual([false, false]);
    expect(selectAll(yasaku).get('input').attributes('aria-checked')).toBeUndefined();
  });

  it('ticks every grantable tool of that server, and shows as ticked', async () => {
    const { selectAll, box, config } = picker(emptyConfig());
    await selectAll(yasaku).get('input').setValue(true);
    expect(config().grants.find(g => g.serverId === yasaku.id)!.tools.map(t => t.name).sort()).toEqual(grantable);
    expect(config().grants.some(g => g.serverId === tagihan.id)).toBe(false);
    expect([box(yasaku).checked, box(yasaku).indeterminate]).toEqual([true, false]);
  });

  it('shows the mixed state when only some tools are on, and ticking it then grants the rest', async () => {
    const first = yasaku.tools.find(t => t.grantable)!;
    const { selectAll, box, config } = picker(withGrant(emptyConfig(), yasaku, first, true));
    expect([box(yasaku).checked, box(yasaku).indeterminate]).toEqual([false, true]);
    expect(selectAll(yasaku).get('input').attributes('aria-checked')).toBe('mixed');
    await selectAll(yasaku).get('input').setValue(true);
    expect(grantable.every(name => grantFor(config(), yasaku.id, name))).toBe(true);
    expect(selectAll(yasaku).get('input').attributes('aria-checked')).toBeUndefined();
  });

  it('unticking clears only that server\'s tools', async () => {
    const both = withGrant(emptyConfig(), tagihan, tagihan.tools.find(t => t.grantable)!, true);
    const { selectAll, box, config } = picker(both);
    await selectAll(yasaku).get('input').setValue(true);
    await selectAll(yasaku).get('input').setValue(false);
    expect(config().grants.some(g => g.serverId === yasaku.id)).toBe(false);
    expect(config().grants.some(g => g.serverId === tagihan.id)).toBe(true);
    expect([box(yasaku).checked, box(yasaku).indeterminate]).toEqual([false, false]);
  });

  it('is disabled with the rest of the picker, and absent for a server with no grantable tools', () => {
    const w = mount(McpGrants, { props: { servers: [yasaku, { ...tagihan, tools: [] }], serversLink: '/servers', modelValue: emptyConfig(), disabled: true } });
    const rows = w.findAll('[data-testid="select-all"]');
    expect(rows).toHaveLength(1);
    expect(rows[0]!.get('input').attributes('disabled')).toBeDefined();
  });
});

describe('mcp grants: a shorter list', () => {
  const details = (w: ReturnType<typeof picker>['w']) => w.findAll('details').map(d => (d.element as HTMLDetailsElement).open);
  const filter = (w: ReturnType<typeof picker>['w']) => w.findAll('details')[0]!.get('input[type="search"]');
  const rows = (w: ReturnType<typeof picker>['w']) => w.findAll('details')[0]!.findAll('li').length;

  it('starts every server closed, even one with tools on, but opens a lone server', () => {
    const first = yasaku.tools.find(t => t.grantable)!;
    expect(details(picker(withGrant(emptyConfig(), yasaku, first, true)).w)).toEqual([false, false]);
    expect(details(picker(emptyConfig(), [tagihan]).w)).toEqual([true]);
  });

  it('shows only how many are on until a server\'s tools have loaded, never 0 of 0', () => {
    const { w } = picker(emptyConfig(), [yasaku, { ...tagihan, tools: [] }]);
    const [loaded, unloaded] = w.findAll('summary');
    expect(loaded!.text()).toContain(`grants.on_count|{"on":0,"total":${yasaku.tools.length}}`);
    expect(unloaded!.get('.sr-only').text()).toBe('grants.collapse_label_unloaded|{"server":"tagihan","on":0}');
    expect(unloaded!.text()).toContain('grants.on_count_none');
    expect(unloaded!.text()).not.toContain('grants.on_count_unloaded');
    expect(unloaded!.text()).not.toContain('"total"');
  });

  it('shows how many are on, before the tools load, once some are', () => {
    const { w } = picker(withGrant(emptyConfig(), tagihan, tagihan.tools.find(t => t.grantable)!, true), [yasaku, { ...tagihan, tools: [] }]);
    const unloaded = w.findAll('summary')[1]!;
    expect(unloaded.text()).toContain('grants.on_count_unloaded|{"on":1}');
    expect(unloaded.get('.sr-only').text()).toBe('grants.collapse_label_unloaded|{"server":"tagihan","on":1}');
  });

  it('names each tool checkbox by the tool only and describes it with its description', () => {
    const { w } = picker(emptyConfig());
    const tool = yasaku.tools.find(t => t.grantable && t.description)!;
    const box = w.findAll('li input.checkbox').find(i => i.element.closest('label')!.textContent!.includes(`${yasaku.name}__${tool.name}`))!;
    const label = box.element.closest('label')!;
    const named = [...label.querySelectorAll('span')].filter(s => !s.closest('[aria-hidden="true"]') && !s.querySelector('span')).map(s => s.textContent!.trim());
    expect(named).toEqual([`grants.use_tool|{"tool":"${yasaku.name}__${tool.name}"}`]);
    const desc = document.getElementById(box.attributes('aria-describedby')!)!;
    expect([desc.textContent, desc.getAttribute('aria-hidden')]).toEqual([tool.description, 'true']);
  });

  it('names the tools that are on in the summary, three then a count', () => {
    const tools = yasaku.tools.filter(t => t.grantable);
    const two = tools.slice(0, 2).reduce((c, t) => withGrant(c, yasaku, t, true), emptyConfig());
    const { w } = picker(two);
    expect(w.get('[data-testid="on-names"]').text()).toBe(tools.slice(0, 2).map(t => t.name).join(', '));
    expect(w.get('[data-testid="on-names"]').attributes('aria-hidden')).toBe('true');
    const five = tools.slice(0, 5).reduce((c, t) => withGrant(c, yasaku, t, true), emptyConfig());
    expect(picker(five).w.get('[data-testid="on-names"]').text()).toBe(`grants.on_names_more|{"names":"${tools.slice(0, 3).map(t => t.name).join(', ')}","n":2}`);
    expect(picker(emptyConfig()).w.find('[data-testid="on-names"]').exists()).toBe(false);
  });

  it('offers a filter only above eight tools, and reports what it shows', async () => {
    const { w } = picker(emptyConfig());
    expect(w.findAll('details')[1]!.find('input[type="search"]').exists()).toBe(tagihan.tools.length > 8);
    expect(yasaku.tools.length).toBeGreaterThan(8);
    const status = w.findAll('details')[0]!.get('[role="status"]');
    expect(status.text()).toBe('');
    const target = yasaku.tools.find(t => t.grantable)!;
    await filter(w).setValue(target.name);
    const n = yasaku.tools.filter(t => `${yasaku.name}__${t.name} ${t.description}`.toLowerCase().includes(target.name.toLowerCase())).length;
    expect(rows(w)).toBe(n);
    expect(status.text()).toBe(`grants.filter_count|{"n":${n},"total":${yasaku.tools.length}}`);
    await filter(w).setValue(target.description.toUpperCase());
    expect(rows(w)).toBeGreaterThan(0);
  });

  it('turns on only the shown tools, keeps hidden ones on, and says so', async () => {
    const tools = yasaku.tools.filter(t => t.grantable);
    const hidden = tools.find(t => !t.name.includes(tools[0]!.name))!;
    const { w, selectAll, config } = picker(withGrant(emptyConfig(), yasaku, hidden, true));
    await filter(w).setValue(tools[0]!.name);
    const shownN = tools.filter(t => `${yasaku.name}__${t.name} ${t.description}`.toLowerCase().includes(tools[0]!.name.toLowerCase())).length;
    expect(selectAll(yasaku).text()).toContain(`grants.select_all_shown|{"n":${shownN}}`);
    await selectAll(yasaku).get('input').setValue(true);
    await selectAll(yasaku).get('input').setValue(false);
    expect(grantFor(config(), yasaku.id, hidden.name)).toBeDefined();
    expect(grantFor(config(), yasaku.id, tools[0]!.name)).toBeUndefined();
  });

  it('says when nothing matches, and clearing the filter brings the list back and focuses it', async () => {
    const { w } = picker(emptyConfig());
    await filter(w).setValue('zzz-no-such-tool');
    expect(rows(w)).toBe(0);
    expect(w.findAll('details')[0]!.find('[data-testid="select-all"]').exists()).toBe(false);
    const none = w.findAll('details')[0]!.findAll('p').find(p => p.text().includes('grants.filter_none'))!;
    expect(none.text()).toContain('"query":"zzz-no-such-tool"');
    await none.get('button').trigger('click');
    expect(rows(w)).toBe(yasaku.tools.length);
    expect(document.activeElement).toBe(filter(w).element);
  });

  it('clamps each description to one line, keeping the full text', () => {
    const { w } = picker(emptyConfig());
    const desc = w.findAll('details')[0]!.findAll('li .line-clamp-1');
    expect(desc.length).toBeGreaterThan(0);
    expect(desc.map(d => d.text())).toContain(yasaku.tools.find(t => t.grantable)!.description);
  });
});
