import type { Meta, StoryObj } from '@storybook/vue3-vite';
import LoadingStatus from './LoadingStatus.vue';

const meta = {
  title: 'UI/LoadingStatus',
  component: LoadingStatus,
  args: { label: 'Loading…' },
} satisfies Meta<typeof LoadingStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
