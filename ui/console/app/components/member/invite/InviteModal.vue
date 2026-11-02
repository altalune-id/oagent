<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
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
  <AppModal :open="open" :title="labels.title" :dismissible="!sentLink" @close="emit('close')">
    <div v-if="sentLink" class="space-y-3">
      <p class="text-ui">
        {{ sentMessage }}
      </p>
      <p class="text-xs text-base-content/70">
        {{ labels.linkHint }}
      </p>
      <SecretField :value="sentLink" :label="labels.copy" />
    </div>
    <form v-else id="invite-form" novalidate class="space-y-3" @submit.prevent="emit('submit')">
      <ErrorText :error="error" />
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ labels.email }}</span>
          <input
            v-model="email" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.email }" type="email"
            autocomplete="off" required :disabled="busy" :aria-invalid="!!fieldErrors.email" aria-describedby="invite-email-error"
          >
        </label>
        <FieldError id="invite-email-error" :message="fieldErrors.email" />
      </div>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ labels.role }}</span>
          <select
            v-model="role" class="select select-sm w-full text-ui" :class="{ 'select-error': fieldErrors.role }" :disabled="busy"
            :aria-invalid="!!fieldErrors.role" aria-describedby="invite-role-error"
          >
            <option v-for="r in roles" :key="r" :value="r">
              {{ roleLabel(r) }}
            </option>
          </select>
        </label>
        <FieldError id="invite-role-error" :message="fieldErrors.role" />
      </div>
    </form>
    <template #footer>
      <template v-if="sentLink">
        <AppButton size="sm" @click="emit('close')">
          {{ labels.done }}
        </AppButton>
      </template>
      <template v-else>
        <AppButton variant="ghost" size="sm" @click="emit('close')">
          {{ labels.cancel }}
        </AppButton>
        <AppButton type="submit" form="invite-form" size="sm" :disabled="busy || !email.trim()">
          {{ labels.send }}
        </AppButton>
      </template>
    </template>
  </AppModal>
</template>
