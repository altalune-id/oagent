import type { Meta, StoryObj } from '@storybook/vue3-vite';
import PersonalKeyCreateForm from './PersonalKeyCreateForm.vue';

const expiryOptions = [{ days: 7, label: '7 days' }, { days: 30, label: '30 days' }, { days: 90, label: '90 days' }, { days: 365, label: '1 year' }];
const projects = [{ id: 'x6MH9WPWjktMwCBKHy4pF', name: 'Website' }, { id: 'KdrF3t3FHtppBPmMgj4sy', name: 'App' }];
const scopeOptions = [
  { name: 'agent:read', label: 'Read agents' },
  { name: 'agent:write', label: 'Manage agents' },
  { name: 'project:read', label: 'Read projects' },
];

const meta = {
  title: 'API keys/PersonalKeyCreateForm',
  component: PersonalKeyCreateForm,
  args: { expiryOptions, projects, scopeOptions },
} satisfies Meta<typeof PersonalKeyCreateForm>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { PersonalKeyCreateForm },
  setup: () => ({ args }),
  template: '<div class="max-w-2xl p-8"><PersonalKeyCreateForm v-bind="args" /></div>',
});

export const Empty: Story = { render };

export const Filled: Story = {
  args: { name: 'Laptop', scopes: ['agent:read'], canSubmit: true },
  render,
};

export const SomeProjects: Story = {
  args: { name: 'CI', allProjects: false, projectIds: ['x6MH9WPWjktMwCBKHy4pF'], canSubmit: true },
  render,
};

export const Failed: Story = {
  args: { name: 'CI', scopes: ['agent:write'], canSubmit: true, error: { message: 'Missing permission: Manage agents', reference: 'Reference: req-1' } },
  render,
};
