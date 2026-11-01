import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SecretReveal from './SecretReveal.vue';

const meta = {
  title: 'UI/SecretReveal',
  component: SecretReveal,
  args: { value: 'sample_not_a_real_secret_4f2a', env: { OAGENT_TOKEN: 'sample_not_a_real_secret_4f2a' }, labels: { once: 'Shown once. After this page you can only rotate it.', copy: 'Copy', copyEnv: 'Copy as .env', confirm: 'I stored this secret somewhere safe', missing: 'The secret could not be loaded.' } },
} satisfies Meta<typeof SecretReveal>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { SecretReveal },
    setup: () => ({ args }),
    template: `<SecretReveal v-bind="args" />`,
  }),
};
