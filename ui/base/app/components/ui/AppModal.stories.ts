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
          <button class="btn btn-ghost btn-sm text-ui">Cancel</button>
          <button class="btn btn-primary btn-sm text-ui">Create</button>
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
          <button class="btn btn-primary btn-sm text-ui">Done</button>
        </template>
      </AppModal>
    `,
  }),
};

export const Wide: Story = {
  args: { title: 'Edit provider', wide: true },
  render: Default.render,
};

export const StickyFooter: Story = {
  args: { title: 'Add MCP server', wide: true, stickyFooter: true },
  render: args => ({
    components: { AppModal },
    setup: () => ({ args }),
    template: `
      <AppModal v-bind="args">
        <p v-for="n in 30" :key="n" class="text-sm">Tool {{ n }}</p>
        <template #footer>
          <button class="btn btn-ghost btn-sm text-ui">Cancel</button>
          <button class="btn btn-primary btn-sm text-ui">Save</button>
        </template>
      </AppModal>
    `,
  }),
};
