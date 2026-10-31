import type { Meta, StoryObj } from '@storybook/vue3-vite';
import NavbarShell from './NavbarShell.vue';

const meta = {
  title: 'UI/NavbarShell',
  component: NavbarShell,
} satisfies Meta<typeof NavbarShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => ({
    components: { NavbarShell },
    template: `
      <NavbarShell>
        <template #brand>
          <span class="btn btn-ghost px-2 font-bold">oagent</span>
        </template>
        <span class="text-sm text-base-content/60">acme / support-bot</span>
        <template #end>
          <button class="btn btn-ghost btn-sm">Dark</button>
        </template>
      </NavbarShell>
    `,
  }),
};
