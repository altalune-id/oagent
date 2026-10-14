import type { Meta, StoryObj } from '@storybook/vue3-vite';
import CopyButton from './CopyButton.vue';

const meta = {
  title: 'UI/CopyButton',
  component: CopyButton,
  args: { value: 'prj_a1b2c3d4e5f6', label: 'Copy' },
} satisfies Meta<typeof CopyButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { CopyButton },
    setup: () => ({ args }),
    template: '<div class="p-8"><CopyButton v-bind="args" /></div>',
  }),
};
