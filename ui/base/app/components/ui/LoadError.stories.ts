import type { Meta, StoryObj } from '@storybook/vue3-vite';
import LoadError from './LoadError.vue';

const meta = {
  title: 'UI/LoadError',
  component: LoadError,
  args: { error: { message: 'Failed to load your sessions', reference: 'Ref: req-1' } },
} satisfies Meta<typeof LoadError>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Failed: Story = {};
export const Retrying: Story = { args: { busy: true } };
export const Fine: Story = { args: { error: null } };
