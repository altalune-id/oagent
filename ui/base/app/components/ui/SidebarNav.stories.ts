import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SidebarNav from './SidebarNav.vue';

const meta = {
  title: 'UI/SidebarNav',
  component: SidebarNav,
  args: {
    label: 'Support Copilot',
    groups: [
      { items: [{ key: 'overview', label: 'Overview', to: '#overview', icon: 'lucide:activity', active: true }] },
      {
        label: 'Build',
        items: [
          { key: 'agents', label: 'Agents', to: '#agents', icon: 'lucide:bot' },
          { key: 'webhooks', label: 'Webhooks', to: '#webhooks', icon: 'lucide:webhook' },
        ],
      },
    ],
  },
} satisfies Meta<typeof SidebarNav>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-[40rem]"><SidebarNav v-bind="args" /></div>',
  }),
};

export const WithFooter: Story = {
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-[40rem]"><SidebarNav v-bind="args"><template #footer><span class="px-2 text-xs text-base-content/70">OAgent v0.1.0-dev</span></template></SidebarNav></div>',
  }),
};

const nested = [{
  items: [
    { key: 'configure', label: 'Configure', to: '#configure', icon: 'lucide:sliders-horizontal' },
    {
      key: 'channels',
      label: 'Channels',
      to: '#whatsapp',
      icon: 'lucide:radio-tower',
      active: true,
      children: [
        { key: 'whatsapp', label: 'WhatsApp', to: '#whatsapp', active: true },
        { key: 'api', label: 'API', to: '#api' },
      ],
    },
    { key: 'runs', label: 'Runs', to: '#runs', icon: 'lucide:list-tree' },
  ],
}];

export const Nested: Story = {
  args: { groups: nested },
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-[40rem]"><SidebarNav v-bind="args" /></div>',
  }),
};

export const NestedRail: Story = {
  args: { groups: nested, rail: true },
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-[40rem]"><SidebarNav v-bind="args" /></div>',
  }),
};
