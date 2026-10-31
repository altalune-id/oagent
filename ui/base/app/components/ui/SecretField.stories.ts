import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SecretField from './SecretField.vue';

const meta = {
  title: 'UI/SecretField',
  component: SecretField,
  args: { value: 'oagent_Q0QeIWMvn5Tf84kAq2gaFarjbHX2PQA__J-NsdUqAuE', label: 'Copy' },
} satisfies Meta<typeof SecretField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { SecretField },
    setup: () => ({ args }),
    template: '<div class="max-w-md p-8"><SecretField v-bind="args" /></div>',
  }),
};

export const Narrow: Story = {
  render: args => ({
    components: { SecretField },
    setup: () => ({ args }),
    template: '<div class="w-64 p-8"><SecretField v-bind="args" /></div>',
  }),
};
