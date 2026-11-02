<script setup lang="ts">
import type { ErrorKind } from './errorPage';

const { kind, title, body, homeLabel, retryLabel } = defineProps<{
  kind: ErrorKind,
  title: string,
  body: string,
  homeLabel: string,
  retryLabel: string,
}>();

const emit = defineEmits<{ home: [], retry: [] }>();

const icons: Record<ErrorKind, string> = { not_found: 'lucide:compass', forbidden: 'lucide:lock', failed: 'lucide:cloud-alert' };
</script>

<template>
  <main class="mx-auto flex w-full max-w-lg flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
    <Icon :name="icons[kind]" size="40" class="text-base-content/70" />
    <h1 class="text-2xl font-semibold">
      {{ title }}
    </h1>
    <p class="text-base-content/70">
      {{ body }}
    </p>
    <div class="flex flex-wrap justify-center gap-2">
      <button v-if="kind === 'failed'" type="button" class="btn btn-primary btn-sm text-ui" @click="emit('retry')">
        <Icon name="lucide:rotate-ccw" size="16" />
        {{ retryLabel }}
      </button>
      <button type="button" class="btn btn-sm text-ui" :class="kind === 'failed' ? 'btn-ghost' : 'btn-primary'" @click="emit('home')">
        {{ homeLabel }}
      </button>
    </div>
  </main>
</template>
