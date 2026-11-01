import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppShell from './AppShell.vue';

const meta = {
  title: 'UI/AppShell',
  component: AppShell,
  args: { open: false },
} satisfies Meta<typeof AppShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { AppShell },
    setup: () => ({ args }),
    template: `<AppShell v-bind="args" class="h-80"><main class="p-4">Page</main><template #sidebar><ul class="menu min-h-full w-64 bg-base-200"><li><a href="#one">Agents</a></li></ul></template></AppShell>`,
  }),
};
