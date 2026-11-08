import type { Meta, StoryObj } from '@storybook/vue3-vite';
import SelectAllCheckbox from './SelectAllCheckbox.vue';

const meta = {
  title: 'UI/SelectAllCheckbox',
  component: SelectAllCheckbox,
  args: { state: 'none', label: 'Select all scopes' },
} satisfies Meta<typeof SelectAllCheckbox>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NoneOn: Story = {};
export const SomeOn: Story = { args: { state: 'some' } };
export const AllOn: Story = { args: { state: 'all' } };
export const Small: Story = { args: { state: 'some', size: 'xs' } };
export const Disabled: Story = { args: { state: 'some', disabled: true } };
