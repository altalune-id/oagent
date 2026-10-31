import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SidebarNav from './SidebarNav.vue';

const meta = {
  title: 'UI/SidebarNav',
  component: SidebarNav,
  args: {
    label: 'acme',
    items: [
      { key: 'overview', label: 'Overview', active: true },
      { key: 'projects', label: 'Projects' },
      { key: 'settings', label: 'Settings' },
    ],
  },
} satisfies Meta<typeof SidebarNav>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-64"><SidebarNav v-bind="args" /></div>',
  }),
};

export const WithFooter: Story = {
  render: args => ({
    components: { SidebarNav },
    setup: () => ({ args }),
    template: '<div class="flex h-64"><SidebarNav v-bind="args"><template #footer>oagent v0.1.0-dev</template></SidebarNav></div>',
  }),
};
