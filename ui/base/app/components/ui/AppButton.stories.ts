import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppButton from './AppButton.vue';

const meta = {
  title: 'UI/AppButton',
  component: AppButton,
  args: { variant: 'primary', size: 'sm' },
  argTypes: {
    variant: { control: 'select', options: ['primary', 'neutral', 'ghost', 'outline', 'error', 'plain'] },
    size: { control: 'inline-radio', options: ['xs', 'sm', 'md', 'lg'] },
    type: { control: 'select', options: ['button', 'submit', 'reset'] },
  },
} satisfies Meta<typeof AppButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Playground: Story = {
  render: args => ({
    components: { AppButton },
    setup: () => ({ args }),
    template: '<AppButton v-bind="args">New agent</AppButton>',
  }),
};

export const ConsoleSet: Story = {
  name: 'Console set (artifact)',
  render: () => ({
    components: { AppButton },
    template: `
      <div class="flex flex-wrap items-center gap-2">
        <AppButton size="sm"><Icon name="lucide:plus" size="16" />New agent</AppButton>
        <AppButton variant="plain" size="sm"><Icon name="lucide:git-compare" size="16" />View changes</AppButton>
        <AppButton size="sm"><Icon name="lucide:upload" size="16" />Publish</AppButton>
        <AppButton variant="plain" size="sm">Show sample agents</AppButton>
        <AppButton variant="ghost" size="sm" class="text-base-content/70"><Icon name="lucide:search" size="16" /><span>Search</span><kbd class="kbd kbd-xs">⌘K</kbd></AppButton>
        <AppButton variant="ghost" size="xs" square aria-label="Copy link to this event"><Icon name="lucide:link" size="14" /></AppButton>
        <AppButton variant="ghost" size="sm" square aria-label="Toggle navigation"><Icon name="lucide:menu" size="16" /></AppButton>
      </div>`,
  }),
};

export const ApprovalSet: Story = {
  name: 'Approval set (artifact)',
  render: () => ({
    components: { AppButton },
    template: `
      <div class="flex flex-wrap items-center gap-2">
        <AppButton size="sm"><Icon name="lucide:check" size="16" />Approve</AppButton>
        <AppButton variant="plain" size="sm"><Icon name="lucide:pencil" size="16" />Edit amount</AppButton>
        <AppButton variant="plain" size="sm"><Icon name="lucide:x" size="16" />Reject</AppButton>
      </div>`,
  }),
};

export const States: Story = {
  render: () => ({
    components: { AppButton },
    template: `
      <div class="flex flex-wrap items-center gap-2">
        <AppButton size="sm" busy>Saving</AppButton>
        <AppButton size="sm" disabled>Publish</AppButton>
        <AppButton variant="error" size="sm" soft>Delete agent</AppButton>
      </div>`,
  }),
};
