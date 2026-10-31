<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SecretField from '#base/components/ui/SecretField.vue';

export interface InviteModalLabels {
  title: string;
  email: string;
  role: string;
  owner: string;
  admin: string;
  member: string;
  send: string;
  cancel: string;
  done: string;
  linkHint: string;
  copy: string;
}

const props = withDefaults(defineProps<{
  open: boolean,
  roles: string[],
  busy?: boolean,
  error?: ShownError | null,
  fieldErrors?: Record<string, string>,
  sentLink?: string,
  sentMessage?: string,
  labels?: InviteModalLabels,
}>(), {
  busy: false,
  error: null,
  fieldErrors: () => ({}),
  sentLink: '',
  sentMessage: '',
  labels: () => ({
    title: 'Invite people',
    email: 'Email',
    role: 'Role',
    owner: 'Owner',
    admin: 'Admin',
    member: 'Member',
    send: 'Send invite',
    cancel: 'Cancel',
    done: 'Done',
    linkHint: 'Share this link if the email doesn\'t arrive. It is shown only now.',
    copy: 'Copy link',
  }),
});

const emit = defineEmits<{ submit: [], close: [] }>();
const email = defineModel<string>('email', { default: '' });
const role = defineModel<string>('role', { default: 'member' });
function roleLabel(r: string): string {
  return ({ owner: props.labels.owner, admin: props.labels.admin, member: props.labels.member } as Record<string, string>)[r] ?? r;
}
</script>

<template>
  <AppModal :open="open" :title="labels.title" @close="emit('close')">
    <div v-if="sentLink" class="space-y-3">
      <p class="text-sm">
        {{ sentMessage }}
      </p>
      <p class="text-xs text-base-content/60">
        {{ labels.linkHint }}
      </p>
      <SecretField :value="sentLink" :label="labels.copy" />
    </div>
    <form v-else id="invite-form" novalidate class="space-y-3" @submit.prevent="emit('submit')">
      <ErrorText :error="error" />
      <label class="floating-label block">
        <span>{{ labels.email }}</span>
        <input
          v-model="email" class="input w-full" :class="{ 'input-error': fieldErrors.email }" type="email" :placeholder="labels.email"
          autocomplete="off" required :disabled="busy" :aria-invalid="!!fieldErrors.email" aria-describedby="invite-email-error"
        >
        <FieldError id="invite-email-error" :message="fieldErrors.email" />
      </label>
      <label class="floating-label block">
        <span>{{ labels.role }}</span>
        <select
          v-model="role" class="select w-full" :class="{ 'select-error': fieldErrors.role }" :disabled="busy" :aria-label="labels.role"
          :aria-invalid="!!fieldErrors.role" aria-describedby="invite-role-error"
        >
          <option v-for="r in roles" :key="r" :value="r">
            {{ roleLabel(r) }}
          </option>
        </select>
        <FieldError id="invite-role-error" :message="fieldErrors.role" />
      </label>
    </form>
    <template #footer>
      <template v-if="sentLink">
        <button type="button" class="btn btn-primary btn-sm" @click="emit('close')">
          {{ labels.done }}
        </button>
      </template>
      <template v-else>
        <button type="button" class="btn btn-ghost btn-sm" @click="emit('close')">
          {{ labels.cancel }}
        </button>
        <button type="submit" form="invite-form" class="btn btn-primary btn-sm" :disabled="busy || !email.trim()">
          {{ labels.send }}
        </button>
      </template>
    </template>
  </AppModal>
</template>
