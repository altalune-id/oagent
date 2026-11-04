import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SystemBadge from './SystemBadge.vue';

const meta = {
  title: 'UI/SystemBadge',
  component: SystemBadge,
  args: { label: 'System' },
} satisfies Meta<typeof SystemBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NextToAName: Story = {
  render: args => ({
    components: { SystemBadge },
    setup: () => ({ args }),
    template: '<h1 class="flex flex-wrap items-center gap-2 text-2xl font-bold">Console <SystemBadge v-bind="args" /></h1>',
  }),
};
