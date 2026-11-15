import type { Meta, StoryObj } from '@storybook/vue3-vite';
import { ref } from 'vue';
import MarkdownEditor from './MarkdownEditor.vue';

const filled = `## Order help

- Look up the order before you answer.
- Never promise a refund; offer to **open a request**.
- See [the returns policy](https://example.com/returns).

> If you can't help, say so and share the support email.`;

const meta = {
  title: 'UI/MarkdownEditor',
  component: MarkdownEditor,
  args: { label: 'Instructions', hint: 'Rules, steps and what to avoid.', max: 32000, rows: 6, modelValue: '', placeholder: '- Look up the order before you answer.' },
} satisfies Meta<typeof MarkdownEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { MarkdownEditor },
  setup: () => ({ args, value: ref(args.modelValue) }),
  template: `<div class="max-w-xl"><MarkdownEditor v-bind="args" v-model="value" /></div>`,
});

export const Empty: Story = { render };

export const Filled: Story = { render, args: { modelValue: filled } };

export const ErrorState: Story = { name: 'Error state', render, args: { modelValue: filled, error: 'At most 32000 characters' } };

export const OverLimit: Story = { name: 'Over limit', render, args: { label: 'Who you are', hint: 'The agent\'s role and who it serves.', rows: 3, max: 40, modelValue: 'You are the support assistant for Acme 🛒 online.' } };

export const ReadOnly: Story = { name: 'Read-only', render, args: { modelValue: filled, readonly: true } };
