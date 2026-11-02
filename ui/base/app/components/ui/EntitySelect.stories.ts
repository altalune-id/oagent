import type { Meta, StoryObj } from '@storybook/vue3-vite';
import EntitySelect from './EntitySelect.vue';

const meta = {
  title: 'UI/EntitySelect',
  component: EntitySelect,
  args: {
    label: 'Switch organization',
    open: true,
    items: [
      { key: 'acme', label: 'Acme', active: true },
      { key: 'north', label: 'Northwind Labs' },
    ],
  },
} satisfies Meta<typeof EntitySelect>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Organization: Story = {
  name: 'Org switcher (artifact)',
  render: args => ({
    components: { EntitySelect },
    setup: () => ({ args }),
    template: `
      <div class="h-80 w-60 bg-base-200 p-2">
        <EntitySelect v-bind="args">
          <template #trigger>
            <span class="inline-flex min-w-0 items-center gap-2"><Icon name="lucide:route" size="16" class="shrink-0" aria-hidden="true" /><span class="font-semibold">OAgent</span><span class="text-base-content/70">/</span><span class="truncate">Acme</span></span>
          </template>
          <template #actions>
            <li><button type="button"><Icon name="lucide:plus" size="16" class="shrink-0" />New organization</button></li>
          </template>
        </EntitySelect>
      </div>`,
  }),
};

export const Project: Story = {
  name: 'Project row (artifact)',
  args: {
    label: 'Switch project',
    title: 'Products and use cases',
    triggerClass: 'h-auto min-h-8 py-1',
    items: [
      { key: 'support', label: 'Support Copilot', active: true },
      { key: 'invoices', label: 'Invoice Review' },
    ],
  },
  render: args => ({
    components: { EntitySelect },
    setup: () => ({ args }),
    template: `
      <div class="h-80 w-60 bg-base-200">
        <div class="my-1 border-y border-base-300 p-2">
          <EntitySelect v-bind="args">
            <template #trigger><span class="flex min-w-0 flex-col items-start gap-0.5"><span class="truncate font-medium">Support Copilot</span><span class="badge badge-xs">Production</span></span></template>
            <template #actions>
              <li><button type="button"><Icon name="lucide:plus" size="16" class="shrink-0" />New project</button></li>
            </template>
          </EntitySelect>
        </div>
      </div>`,
  }),
};
