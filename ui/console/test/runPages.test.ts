// @vitest-environment happy-dom
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

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
  it('shows status, duration, cost and steps, and every event as a labelled minimap block', async () => {
    const w = await mountPage(RunPage);
    expect(w.get('h1').text()).toContain(run.id);
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    expect(w.findAll('.stat-value').map(s => s.text())).toEqual(['run.status_waiting', '19.7s', '$0.016', 'run.steps_of|{"n":8,"max":12}']);
    const blocks = w.findAll('button[aria-pressed][aria-label^="run.block_label"]');
    expect(blocks).toHaveLength(8);
    expect(w.get('a[href="#approval"]').text()).toBe('run.review_approval');
  });

  it('selects an event from the minimap and shows it in the inspector', async () => {
    const w = await mountPage(RunPage, { attachTo: document.body });
    const tool = w.findAll('button[aria-pressed][aria-label^="run.block_label"]').find(b => b.attributes('aria-label')!.includes('orders.lookup'))!;
    await tool.trigger('click');
    expect(tool.attributes('aria-pressed')).toBe('true');
    expect(w.text()).toContain('{ "order": "2291" }');
    expect(document.activeElement?.id).toBe('event-e3');
    w.unmount();
  });

  it('hides an event type from the transcript and shows it again', async () => {
    const w = await mountPage(RunPage);
    const errorFilter = w.findAll('[role="group"] button').find(b => b.text().includes('run.lane_error'))!;
    await errorFilter.trigger('click');
    expect(errorFilter.attributes('aria-pressed')).toBe('false');
    expect(w.find('#event-e6').exists()).toBe(false);
    await errorFilter.trigger('click');
    expect(w.find('#event-e6').exists()).toBe(true);
  });

  it('approves locally: the run goes on, finishes, and the totals update', async () => {
    const w = await mountPage(RunPage);
    await w.findAll('button').find(b => b.text() === 'run.approve')!.trigger('click');
    expect(w.findAll('.stat-value')[0]!.text()).toBe('run.status_approved');
    expect(w.findAll('.stat-value')[1]!.text()).toBe('77.4s');
    expect(w.get('#approval [role="status"]').text()).toContain('run.approved_note');
    expect(w.text()).toContain('Final reply');
    expect(w.find('a[href="#approval"]').exists()).toBe(false);
  });

  it('rejects locally: the agent stops', async () => {
    const w = await mountPage(RunPage);
    await w.findAll('button').find(b => b.text() === 'run.reject')!.trigger('click');
    expect(w.findAll('.stat-value')[0]!.text()).toBe('run.status_rejected');
    expect(w.get('#approval [role="status"]').text()).toBe('run.rejected_note');
  });
});
