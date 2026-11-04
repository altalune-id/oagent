import type { Meta, StoryObj } from '@storybook/vue3-vite';
import KeyValue from './KeyValue.vue';
import Section from './Section.vue';
import StatusLabel from './StatusLabel.vue';

const meta = {
  title: 'UI/Section',
  component: Section,
  args: { title: 'Model', description: 'The model that runs each step' },
} satisfies Meta<typeof Section>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { Section },
    setup: () => ({ args }),
    template: `<Section v-bind="args" class="max-w-xl"><p class="font-mono text-xs">large-2</p><p class="text-xs text-base-content/70">About $0.004 per step at this length.</p></Section>`,
  }),
};

const stats = [{ label: 'Status' }, { label: 'Duration', value: '52.0 s' }, { label: 'Cost', value: '$0.0213' }, { label: 'Steps', value: '9' }];

export const RunStats: Story = {
  name: 'Status rows, flush',
  render: () => ({
    components: { KeyValue, Section, StatusLabel },
    setup: () => ({ stats }),
    template: `
      <Section title="Run stats" description="Refund Triage v12" flush class="max-w-xl">
        <template #actions><a class="link text-ui" href="#">Open run</a></template>
        <KeyValue :items="stats">
          <template #value-0><StatusLabel tone="warning" label="Waiting for approval" detail="refund over $200" /></template>
        </KeyValue>
      </Section>`,
  }),
};

export const Danger: Story = {
  args: { title: 'Delete agent', description: 'Deletes every version and run of this agent.', tone: 'danger' },
};
