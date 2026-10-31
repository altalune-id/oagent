import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppDropdown from './AppDropdown.vue';

const meta = {
  title: 'UI/AppDropdown',
  component: AppDropdown,
  args: { label: 'Account', open: true },
} satisfies Meta<typeof AppDropdown>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Menu: Story = {
  render: args => ({
    components: { AppDropdown },
    setup: () => ({ args }),
    template: `
      <AppDropdown v-bind="args" class="dropdown-end">
        <template #trigger>ada@example.com</template>
        <ul tabindex="-1" class="dropdown-content menu z-50 mt-2 w-60 rounded-box border border-base-300 bg-base-100 p-2 shadow">
          <li><button type="button">Switch account</button></li>
          <li><button type="button">Sign out</button></li>
        </ul>
      </AppDropdown>
    `,
  }),
};
