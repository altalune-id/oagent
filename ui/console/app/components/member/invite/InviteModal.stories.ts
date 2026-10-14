import type { Meta, StoryObj } from '@storybook/vue3-vite';
import InviteModal from './InviteModal.vue';

const meta = {
  title: 'Members/InviteModal',
  component: InviteModal,
  args: { open: true, roles: ['owner', 'admin', 'member'], email: '', role: 'member' },
} satisfies Meta<typeof InviteModal>;

export default meta;
type Story = StoryObj<typeof meta>;

export const OwnerForm: Story = {};

export const AdminForm: Story = { args: { roles: ['admin', 'member'], email: 'cy@example.com' } };

export const WithError: Story = {
  args: { email: 'cy@example.com', role: 'owner', error: { message: 'Only an owner can grant, change or remove the owner role', reference: 'Ref: req-1' } },
};

export const WithFieldErrors: Story = {
  args: { email: 'cy at example', fieldErrors: { email: 'Not in the right format' } },
};

export const Sent: Story = {
  args: { sentLink: 'https://oagent.example.com/invite/Zx3kq9-yT2mWb7nLp0aRvE5sQ1uC8dHf', sentMessage: 'We also emailed cy@example.com.' },
};
