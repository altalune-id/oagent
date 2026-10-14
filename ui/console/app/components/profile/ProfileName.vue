<script setup lang="ts">
import ErrorText from '../ui/ErrorText.vue'
import type { ShownError } from '../../lib/apperror/message'

export interface ProfileNameLabels {
  title: string
  name: string
  hint: string
  save: string
}

const name = defineModel<string>('name', { required: true })
const { busy = false, notice = '', error = null, labels } = defineProps<{
  busy?: boolean
  notice?: string
  error?: ShownError | null
  labels: ProfileNameLabels
}>()
const emit = defineEmits<{ save: [] }>()
</script>

<template>
  <form class="card border border-base-300 bg-base-100" @submit.prevent="emit('save')">
    <div class="card-body gap-3">
      <h2 class="card-title text-base">{{ labels.title }}</h2>
      <label class="floating-label">
        <span>{{ labels.name }}</span>
        <input v-model="name" class="input w-full" type="text" name="name" maxlength="200" autocomplete="name" :placeholder="labels.name">
      </label>
      <p class="text-xs text-base-content/60">{{ labels.hint }}</p>
      <ErrorText :error="error" />
      <p v-if="notice" role="status" class="text-sm text-success">{{ notice }}</p>
      <div class="card-actions justify-end">
        <button type="submit" class="btn btn-primary btn-sm" :disabled="busy">{{ labels.save }}</button>
      </div>
    </div>
  </form>
</template>
