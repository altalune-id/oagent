import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppBadge from './AppBadge.vue';
import StatusLabel from './StatusLabel.vue';

const meta = {
  title: 'UI/Badges',
  component: AppBadge,
  args: { label: 'Live' },
} satisfies Meta<typeof AppBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Playground: Story = {};

export const Versions: Story = {
  name: 'Version state (artifact)',
  render: () => ({
    components: { AppBadge },
    template: `
      <div class="flex flex-col gap-2 text-ui">
        <span class="whitespace-nowrap"><span class="font-mono text-xs">v12</span> <AppBadge label="Draft changes" outline /></span>
        <span class="whitespace-nowrap"><span class="font-mono text-xs">v8</span> <AppBadge label="Live" /></span>
        <span class="whitespace-nowrap"><AppBadge label="Sample" /></span>
      </div>`,
  }),
};

export const Environment: Story = {
  name: 'Project tag (artifact)',
  render: () => ({
    components: { AppBadge },
    template: `
      <span class="flex w-56 min-w-0 flex-col items-start gap-0.5 text-ui">
        <span class="truncate font-medium">Support Copilot</span>
        <AppBadge label="Production" size="xs" />
      </span>`,
  }),
};

export const ToolApproval: Story = {
  name: 'Tool approval (artifact)',
  render: () => ({
    components: { AppBadge },
    template: `
      <label class="flex w-96 items-start gap-2 px-3 py-2 text-ui">
        <input type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm" checked />
        <span class="min-w-0 flex-1"><span class="font-mono text-xs font-medium">billing.refunds.create</span><span class="block text-base-content/70">Create a refund</span></span>
        <AppBadge label="Needs approval" icon="lucide:hand" class="shrink-0" />
      </label>`,
  }),
};

export const Status: Story = {
  name: 'Status dot + label',
  render: () => ({
    components: { StatusLabel },
    template: `
      <div class="flex flex-col gap-2">
        <StatusLabel tone="warning" label="Waiting for approval" detail="refund over $200" />
        <StatusLabel tone="success" label="Completed" detail="8 steps, 14.2 s" />
        <StatusLabel tone="error" label="Failed" detail="Retry" />
      </div>`,
  }),
};
