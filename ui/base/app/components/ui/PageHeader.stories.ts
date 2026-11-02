import type { Meta, StoryObj } from '@storybook/vue3-vite';
import PageHeader from './PageHeader.vue';

const meta = {
  title: 'UI/PageHeader',
  component: PageHeader,
  args: { title: 'Refund Triage', description: 'Triages refund requests for Acme support', preview: 'Preview', crumbs: [{ label: 'Agents', to: '#agents' }, { label: 'Refund Triage' }] },
} satisfies Meta<typeof PageHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { PageHeader },
    setup: () => ({ args }),
    template: `<PageHeader v-bind="args"><template #actions><button type="button" class="btn btn-primary btn-sm text-ui">Publish</button></template></PageHeader>`,
  }),
};
