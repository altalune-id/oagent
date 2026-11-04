import type { Meta, StoryObj } from '@storybook/vue3-vite';
import FormField from './FormField.vue';
import SearchInput from './SearchInput.vue';

const meta = {
  title: 'UI/Inputs',
  component: FormField,
  args: { label: 'Agent name', hint: 'Shown on runs and in the API', error: '' },
} satisfies Meta<typeof FormField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { FormField },
    setup: () => ({ args }),
    template: `<FormField v-bind="args" v-slot="{ id, describedBy, invalid }"><input :id="id" class="input input-sm text-ui w-full" :class="{ 'input-error': invalid }" value="Refund Triage" :aria-describedby="describedBy" :aria-invalid="invalid"></FormField>`,
  }),
};

export const Fields: Story = {
  name: 'Configure fields (artifact)',
  render: () => ({
    components: { FormField },
    template: `
      <div class="flex max-w-xl flex-col gap-5">
        <FormField label="Instructions" v-slot="{ id, describedBy }"><textarea :id="id" class="textarea w-full h-40 text-ui leading-relaxed" :aria-describedby="describedBy">You triage refund requests for Acme support. Look up the order and its charges first. Refund duplicate charges up to the order total. Ask for approval before you create a refund. Never promise a date.</textarea></FormField>
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Model" hint="About $0.002 per step at this size" v-slot="{ id, describedBy }"><select :id="id" class="select select-sm text-ui w-full font-mono" :aria-describedby="describedBy"><option>large-2</option><option>small-3</option></select></FormField>
          <FormField label="Temperature" hint="Lower is more consistent" v-slot="{ id, describedBy }"><input :id="id" type="range" min="0" max="10" value="2" class="range range-xs w-full" :aria-describedby="describedBy"></FormField>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Max steps" v-slot="{ id, describedBy }"><input :id="id" type="number" min="1" value="12" class="input input-sm text-ui w-full" :aria-describedby="describedBy"></FormField>
          <FormField label="Budget per run" v-slot="{ id, describedBy }"><label class="input input-sm text-ui w-full">$<input :id="id" type="number" step="0.01" value="0.25" :aria-describedby="describedBy"></label></FormField>
        </div>
      </div>`,
  }),
};

export const ErrorState: Story = {
  name: 'Error state',
  render: () => ({
    components: { FormField },
    template: `
      <div class="max-w-xs">
        <FormField label="Max steps" error="Enter at least 1 step" v-slot="{ id, describedBy, invalid }"><input :id="id" type="number" value="0" class="input input-sm text-ui w-full" :class="{ 'input-error': invalid }" :aria-describedby="describedBy" :aria-invalid="invalid"></FormField>
      </div>`,
  }),
};

export const Search: Story = {
  name: 'Search with icon (artifact)',
  render: () => ({
    components: { SearchInput },
    template: `<SearchInput label="Filter agents" class="w-full sm:w-64" />`,
  }),
};
