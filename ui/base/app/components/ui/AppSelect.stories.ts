import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppSelect from './AppSelect.vue';

const options = [
  { value: 'acme', label: 'Acme Corp' },
  { value: 'globex', label: 'Globex' },
];

const meta = {
  title: 'UI/AppSelect',
  component: AppSelect,
  args: {
    modelValue: 'acme',
    options,
    placeholder: 'Select organization',
    ariaLabel: 'Organization',
  },
} satisfies Meta<typeof AppSelect>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { AppSelect },
    setup: () => ({ args }),
    template: '<AppSelect v-bind="args" />',
  }),
};

export const Empty: Story = {
  args: { modelValue: '' },
  render: args => ({
    components: { AppSelect },
    setup: () => ({ args }),
    template: '<AppSelect v-bind="args" />',
  }),
};
