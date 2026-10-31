import type { Meta, StoryObj } from '@storybook/vue3-vite';
import LanguageSelect from './LanguageSelect.vue';

const meta = {
  title: 'UI/LanguageSelect',
  component: LanguageSelect,
  args: {
    modelValue: 'en-US',
    label: 'Language',
    open: true,
    options: [
      { code: 'en-US', name: 'English (US)', icon: 'circle-flags:us' },
      { code: 'id-ID', name: 'Bahasa Indonesia', icon: 'circle-flags:id' },
      { code: 'fr-FR', name: 'Français', icon: 'circle-flags:fr' },
    ],
  },
} satisfies Meta<typeof LanguageSelect>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: args => ({
    components: { LanguageSelect },
    setup: () => ({ args }),
    template: '<div class="flex justify-end p-24"><LanguageSelect v-bind="args" /></div>',
  }),
};
