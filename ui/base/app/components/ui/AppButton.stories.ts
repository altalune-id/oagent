import type { Meta, StoryObj } from '@storybook/vue3-vite';
import AppButton from './AppButton.vue';

const meta = {
  title: 'UI/AppButton',
  component: AppButton,
  args: { variant: 'primary', size: 'md' },
  argTypes: {
    variant: { control: 'select', options: ['primary', 'neutral', 'ghost', 'outline', 'error', 'plain'] },
    size: { control: 'inline-radio', options: ['xs', 'sm', 'md', 'lg'] },
    type: { control: 'select', options: ['button', 'submit', 'reset'] },
  },
} satisfies Meta<typeof AppButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Primary: Story = {
  render: args => ({
    components: { AppButton },
    setup: () => ({ args }),
    template: '<AppButton v-bind="args">Button</AppButton>',
  }),
};

export const Outline: Story = {
  args: { variant: 'outline' },
  render: args => ({
    components: { AppButton },
    setup: () => ({ args }),
    template: '<AppButton v-bind="args">Outline</AppButton>',
  }),
};

export const Small: Story = {
  args: { size: 'sm' },
  render: args => ({
    components: { AppButton },
    setup: () => ({ args }),
    template: '<AppButton v-bind="args">Small</AppButton>',
  }),
};
