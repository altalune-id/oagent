import type { Meta, StoryObj } from '@storybook/vue3-vite';
import { useToast } from './toast';
import ToastHost from './ToastHost.vue';

const meta = {
  title: 'UI/ToastHost',
  component: ToastHost,
  args: { closeLabel: 'Dismiss' },
} satisfies Meta<typeof ToastHost>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { ToastHost },
    setup: () => ({ args, push: useToast().push }),
    template: `<div><button type="button" class="btn btn-sm text-ui" @click="push('success', 'Draft saved')">Show a toast</button><ToastHost v-bind="args" /></div>`,
  }),
};
