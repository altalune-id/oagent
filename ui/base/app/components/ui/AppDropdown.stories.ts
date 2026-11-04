import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppDropdown from './AppDropdown.vue';

const meta = {
  title: 'UI/AppDropdown',
  component: AppDropdown,
  args: { label: 'Account', open: true, placement: 'end', contentClass: 'w-60' },
} satisfies Meta<typeof AppDropdown>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Menu: Story = {
  render: args => ({
    components: { AppDropdown },
    setup: () => ({ args }),
    template: `
      <AppDropdown v-bind="args">
        <template #trigger>ada@example.com</template>
        <ul class="menu w-full p-0">
          <li><button type="button">Switch account</button></li>
          <li><button type="button">Sign out</button></li>
        </ul>
      </AppDropdown>
    `,
  }),
};
