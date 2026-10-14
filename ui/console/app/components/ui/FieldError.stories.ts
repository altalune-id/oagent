import type { Meta, StoryObj } from '@storybook/vue3-vite';
import FieldError from './FieldError.vue';

const meta = {
  title: 'UI/FieldError',
  component: FieldError,
  args: { id: 'name-error', message: 'At most 200 characters' },
} satisfies Meta<typeof FieldError>;

export default meta;
type Story = StoryObj<typeof meta>;

export const UnderAnInput: Story = {
  render: args => ({
    components: { FieldError },
    setup: () => ({ args }),
    template: '<label class="block w-72"><input class="input input-sm input-bordered input-error w-full" aria-invalid="true" :aria-describedby="args.id" value="A very long name"><FieldError v-bind="args" /></label>',
  }),
};

export const Empty: Story = { args: { message: undefined } };
