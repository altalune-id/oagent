import type { Meta, StoryObj } from '@storybook/vue3-vite';
import Avatar from './Avatar.vue';

const meta = {
  title: 'UI/Avatar',
  component: Avatar,
  args: { id: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Ana Maria', email: 'ana@example.com', size: 'md' },
} satisfies Meta<typeof Avatar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Initials: Story = {};

export const EmailOnly: Story = { args: { id: 'FtGrdYskgDVDf8apKsMdn', name: '', email: 'bob@example.com' } };

export const Picture: Story = {
  args: { avatarUrl: 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1" fill="%23888"/></svg>' },
};

export const Sizes: Story = {
  render: args => ({
    components: { Avatar },
    setup: () => ({ args }),
    template: '<div class="flex items-center gap-3"><Avatar v-bind="args" size="sm" /><Avatar v-bind="args" size="md" /><Avatar v-bind="args" size="lg" /></div>',
  }),
};

export const ManyPeople: Story = {
  render: () => ({
    components: { Avatar },
    setup: () => ({ people: ['Ana Maria', 'Budi Santoso', 'Cy', 'Dewi Lestari', 'Eko', 'Fajar Nugroho', 'Gita', 'Hadi'].map((name, i) => ({ id: `person-${i}-public-id-0000`.slice(0, 21), name })) }),
    template: '<div class="flex gap-2"><Avatar v-for="p in people" :key="p.id" :id="p.id" :name="p.name" /></div>',
  }),
};
