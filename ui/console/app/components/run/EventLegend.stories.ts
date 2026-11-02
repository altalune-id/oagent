import type { Meta, StoryObj } from '@storybook/vue3-vite';
import EventLegend from './EventLegend.vue';

const meta = {
  title: 'Run/EventLegend',
  component: EventLegend,
  args: { labels: { model: 'Model', tool: 'Tool', approval: 'Approval', error: 'Error' } },
} satisfies Meta<typeof EventLegend>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { name: 'Shape + colour + label (artifact)' };
