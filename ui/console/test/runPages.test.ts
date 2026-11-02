// @vitest-environment happy-dom
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useToast } from '#base/components/ui/toast';
import { mountPage } from '#test/mount-page';
import { setRoute } from '#test/nuxt-globals';
import RunPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/[run].vue';
import RunsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/index.vue';
import { sampleRun } from '~/preview/samples';

const run = sampleRun();

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: `/o/acme/projects/web/agents/refund-triage/runs/${run.id}`, params: { org: 'acme', project: 'web', agent: 'refund-triage', run: run.id } });
});

describe('runs page', () => {
  it('lists the sample run under a Preview badge and note, linking to the viewer', async () => {
    const w = await mountPage(RunsPage);
    expect(w.get('.badge').text()).toBe('preview.badge');
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    expect(w.get('caption').text()).toBe('run.samples_caption');
    expect(w.text()).toContain(run.id);
  });
});

describe('run viewer', () => {
  beforeEach(() => {
    vi.stubGlobal('definePageMeta', vi.fn());
  });

  const stats = (w: Awaited<ReturnType<typeof mountPage>>) => w.findAll('header dl dd').map(d => d.text());
  const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text() === text);

  it('shows status, duration, cost and steps inline, and every event as a labelled minimap block', async () => {
    const w = await mountPage(RunPage);
    expect(w.get('h1').text()).toContain(run.id);
    expect(w.get('header p span.font-mono.text-xs').text()).toBe(run.model);
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    expect(w.findAll('header dl dt').map(d => d.text())).toEqual(['run.status', 'run.duration', 'run.cost', 'run.steps']);
    expect(stats(w)).toEqual(['run.status_waiting', '19.7s', '$0.016', 'run.steps_of|{"n":8,"max":12}']);
    const blocks = w.findAll('button[aria-pressed][aria-label^="run.block_label"]');
    expect(blocks).toHaveLength(8);
    expect(blocks.every(b => b.attributes('title') === b.attributes('aria-label'))).toBe(true);
    expect(w.findAll('.relative.h-4 span').map(s => s.text())).toEqual(['0 s', '20 s']);
  });

  it('jumps to the approval from the header', async () => {
    const w = await mountPage(RunPage, { attachTo: document.body });
    await button(w, 'run.review_approval')!.trigger('click');
    await flushPromises();
    expect(document.activeElement?.id).toBe('event-e9');
    expect(w.get('#event-e9').attributes('aria-pressed')).toBe('true');
    w.unmount();
  });

  it('selects an event from the minimap and shows it in the inspector, with a link to it', async () => {
    const w = await mountPage(RunPage, { attachTo: document.body });
    const tool = w.findAll('button[aria-pressed][aria-label^="run.block_label"]').find(b => b.attributes('aria-label')!.includes('orders.lookup'))!;
    await tool.trigger('click');
    await flushPromises();
    expect(tool.attributes('aria-pressed')).toBe('true');
    const inspector = w.get('aside[aria-label="run.inspector"]');
    expect(inspector.get('h2').text()).toBe('orders.lookup');
    expect(inspector.findAll('dd').map(d => d.text())).toEqual(['3.1s', '0.8s', '—', '—']);
    expect(inspector.text()).toContain('{ "order": "2291" }');
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    window.history.replaceState(null, '', '/console/o/acme/projects/web/agents/refund-triage/runs/r7Xp2KqLvN9sT4bWzY1cD');
    const linkButton = inspector.get('button[aria-label="run.copy_link"]');
    expect(linkButton.find('[data-icon="lucide:link"]').exists()).toBe(true);
    await linkButton.trigger('click');
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/console/o/acme/projects/web/agents/refund-triage/runs/r7Xp2KqLvN9sT4bWzY1cD#event-e3`);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'common.copied' });
    vi.unstubAllGlobals();
    expect(inspector.findAll('tbody tr').map(r => r.findAll('td').map(d => d.text()).join(' '))).toEqual([
      'orders.lookup run.tool_calls|{"calls":1,"seconds":0.8}',
      'billing.charges.list run.tool_calls|{"calls":1,"seconds":1.2}',
      'billing.refunds.preview run.tool_calls_failed|{"calls":2,"failed":1}',
    ]);
    expect(document.activeElement?.id).toBe('event-e3');
    w.unmount();
  });

  it('opens the event named in the link', async () => {
    setRoute({ path: `/o/acme/projects/web/agents/refund-triage/runs/${run.id}`, params: { run: run.id }, hash: '#event-e4' });
    const w = await mountPage(RunPage);
    expect(w.get('aside[aria-label="run.inspector"] h2').text()).toBe('billing.charges.list');
  });

  it('hides an event type from the transcript with its checkbox and shows it again', async () => {
    const w = await mountPage(RunPage);
    const boxes = w.findAll('fieldset input[type="checkbox"]');
    expect(boxes.map(b => (b.element as HTMLInputElement).checked)).toEqual([true, true, true, true]);
    expect(w.get('fieldset legend').text()).toBe('run.show_types');
    await boxes[3]!.setValue(false);
    expect(w.find('#event-e6').exists()).toBe(false);
    await boxes[3]!.setValue(true);
    expect(w.find('#event-e6').exists()).toBe(true);
  });

  it('approves an edited amount locally: the run goes on, finishes, and the totals update', async () => {
    const w = await mountPage(RunPage);
    expect(w.find('input[type="number"]').exists()).toBe(false);
    await button(w, 'run.edit_amount')!.trigger('click');
    const input = w.get('input[type="number"]');
    expect(w.find(`label[for="${input.attributes('id')}"]`).text()).toBe('run.amount');
    await input.setValue('40');
    await button(w, 'run.approve')!.trigger('click');
    expect(stats(w)[0]).toBe('run.status_approved');
    expect(stats(w)[1]).toBe('77.4s');
    expect(w.get('#approval [role="status"]').text()).toBe('run.approved_note|{"amount":"$40.00"}');
    expect(w.text()).toContain('Final reply');
    expect(button(w, 'run.review_approval')).toBeUndefined();
  });

  it('rejects locally: the agent stops', async () => {
    const w = await mountPage(RunPage);
    await button(w, 'run.reject')!.trigger('click');
    expect(stats(w)[0]).toBe('run.status_rejected');
    expect(w.get('#approval [role="status"]').text()).toBe('run.rejected_note');
  });
});
