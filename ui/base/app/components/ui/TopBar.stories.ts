import type { Meta, StoryObj } from '@storybook/vue3-vite';
import CommandPalette from './CommandPalette.vue';
import ThemeSelect from './ThemeSelect.vue';
import ThemeToggle from './ThemeToggle.vue';
import TopBar from './TopBar.vue';

const meta = {
  title: 'UI/TopBar',
  component: TopBar,
  args: { crumbs: [{ label: 'Acme', to: '#org' }, { label: 'acme-prod', to: '#project', mono: true }, { label: 'Overview' }] },
} satisfies Meta<typeof TopBar>;

export default meta;
type Story = StoryObj<typeof meta>;

const families = [{ id: 'othra', label: 'Othra', theme: 'othra-light' }, { id: 'bumblebee', label: 'Bumblebee', theme: 'bumblebee-light' }, { id: 'night', label: 'Night', theme: 'night-dark' }];

export const Overview: Story = {
  name: 'Project overview (artifact)',
  render: args => ({
    components: { ThemeSelect, ThemeToggle, TopBar },
    setup: () => ({ args, families }),
    template: `
      <div class="-m-4 border-x border-base-300">
        <TopBar v-bind="args">
          <template #controls>
            <ThemeSelect :families="families" model-value="othra" />
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
        { key: 'o1', label: 'Acme / Org overview', to: '#' },
        { key: 'o2', label: 'Acme / Members', to: '#' },
        { key: 'p1', label: 'acme-prod / Overview', to: '#' },
        { key: 'p2', label: 'acme-prod / Clients', to: '#' },
        { key: 'p3', label: 'acme-prod / Signing keys', to: '#' },
      ],
    }),
    template: '<CommandPalette :open="true" :items="items" label="Search and jump" empty="No page matches." />',
  }),
};
