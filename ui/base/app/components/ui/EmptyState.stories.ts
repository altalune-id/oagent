import type { Meta, StoryObj } from '@storybook/vue3-vite';
import EmptyState from './EmptyState.vue';

const meta = {
  title: 'UI/EmptyState',
  component: EmptyState,
  args: { title: 'No agents in Support Copilot yet', body: 'An agent needs instructions, a model and the tools it may call.', icon: 'lucide:bot' },
} satisfies Meta<typeof EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { EmptyState },
    setup: () => ({ args }),
    template: `<EmptyState v-bind="args"><template #actions><button type="button" class="btn btn-primary btn-sm text-ui">New agent</button></template></EmptyState>`,
  }),
};
