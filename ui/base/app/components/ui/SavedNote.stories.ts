import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SavedNote from './SavedNote.vue';

const meta = {
  title: 'UI/SavedNote',
  component: SavedNote,
  args: { label: 'Saved' },
} satisfies Meta<typeof SavedNote>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NextToSave: Story = {
  render: args => ({
    components: { SavedNote },
    setup: () => ({ args }),
    template: '<div class="flex items-center gap-3 p-8"><button class="btn btn-primary btn-sm">Save</button><SavedNote v-bind="args" /></div>',
  }),
};

export const Published: Story = { args: { label: 'Version 2 published.' } };
