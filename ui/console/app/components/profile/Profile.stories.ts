import type { Meta, StoryObj } from '@storybook/vue3-vite';
import ProfileName from './ProfileName.vue';
import ProfilePassword from './ProfilePassword.vue';
import ProfileSessions from './ProfileSessions.vue';

const meta = {
  title: 'Profile/Sections',
  component: ProfileName,
  args: {
    name: 'Ana Maria',
    labels: { title: 'Name', name: 'Name', hint: 'Shown to the people in your organizations.', save: 'Save' },
  },
} satisfies Meta<typeof ProfileName>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Name: Story = {};

export const NameSaved: Story = { args: { notice: 'Your name was saved.' } };

export const NameRefused: Story = { args: { name: 'x'.repeat(40), fieldError: 'At most 200 characters' } };

const passwordLabels = {
  title: 'Password',
  current: 'Current password',
  next: 'New password',
  rules: 'At least 12 characters, not a common password, and not your email or name.',
  hint: 'Your other sessions are signed out.',
  change: 'Change password',
};

export const Password: Story = {
  render: () => ({
    components: { ProfilePassword },
    setup: () => ({ labels: passwordLabels }),
    template: '<ProfilePassword current="" next="" email="ana@example.com" :labels="labels" />',
  }),
};

export const PasswordWrong: Story = {
  render: () => ({
    components: { ProfilePassword },
    setup: () => ({ labels: passwordLabels, error: { message: 'Invalid email or password' } }),
    template: '<ProfilePassword current="nope" next="" :error="error" :labels="labels" />',
  }),
};

export const PasswordTooShort: Story = {
  render: () => ({
    components: { ProfilePassword },
    setup: () => ({ labels: passwordLabels, fieldErrors: { new_password: 'At least 12 characters' } }),
    template: '<ProfilePassword current="old one" next="short" :field-errors="fieldErrors" :labels="labels" />',
  }),
};

const sessionLabels = {
  title: 'Sessions',
  body: 'Sign out of oagent on every browser and device, this one too.',
  signOut: 'Sign out everywhere',
  confirmTitle: 'Sign out everywhere?',
  confirmBody: 'You will need to sign in again on every device.',
  cancel: 'Cancel',
};

export const Sessions: Story = {
  render: () => ({
    components: { ProfileSessions },
    setup: () => ({ labels: sessionLabels }),
    template: '<ProfileSessions :labels="labels" />',
  }),
};

export const SessionsConfirm: Story = {
  render: () => ({
    components: { ProfileSessions },
    setup: () => ({ labels: sessionLabels }),
    template: '<ProfileSessions confirming :labels="labels" />',
  }),
};
