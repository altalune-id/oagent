<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';

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
        <AppButton variant="error" soft size="sm" :disabled="busy" @click="emit('ask')">
          <Icon name="lucide:log-out" size="16" aria-hidden="true" />
          {{ labels.signOut }}
        </AppButton>
      </div>
    </div>
    <AppModal :open="confirming" :title="labels.confirmTitle" @close="emit('cancel')">
      <p class="text-ui">
        {{ labels.confirmBody }}
      </p>
      <template #footer>
        <AppButton variant="ghost" size="sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </AppButton>
        <AppButton variant="error" size="sm" :disabled="busy" @click="emit('confirm')">
          {{ labels.signOut }}
        </AppButton>
      </template>
    </AppModal>
  </section>
</template>
