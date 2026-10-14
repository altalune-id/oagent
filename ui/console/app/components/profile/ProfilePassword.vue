<script setup lang="ts">
import ErrorText from '../ui/ErrorText.vue'
import type { ShownError } from '../../lib/apperror/message'

export interface ProfilePasswordLabels {
  title: string
  current: string
  next: string
  rules: string
  hint: string
  change: string
}

const current = defineModel<string>('current', { required: true })
const next = defineModel<string>('next', { required: true })
const { email = '', busy = false, notice = '', error = null, labels } = defineProps<{
  email?: string
  busy?: boolean
  notice?: string
  error?: ShownError | null
  labels: ProfilePasswordLabels
}>()
const emit = defineEmits<{ submit: [] }>()
</script>

<template>
  <form class="card border border-base-300 bg-base-100" @submit.prevent="emit('submit')">
    <div class="card-body gap-3">
      <h2 class="card-title text-base">{{ labels.title }}</h2>
      <input type="email" name="email" :value="email" autocomplete="username" class="hidden" readonly>
      <label class="floating-label">
        <span>{{ labels.current }}</span>
        <input v-model="current" class="input w-full" type="password" name="current-password" autocomplete="current-password" required :placeholder="labels.current">
      </label>
      <label class="floating-label">
        <span>{{ labels.next }}</span>
        <input v-model="next" class="input w-full" type="password" name="new-password" autocomplete="new-password" minlength="12" required :placeholder="labels.next" aria-describedby="new-password-rules">
      </label>
      <p id="new-password-rules" class="text-xs text-base-content/60">{{ labels.rules }}</p>
      <ErrorText :error="error" />
      <p v-if="notice" role="status" class="text-sm text-success">{{ notice }}</p>
      <div class="card-actions items-center justify-between">
        <p class="text-xs text-base-content/60">{{ labels.hint }}</p>
        <button type="submit" class="btn btn-primary btn-sm" :disabled="busy">{{ labels.change }}</button>
      </div>
    </div>
  </form>
</template>
