import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppModal from './AppModal.vue';

const meta = {
  title: 'UI/AppModal',
  component: AppModal,
  args: { open: true, title: 'New organization' },
} satisfies Meta<typeof AppModal>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { AppModal },
    setup: () => ({ args }),
    template: `
      <AppModal v-bind="args">
        <p class="text-sm">Modal body content.</p>
        <template #footer>
          <button class="btn btn-ghost btn-sm">Cancel</button>
          <button class="btn btn-primary btn-sm">Create</button>
        </template>
      </AppModal>
    `,
  }),
};

export const NotDismissible: Story = {
  args: { title: 'Save the client secret', dismissible: false },
  render: args => ({
    components: { AppModal },
    setup: () => ({ args }),
    template: `
      <AppModal v-bind="args">
        <p class="text-sm">Esc and the backdrop do nothing until the footer action runs.</p>
        <template #footer>
          <button class="btn btn-primary btn-sm">Done</button>
        </template>
      </AppModal>
    `,
  }),
};
