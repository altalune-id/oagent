import type { Meta, StoryObj } from '@storybook/vue3-vite';
import FormField from './FormField.vue';

const meta = {
  title: 'UI/FormField',
  component: FormField,
  args: { label: 'Name', hint: 'People see it in the run viewer', error: '' },
} satisfies Meta<typeof FormField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { FormField },
    setup: () => ({ args }),
    template: `<FormField v-bind="args" v-slot="{ id, describedBy, invalid }"><input :id="id" class="input w-full" :aria-describedby="describedBy" :aria-invalid="invalid"></FormField>`,
  }),
};
