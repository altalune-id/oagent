<script setup lang="ts">
import type { ErrorKind } from './errorPage';
import AppButton from './AppButton.vue';

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
  <main class="mx-auto flex w-full max-w-lg flex-1 flex-col items-center justify-center gap-3 px-4 py-16 text-center">
    <Icon :name="icons[kind]" size="24" class="shrink-0" aria-hidden="true" />
    <h1 class="text-xl font-semibold">
      {{ title }}
    </h1>
    <p class="text-ui text-base-content/70">
      {{ body }}
    </p>
    <div class="flex flex-wrap justify-center gap-2 pt-1">
      <AppButton v-if="kind === 'failed'" size="sm" @click="emit('retry')">
        <Icon name="lucide:rotate-ccw" size="16" class="shrink-0" aria-hidden="true" />
        {{ retryLabel }}
      </AppButton>
      <AppButton :variant="kind === 'failed' ? 'ghost' : 'primary'" size="sm" @click="emit('home')">
        {{ homeLabel }}
      </AppButton>
    </div>
  </main>
</template>
