import type { Meta, StoryObj } from '@storybook/vue3-vite';
import type { Column, TableSort } from './types';
import { computed, ref } from 'vue';
import AppBadge from './AppBadge.vue';
import DataTable from './DataTable.vue';

const meta = {
  title: 'UI/DataTable',
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const columns: Column[] = [
  { key: 'name', label: 'Agent', sortable: true },
  { key: 'version', label: 'Version' },
  { key: 'model', label: 'Model', mono: true },
  { key: 'updated', label: 'Updated', sortable: true },
];
const rows = [
  { id: 'refund-triage', name: 'Refund Triage', version: 12, draft: true, model: 'large-2', updated: '2 hours ago', age: 1 },
  { id: 'order-status', name: 'Order Status', version: 31, draft: false, model: 'small-3', updated: 'Yesterday', age: 2 },
  { id: 'ticket-router', name: 'Ticket Router', version: 8, draft: false, model: 'small-3', updated: 'Oct 28', age: 3 },
  { id: 'knowledge-answers', name: 'Knowledge Answers', version: 19, draft: false, model: 'large-2', updated: 'Oct 25', age: 4 },
  { id: 'escalation-drafts', name: 'Escalation Drafts', version: 2, draft: true, model: 'large-2', updated: 'Oct 22', age: 5 },
];
type Row = (typeof rows)[number];
const keys: Record<string, (r: Row) => string | number> = { name: r => r.name, updated: r => -r.age };

export const Agents: Story = {
  name: 'Agents (artifact)',
  render: () => ({
    components: { AppBadge, DataTable },
    setup: () => {
      const sort = ref<TableSort>({ key: 'updated', dir: 'descending' });
      const sorted = computed(() => {
        const by = keys[sort.value.key]!;
        const out = rows.toSorted((a, b) => (by(a) < by(b) ? -1 : by(a) > by(b) ? 1 : 0));
        return sort.value.dir === 'ascending' ? out : out.toReversed();
      });
      return { columns, sort, sorted };
    },
    template: `
      <DataTable v-model:sort="sort" caption="Agents" row-key="id" :columns="columns" :rows="sorted">
        <template #cell-name="{ row }"><a class="link link-hover font-medium whitespace-nowrap" href="#">{{ row.name }}</a></template>
        <template #cell-version="{ row }"><span class="whitespace-nowrap"><span class="font-mono text-xs">v{{ row.version }}</span> <AppBadge :label="row.draft ? 'Draft changes' : 'Live'" :outline="row.draft" /></span></template>
        <template #cell-updated="{ row }"><span class="whitespace-nowrap text-base-content/70">{{ row.updated }}</span></template>
      </DataTable>`,
  }),
};

export const Loading: Story = {
  render: () => ({
    components: { DataTable },
    setup: () => ({ columns }),
    template: '<DataTable caption="Agents" row-key="id" :columns="columns" :rows="[]" loading />',
  }),
};
