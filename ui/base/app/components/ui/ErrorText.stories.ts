import type { Meta, StoryObj } from '@storybook/vue3-vite';
import ErrorText from './ErrorText.vue';

const meta = {
  title: 'UI/ErrorText',
  component: ErrorText,
  args: { error: { message: 'Webhook URL must use https' } },
} satisfies Meta<typeof ErrorText>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { ErrorText },
    setup: () => ({ args }),
    template: '<div class="max-w-md p-8"><ErrorText v-bind="args" /></div>',
  }),
};

export const WithReference: Story = {
  args: { error: { message: 'An unexpected error occurred. Please try again.', reference: 'Ref: 01J9Z3K6QW8M2' } },
  render: args => ({
    components: { ErrorText },
    setup: () => ({ args }),
    template: '<div class="max-w-md p-8"><ErrorText v-bind="args" /></div>',
  }),
};
