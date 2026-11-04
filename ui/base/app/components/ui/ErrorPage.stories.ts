import type { Meta, StoryObj } from '@storybook/vue3-vite';
import ErrorPage from './ErrorPage.vue';

const meta = {
  title: 'UI/ErrorPage',
  component: ErrorPage,
  args: { kind: 'not_found', title: 'Page not found', body: 'This page doesn\'t exist, or it moved.', homeLabel: 'All organizations', retryLabel: 'Retry' },
} satisfies Meta<typeof ErrorPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NotFound: Story = {};
export const Forbidden: Story = { args: { kind: 'forbidden', title: 'You don\'t have access' } };
export const Failed: Story = { args: { kind: 'failed', title: 'Couldn\'t load this page' } };
