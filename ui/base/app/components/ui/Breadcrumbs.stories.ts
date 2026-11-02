import type { Meta, StoryObj } from '@storybook/vue3-vite';
import Breadcrumbs from './Breadcrumbs.vue';

const meta = {
  title: 'UI/Breadcrumbs',
  component: Breadcrumbs,
  args: { crumbs: [{ label: 'Acme', to: '#org' }, { label: 'Support Copilot', to: '#project' }, { label: 'Agents' }] },
} satisfies Meta<typeof Breadcrumbs>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Agents: Story = { name: 'Org › project › page (artifact)' };

export const Run: Story = {
  name: 'Org › project › agents › agent › page (artifact)',
  args: { crumbs: [{ label: 'Acme', to: '#org' }, { label: 'Support Copilot', to: '#project' }, { label: 'Agents', to: '#agents' }, { label: 'Refund Triage', to: '#agent' }, { label: 'Run' }] },
};
