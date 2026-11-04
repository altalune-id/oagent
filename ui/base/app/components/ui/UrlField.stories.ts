import type { Meta, StoryObj } from '@storybook/vue3-vite';
import UrlField from './UrlField.vue';

const meta = {
  title: 'UI/UrlField',
  component: UrlField,
  args: { label: 'Issuer URL', value: 'https://auth.example.com/p/acme' },
} satisfies Meta<typeof UrlField>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { UrlField },
  setup: () => ({ args }),
  template: '<div class="max-w-xl p-8"><UrlField v-bind="args" /></div>',
});

export const Medium: Story = { render };
export const Small: Story = { args: { size: 'sm', label: 'Discovery URL', value: 'https://auth.example.com/p/acme/.well-known/openid-configuration' }, render };
export const WithOpen: Story = { args: { open: true, label: 'OpenID Connect discovery URL', value: 'https://auth.example.com/p/acme/.well-known/openid-configuration' }, render };
export const WithHelp: Story = { args: { help: 'Paste this into your app\'s OpenID Connect settings as the issuer.' }, render };
export const LongAt390: Story = {
  args: { open: true, label: 'OAuth server metadata URL', value: 'https://auth.example.com/.well-known/oauth-authorization-server/othra/p/acme-customer-portal-staging' },
  render: args => ({
    components: { UrlField },
    setup: () => ({ args }),
    template: '<div class="w-[358px] p-4"><UrlField v-bind="args" /></div>',
  }),
};
