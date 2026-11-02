// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import EventLegend from './EventLegend.vue';

describe('eventLegend', () => {
  it('names every event type with a hidden mark whose shape, not only its colour, tells them apart', () => {
    const w = mount(EventLegend, { props: { labels: { model: 'Model', tool: 'Tool', approval: 'Approval', error: 'Error' } } });
    expect(w.get('ul').classes()).toEqual(['flex', 'flex-wrap', 'items-center', 'gap-x-3', 'gap-y-1', 'text-xs']);
    expect(w.findAll('li').map(li => li.text())).toEqual(['Model', 'Tool', 'Approval', 'Error']);
    const marks = w.findAll('li span[aria-hidden="true"]').map(s => s.classes());
    expect(marks).toEqual([
      ['inline-block', 'size-2.5', 'shrink-0', 'rounded-full', 'bg-info'],
      ['inline-block', 'size-2.5', 'shrink-0', 'rounded-[2px]', 'bg-base-content/70'],
      ['inline-block', 'size-2.5', 'shrink-0', 'rotate-45', 'rounded-[1px]', 'bg-warning'],
    ]);
    const x = w.get('[data-icon="lucide:x"]');
    expect(x.attributes('aria-hidden')).toBe('true');
    expect(x.classes()).toEqual(expect.arrayContaining(['stroke-[3]', 'text-error']));
  });
});
