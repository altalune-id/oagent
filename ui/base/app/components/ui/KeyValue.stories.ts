import type { Meta, StoryObj } from '@storybook/vue3-vite';
import KeyValue from './KeyValue.vue';

const meta = {
  title: 'UI/KeyValue',
  component: KeyValue,
  args: { items: [{ label: 'Model', value: 'large-2' }, { label: 'Agent ID', value: 'kT4vRz8Wq2NpLdY6hXsMb', mono: true, copy: true }] },
} satisfies Meta<typeof KeyValue>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { KeyValue },
    setup: () => ({ args }),
    template: `<KeyValue v-bind="args" />`,
  }),
};
