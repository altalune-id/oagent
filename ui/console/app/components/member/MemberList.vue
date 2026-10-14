<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';
import { computed } from 'vue';
import AppModal from '../ui/AppModal.vue';
import ErrorText from '../ui/ErrorText.vue';
import { displayName } from './names';

export interface MemberListRow {
  userId: string;
  email: string;
  name: string;
  role: string;
  initials: string;
  self: boolean;
  roles: string[];
  removable: boolean;
}

export interface MemberListLabels {
  member: string;
  role: string;
  you: string;
  owner: string;
  admin: string;
  memberRole: string;
  remove: string;
  leave: string;
  cancel: string;
  removeTitle: string;
  leaveTitle: string;
  roleTitle: string;
  changeRole: string;
  empty: string;
}

const props = withDefaults(defineProps<{
  rows: MemberListRow[],
  canLeave?: boolean,
  busy?: boolean,
  error?: ShownError | null,
  confirming?: { kind: 'remove' | 'leave' | 'role', name: string } | null,
  confirmMessage?: string,
  labels?: MemberListLabels,
}>(), {
  canLeave: false,
  busy: false,
  error: null,
  confirming: null,
  confirmMessage: '',
  labels: () => ({
    member: 'Member',
    role: 'Role',
    you: 'you',
    owner: 'Owner',
    admin: 'Admin',
    memberRole: 'Member',
    remove: 'Remove',
    leave: 'Leave organization',
    cancel: 'Cancel',
    removeTitle: 'Remove member',
    leaveTitle: 'Leave organization',
    roleTitle: 'Change role?',
    changeRole: 'Change role',
    empty: 'No members yet.',
  }),
});

const emit = defineEmits<{
  role: [userId: string, role: string],
  remove: [userId: string],
  leave: [],
  confirm: [],
  cancel: [],
}>();

function roleLabel(role: string): string {
  switch (role) {
    case 'owner':
      return props.labels.owner;
    case 'admin':
      return props.labels.admin;
    case 'member':
      return props.labels.memberRole;
    default:
      return role;
  }
}

const confirmTitle = computed(() => {
  switch (props.confirming?.kind) {
    case 'leave':
      return props.labels.leaveTitle;
    case 'role':
      return props.labels.roleTitle;
    default:
      return props.labels.removeTitle;
  }
});

const confirmAction = computed(() => {
  switch (props.confirming?.kind) {
    case 'leave':
      return props.labels.leave;
    case 'role':
      return props.labels.changeRole;
    default:
      return props.labels.remove;
  }
});

function onRole(m: MemberListRow, event: Event) {
  const select = event.target as HTMLSelectElement;
  const role = select.value;
  select.value = m.role;
  emit('role', m.userId, role);
}
</script>

<template>
  <div class="space-y-4">
    <ErrorText :error="error" />

    <p v-if="rows.length === 0" class="text-sm text-base-content/60">
      {{ labels.empty }}
    </p>
    <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{{ labels.member }}</th>
            <th>{{ labels.role }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in rows" :key="m.userId" class="hover:bg-base-200">
            <td>
              <div class="flex items-center gap-3">
                <div class="avatar avatar-placeholder">
                  <div class="w-9 rounded-full bg-neutral text-neutral-content">
                    <span class="text-xs" aria-hidden="true">{{ m.initials }}</span>
                  </div>
                </div>
                <div class="min-w-0">
                  <div class="truncate font-medium">
                    {{ displayName(m) }}
                    <span v-if="m.self" class="badge badge-ghost badge-sm ml-1">{{ labels.you }}</span>
                  </div>
                  <div v-if="m.name.trim()" class="truncate text-xs text-base-content/60">
                    {{ m.email }}
                  </div>
                </div>
              </div>
            </td>
            <td>
              <select
                v-if="m.roles.length > 0"
                class="select select-sm w-36 border-base-300"
                :value="m.role"
                :aria-label="`${labels.role}: ${displayName(m)}`"
                :disabled="busy"
                @change="onRole(m, $event)"
              >
                <option v-for="r in m.roles" :key="r" :value="r">
                  {{ roleLabel(r) }}
                </option>
              </select>
              <span v-else class="badge badge-sm" :class="m.role === 'owner' ? 'badge-primary' : 'badge-ghost'">
                {{ roleLabel(m.role) }}
              </span>
            </td>
            <td class="whitespace-nowrap text-right">
              <button v-if="m.removable" type="button" class="btn btn-ghost btn-xs text-error" :disabled="busy" @click="emit('remove', m.userId)">
                <Icon name="lucide:user-minus" size="14" />
                {{ labels.remove }}
              </button>
              <button v-else-if="m.self && canLeave" type="button" class="btn btn-ghost btn-xs" :disabled="busy" @click="emit('leave')">
                <Icon name="lucide:log-out" size="14" />
                {{ labels.leave }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <AppModal :open="confirming !== null" :title="confirmTitle" @close="emit('cancel')">
      <p class="text-sm">
        {{ confirmMessage }}
      </p>
      <template #footer>
        <button type="button" class="btn btn-ghost btn-sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </button>
        <button type="button" class="btn btn-sm" :class="confirming?.kind === 'role' ? 'btn-primary' : 'btn-error'" :disabled="busy" @click="emit('confirm')">
          {{ confirmAction }}
        </button>
      </template>
    </AppModal>
  </div>
</template>
