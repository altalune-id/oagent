import type { Meta, StoryObj } from '@storybook/vue3-vite'
import PendingInvites from './PendingInvites.vue'

const expires = new Date('2026-10-20T09:00:00Z')
const pending = [
  { id: 'i1', email: 'cy@example.com', role: 'admin', status: 'pending', invitedByName: 'Ann Lee', expiresAt: expires },
  { id: 'i2', email: 'dewi@example.com', role: 'member', status: 'expired', invitedByName: 'Budi Santoso', expiresAt: new Date('2026-10-01T09:00:00Z') },
]
const drafts = [{ id: 'i3', email: 'eko@example.com', role: 'member', status: 'draft', invitedByName: 'Ann Lee' }]

const meta = {
  title: 'Members/PendingInvites',
  component: PendingInvites,
  args: { pending, drafts, canManage: true },
} satisfies Meta<typeof PendingInvites>

export default meta
type Story = StoryObj<typeof meta>

export const Manager: Story = {}

export const ReadOnly: Story = { args: { canManage: false } }

export const ResentLink: Story = { args: { rowLink: { id: 'i1', link: 'https://oagent.example.com/invite/Zx3kq9-yT2mWb7nLp0aRvE5sQ1uC8dHf' } } }

export const ConfirmRevoke: Story = { args: { confirming: { id: 'i1' }, confirmMessage: 'Revoke the invite for cy@example.com? The link stops working.' } }

export const WithError: Story = { args: { error: { message: 'This draft expired. Ask the assistant again.' } } }
