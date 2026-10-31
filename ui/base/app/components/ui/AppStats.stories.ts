import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppStats from './AppStats.vue';

const meta = {
  title: 'UI/AppStats',
  component: AppStats,
  args: {
    items: [
      { label: 'Projects', value: 2 },
      { label: 'Agents', value: 0 },
      { label: 'Members', value: 1 },
    ],
  },
} satisfies Meta<typeof AppStats>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { AppStats },
    setup: () => ({ args }),
    template: '<AppStats v-bind="args" />',
  }),
};
