import type { Meta, StoryObj } from '@storybook/vue3-vite';
import UserMenu from './UserMenu.vue';

const meta = {
  title: 'Layout/UserMenu',
  component: UserMenu,
  args: { logoutUrl: '/logout', me: null },
} satisfies Meta<typeof UserMenu>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { UserMenu },
  setup: () => ({ args }),
  template: '<div class="flex justify-end p-8 pb-64"><UserMenu v-bind="args" /></div>',
});

export const Person: Story = {
  args: { me: { source: 'local', userId: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Ada Lovelace', email: 'ada@example.com', operator: true } },
  render,
};

export const APIKey: Story = {
  args: { me: { source: 'apikey', keyHint: 'uAuE' } },
  render,
};

export const Basic: Story = {
  args: { me: { source: 'basic', name: 'admin', operator: true } },
  render,
};

export const SignedOut: Story = {
  args: { me: null },
  render,
};

export const PersonWithPicture: Story = {
  args: {
    me: {
      source: 'local',
      userId: 'FtGrdYskgDVDf8apKsMdn',
      name: 'Budi Santoso',
      email: 'budi@example.com',
      avatarUrl: 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1" fill="%23888"/></svg>',
    },
  },
  render,
};
