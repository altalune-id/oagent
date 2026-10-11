import type { Meta, StoryObj } from '@storybook/vue3-vite'
import UserMenu from './UserMenu.vue'

const meta = {
  title: 'Layout/UserMenu',
  component: UserMenu,
  args: { logoutUrl: '/logout', me: null },
} satisfies Meta<typeof UserMenu>

export default meta
type Story = StoryObj<typeof meta>

const render: Story['render'] = args => ({
  components: { UserMenu },
  setup: () => ({ args }),
  template: '<div class="flex justify-end p-8 pb-64"><UserMenu v-bind="args" /></div>',
})

export const Person: Story = {
  args: { me: { source: 'local', name: 'Ada Lovelace', email: 'ada@example.com', operator: true } },
  render,
}

export const APIKey: Story = {
  args: { me: { source: 'apikey', keyId: '0b6c0f8e-7a9e-4d0e-9a51-2c1d7c4b8f10' } },
  render,
}

export const Basic: Story = {
  args: { me: { source: 'basic', name: 'admin', operator: true } },
  render,
}

export const SignedOut: Story = {
  args: { me: null },
  render,
}
