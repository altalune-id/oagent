import type { Meta, StoryObj } from '@storybook/vue3-vite';
import Section from './Section.vue';

const meta = {
  title: 'UI/Section',
  component: Section,
  args: { title: 'Model', description: 'The model that runs each step' },
} satisfies Meta<typeof Section>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { Section },
    setup: () => ({ args }),
    template: `<Section v-bind="args"><p>large-2</p></Section>`,
  }),
};
