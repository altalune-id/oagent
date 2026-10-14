import type { Meta, StoryObj } from '@storybook/vue3-vite';
import { mcpSnippet } from './mcpSnippet';
import PersonalKeys from './PersonalKeys.vue';

const scopes = [
  { name: 'agent:read', label: 'Read agents' },
  { name: 'agent:write', label: 'Write agents' },
  { name: 'webhook:read', label: 'Read webhooks' },
];
const projects = [{ id: 'x6MH9WPWjktMwCBKHy4pF', name: 'Default' }, { id: 'KdrF3t3FHtppBPmMgj4sy', name: 'Support' }];

const meta = {
  title: 'API keys/PersonalKeys',
  component: PersonalKeys,
  args: { keys: [], scopes, projects },
} satisfies Meta<typeof PersonalKeys>;

export default meta;
type Story = StoryObj<typeof meta>;

const render: Story['render'] = args => ({
  components: { PersonalKeys },
  setup: () => ({ args }),
  template: '<div class="max-w-3xl p-8"><PersonalKeys v-bind="args" /></div>',
});

export const Empty: Story = { render };

export const List: Story = {
  args: {
    keys: [
      { rowKey: 'key-1', name: 'laptop', scopes: ['agent:read', 'agent:write'], secretHint: 'a1b2' },
      { rowKey: 'key-2', name: 'old ci', scopes: ['agent:read'], secretHint: 'c3d4', revokedAt: new Date('2026-10-01') },
    ],
  },
  render,
};

export const JustCreated: Story = {
  args: {
    keys: [{ rowKey: 'key-1', name: 'laptop', scopes: ['agent:read'], secretHint: 'uAuE' }],
    created: {
      plaintext: 'oagent_Q0QeIWMvn5Tf84kAq2gaFarjbHX2PQA__J-NsdUqAuE',
      snippet: mcpSnippet('oagent', 'https://oagent.example.com/mcp', 'oagent_Q0QeIWMvn5Tf84kAq2gaFarjbHX2PQA__J-NsdUqAuE'),
    },
  },
  render,
};

export const SignedOut: Story = {
  args: { signedIn: false },
  render,
};
