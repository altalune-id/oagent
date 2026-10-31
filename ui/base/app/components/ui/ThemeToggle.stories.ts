import type { Meta, StoryObj } from '@storybook/vue3-vite';
import ThemeToggle from './ThemeToggle.vue';

const meta = {
  title: 'UI/ThemeToggle',
  component: ThemeToggle,
  args: { mode: 'light', lightLabel: 'Light', darkLabel: 'Dark' },
} satisfies Meta<typeof ThemeToggle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Light: Story = {
  render: args => ({
    components: { ThemeToggle },
    setup: () => ({ args }),
    template: '<div class="p-8"><ThemeToggle v-bind="args" /></div>',
  }),
};

export const Dark: Story = {
  args: { mode: 'dark' },
  render: args => ({
    components: { ThemeToggle },
    setup: () => ({ args }),
    template: '<div class="p-8"><ThemeToggle v-bind="args" /></div>',
  }),
};
