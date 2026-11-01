import type { Meta, StoryObj } from '@storybook/vue3-vite';
import DataTable from './DataTable.vue';

const meta = {
  title: 'UI/DataTable',
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const columns = [{ key: 'name', label: 'Agent' }, { key: 'runs', label: 'Runs, 7 days', align: 'end' }];
const rows = [{ id: 'a1', name: 'Refund Triage', runs: 1842 }, { id: 'a2', name: 'Order Status', runs: 12904 }];

export const Default: Story = {
  render: () => ({
    components: { DataTable },
    setup: () => ({ columns, rows }),
    template: '<DataTable caption="Agents" row-key="id" :columns="columns" :rows="rows" />',
  }),
};

export const Loading: Story = {
  render: () => ({
    components: { DataTable },
    setup: () => ({ columns }),
    template: '<DataTable caption="Agents" row-key="id" :columns="columns" :rows="[]" loading />',
  }),
};
