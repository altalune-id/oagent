import type { Meta, StoryObj } from '@storybook/vue3-vite';
import CommandPalette from './CommandPalette.vue';
import ThemeSelect from './ThemeSelect.vue';
import ThemeToggle from './ThemeToggle.vue';
import TopBar from './TopBar.vue';

const meta = {
  title: 'UI/TopBar',
  component: TopBar,
  args: { crumbs: [{ label: 'Acme', to: '#org' }, { label: 'Support Copilot', to: '#project' }, { label: 'Agents' }], place: { label: 'Support Copilot', badge: 'Production' } },
} satisfies Meta<typeof TopBar>;

export default meta;
type Story = StoryObj<typeof meta>;

const families = [{ id: 'oagent', label: 'OAgent', theme: 'oagent-light' }, { id: 'bumblebee', label: 'Bumblebee', theme: 'bumblebee-light' }, { id: 'pastel', label: 'Pastel', theme: 'pastel-light' }, { id: 'night', label: 'Night', theme: 'night-dark' }, { id: 'halloween', label: 'Halloween', theme: 'halloween-dark' }];

export const Agents: Story = {
  name: 'Agents page (artifact)',
  render: args => ({
    components: { ThemeSelect, ThemeToggle, TopBar },
    setup: () => ({ args, families }),
    template: `
      <div class="-m-4 border-x border-base-300">
        <TopBar v-bind="args">
          <template #controls>
            <ThemeSelect :families="families" model-value="oagent" />
            <ThemeToggle mode="light" />
          </template>
        </TopBar>
      </div>`,
  }),
};

export const Palette: Story = {
  name: 'Search and jump (open)',
  render: () => ({
    components: { CommandPalette },
    setup: () => ({
      items: [
        { key: 'o1', group: 'Acme', label: 'Projects', to: '#' },
        { key: 'o2', group: 'Acme', label: 'Members', to: '#' },
        { key: 'p1', group: 'Support Copilot', label: 'Overview', to: '#' },
        { key: 'p2', group: 'Support Copilot', label: 'Agents', to: '#' },
        { key: 'a1', group: 'Refund Triage', label: 'Configure', to: '#' },
        { key: 'a2', group: 'Refund Triage', label: 'Runs', to: '#' },
      ],
    }),
    template: '<CommandPalette :open="true" :items="items" label="Search and jump" empty="No page matches." />',
  }),
};
