<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';
import AppModal from '../ui/AppModal.vue';
import ErrorText from '../ui/ErrorText.vue';

export interface ProfileSessionsLabels {
  title: string;
  body: string;
  signOut: string;
  confirmTitle: string;
  confirmBody: string;
  cancel: string;
}

const { confirming = false, busy = false, error = null, labels } = defineProps<{
  confirming?: boolean,
  busy?: boolean,
  error?: ShownError | null,
  labels: ProfileSessionsLabels,
}>();
const emit = defineEmits<{ ask: [], confirm: [], cancel: [] }>();
</script>

<template>
  <section class="card border border-base-300 bg-base-100">
    <div class="card-body gap-3">
      <h2 class="card-title text-base">
        {{ labels.title }}
      </h2>
      <p class="text-sm text-base-content/70">
        {{ labels.body }}
      </p>
      <ErrorText :error="error" />
      <div class="card-actions justify-end">
        <button type="button" class="btn btn-outline btn-error btn-sm" :disabled="busy" @click="emit('ask')">
          <Icon name="lucide:log-out" size="16" />
          {{ labels.signOut }}
        </button>
      </div>
    </div>
    <AppModal :open="confirming" :title="labels.confirmTitle" @close="emit('cancel')">
      <p class="text-sm">
        {{ labels.confirmBody }}
      </p>
      <template #footer>
        <button type="button" class="btn btn-ghost btn-sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </button>
        <button type="button" class="btn btn-error btn-sm" :disabled="busy" @click="emit('confirm')">
          {{ labels.signOut }}
        </button>
      </template>
    </AppModal>
  </section>
</template>
