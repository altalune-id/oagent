import type { Meta, StoryObj } from '@storybook/vue3-vite';
import OrgsEmptyState from './OrgsEmptyState.vue';

const meta = {
  title: 'Org/OrgsEmptyState',
  component: OrgsEmptyState,
  args: {
    logoutUrl: '/logout',
    labels: {
      title: 'You\'re not in any organization yet',
      body: 'You\'re not a member of any organization. Ask an admin to invite you.',
      signOut: 'Sign out',
    },
  },
} satisfies Meta<typeof OrgsEmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
