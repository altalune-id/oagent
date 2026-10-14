import type { Meta, StoryObj } from '@storybook/vue3-vite';
import type { MemberListRow } from './MemberList.vue';
import MemberList from './MemberList.vue';
import { canRemove, roleOptions } from './roles';

const people = [
  { userId: 'FtGrdYskgDVDf8apKsMdn', email: 'ann@example.com', name: 'Ann Lee', role: 'owner', initials: 'AL' },
  { userId: '5jeWP8ut8V9AC9GBMMHfx', email: 'budi@example.com', name: 'Budi Santoso', role: 'owner', initials: 'BS' },
  { userId: 'jzdsjzt2UHJ3Cmr3uN2VP', email: 'cy@example.com', name: 'Cy', role: 'admin', initials: 'C' },
  { userId: 'kADpdB8fUNPkUJ9KujBCv', email: 'dewi@example.com', name: '', role: 'member', initials: 'D' },
];

function rowsFor(viewerId: string): MemberListRow[] {
  const viewer = { userId: viewerId, role: people.find(p => p.userId === viewerId)?.role ?? '' };
  return people.map(p => ({ ...p, self: p.userId === viewerId, roles: roleOptions(viewer, p), removable: canRemove(viewer, p) }));
}

const meta = {
  title: 'Members/MemberList',
  component: MemberList,
  args: { rows: rowsFor('FtGrdYskgDVDf8apKsMdn'), canLeave: true },
} satisfies Meta<typeof MemberList>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { MemberList },
  setup: () => ({ args }),
  template: '<div class="max-w-3xl p-8"><MemberList v-bind="args" /></div>',
});

export const OwnerView: Story = { render };

export const AdminView: Story = { args: { rows: rowsFor('jzdsjzt2UHJ3Cmr3uN2VP') }, render };

export const MemberView: Story = { args: { rows: rowsFor('kADpdB8fUNPkUJ9KujBCv') }, render };

export const ConfirmRemove: Story = {
  args: { confirming: { kind: 'remove', name: 'Cy' }, confirmMessage: 'Remove Cy from this organization? They lose access at once. They are signed out of the console and their personal API keys for this organization are revoked.' },
  render,
};

export const ConfirmRole: Story = {
  args: { confirming: { kind: 'role', name: 'Cy' }, confirmMessage: 'Change Cy from Admin to Member? They get an email about it. If the role is lowered, they are signed out of the console.' },
  render,
};

export const WithError: Story = {
  args: { error: { message: 'Only an owner can grant, change or remove the owner role', reference: 'Ref: req-1' } },
  render,
};

export const Empty: Story = { args: { rows: [], canLeave: false }, render };
