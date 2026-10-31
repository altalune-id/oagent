import type { Meta, StoryObj } from '@storybook/vue3-vite';
import EntitySelect from './EntitySelect.vue';

const meta = {
  title: 'UI/EntitySelect',
  component: EntitySelect,
  args: {
    label: 'Acme',
    icon: 'lucide:building-2',
    open: true,
    items: [
      { key: 'acme', label: 'Acme', active: true, pinned: true },
      { key: 'globex', label: 'Globex Corporation With A Very Long Name' },
      { key: 'initech', label: 'Initech' },
    ],
    pinLabel: 'Pin',
    unpinLabel: 'Unpin',
  },
} satisfies Meta<typeof EntitySelect>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { EntitySelect },
    setup: () => ({ args }),
    template: '<div class="p-24"><EntitySelect v-bind="args" /></div>',
  }),
};

export const WithActions: Story = {
  render: args => ({
    components: { EntitySelect },
    setup: () => ({ args }),
    template: `<div class="p-24"><EntitySelect v-bind="args">
      <template #actions>
        <li><button type="button"><span>All organizations</span></button></li>
        <li><button type="button"><span>New organization</span></button></li>
      </template>
    </EntitySelect></div>`,
  }),
};

export const Empty: Story = {
  args: { items: [], label: 'Select organization', icon: '', placeholder: true, empty: 'No organizations yet.' },
  render: args => ({
    components: { EntitySelect },
    setup: () => ({ args }),
    template: '<div class="p-24"><EntitySelect v-bind="args" /></div>',
  }),
};
